package cms

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
)

// BuildOptions configures the static site build.
type BuildOptions struct {
	// OutDir is the output directory for static HTML files.
	OutDir string

	// SyncFile is the optional path to write the sync payload JSON.
	// If empty, no sync file is written.
	SyncFile string

	// DownloadMedia downloads CMS images to {OutDir}/media/ during build
	// and rewrites image URLs to local paths. LQIP images are base64-inlined.
	// When false, images reference the CMS media API directly.
	DownloadMedia bool

	// Minify enables HTML/CSS/JS/SVG minification of output files.
	Minify bool
}

// fetchJob represents a single page that needs content + SEO fetched.
type fetchJob struct {
	path       string
	slug       string
	collKey    string // non-empty for collection entries
	isTemplate bool   // true for _template pages
}

// fetchResult holds the fetched data for a single page.
type fetchResult struct {
	job  fetchJob
	page PageData
}

// Build generates static HTML files for all registered pages and collections.
// Page content and SEO data are fetched concurrently (up to 10 at a time),
// then pages are rendered and written to disk.
//
// When the CMS site has multiple locales configured, Build generates
// locale-prefixed pages (e.g. /en/about, /nl/about) for each locale.
// For the default locale, pages are also built at the root path (/about).
// Single-locale sites are built without prefixes (backward compatible).
//
// For each fixed page:
//  1. Fetches content from the CMS (returns empty PageData if unavailable)
//  2. Fetches SEO data
//  3. Calls the registered RenderFunc to produce HTML
//  4. Writes the HTML to {OutDir}/{path}/index.html
//
// For each collection:
//  1. Builds the listing page at basePath
//  2. Builds the template page at basePath/_template (for CMS sync crawling)
//  3. Fetches all published entries from the API
//  4. Builds each entry page
//
// If opts.SyncFile is set, the sync payload is also written.
func (a *App) Build(ctx context.Context, opts BuildOptions) error {
	client := NewClient(a.config)

	// Set up media downloader if requested.
	var imgProc imageProcessor
	var mediaDL *mediaDownloader
	if opts.DownloadMedia {
		mediaDir := filepath.Join(opts.OutDir, "media")
		mediaDL = newMediaDownloader(mediaDir, "/media")
		imgProc = mediaDL.processor()
	}

	// Set up HTML minifier if requested.
	var m *minify.M
	if opts.Minify {
		m = minify.New()
		m.AddFunc("text/html", html.Minify)
		m.AddFunc("text/css", css.Minify)
		m.AddFunc("image/svg+xml", svg.Minify)
		m.AddFunc("application/javascript", js.Minify)
	}

	// List all published pages (shared across locale builds and sitemap).
	allPages, listErr := client.ListPages(ctx)
	if listErr != nil {
		allPages = nil
	}

	// Discover locales from the CMS.
	locales, localeErr := client.ListLocales(ctx)
	multiLocale := localeErr == nil && len(locales) > 1

	if multiLocale {
		if err := a.buildMultiLocale(ctx, client, opts, imgProc, mediaDL, m, locales, allPages); err != nil {
			return err
		}
	} else {
		if err := a.buildSingleLocale(ctx, client, opts, imgProc, mediaDL, m, allPages); err != nil {
			return err
		}
	}

	// Write template files for CMS preview (rendered with empty data,
	// preserving data-cms-* attributes and SubcollectionOr fallback entries).
	// Template files are always single-locale — they're for schema discovery.
	if err := a.writeTemplateFiles(opts.OutDir); err != nil {
		return err
	}

	// Write the layout route manifest for the SPA router.
	if a.hasLayouts() {
		if err := a.writeRouteManifest(opts.OutDir); err != nil {
			return err
		}
	}

	// Generate sitemap.xml and robots.txt when we know the public URL.
	siteURL := a.resolveSiteURL(ctx, client)
	if siteURL != "" {
		var defaultLocale string
		if multiLocale {
			for _, l := range locales {
				if l.IsDefault {
					defaultLocale = l.Code
					break
				}
			}
		}
		sd := a.collectSitemapURLs(allPages, locales, defaultLocale)
		sd.siteURL = strings.TrimRight(siteURL, "/")
		if err := sd.write(opts.OutDir, locales); err != nil {
			return fmt.Errorf("cms: write sitemap: %w", err)
		}
		if err := writeRobotsTxt(opts.OutDir, siteURL); err != nil {
			return fmt.Errorf("cms: write robots.txt: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  [ok]   sitemap.xml + robots.txt written\n")
	}

	// Write sync payload if requested.
	if opts.SyncFile != "" {
		if err := a.WriteSyncJSON(opts.SyncFile); err != nil {
			return fmt.Errorf("cms: write sync file: %w", err)
		}
	}

	return nil
}

// resolveSiteURL determines the public site URL for sitemap generation.
// It uses Config.SiteURL if set, otherwise fetches the domain from the CMS.
func (a *App) resolveSiteURL(ctx context.Context, client *Client) string {
	if a.config.SiteURL != "" {
		return a.config.SiteURL
	}
	info, err := client.GetSiteInfo(ctx)
	if err != nil || info.Domain == nil || *info.Domain == "" {
		return ""
	}
	domain := *info.Domain
	// Ensure scheme is present.
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return domain
}

// buildSingleLocale is the original single-locale build path.
// Used when the CMS site has only one locale configured (or ListLocales fails).
func (a *App) buildSingleLocale(ctx context.Context, client *Client, opts BuildOptions, imgProc imageProcessor, mediaDL *mediaDownloader, m *minify.M, allPages []apiPageListItem) error {
	// 1. Plan all pages to fetch.
	jobs, _ := a.planFetchJobs(allPages)

	// 3. Fetch all page content + SEO concurrently.
	results := a.fetchAllForLocale(ctx, client, jobs, a.config.Locale, imgProc, mediaDL)

	// 4. Assemble listings from entry results.
	listings := make(map[string][]PageData)
	for _, r := range results {
		if r.job.collKey != "" {
			listings[r.job.collKey] = append(listings[r.job.collKey], r.page)
		}
	}

	// 5. Write all pages.
	manifest := a.layoutManifest()
	for _, r := range results {
		page := r.page
		page.layoutManifest = manifest

		// Attach listings to non-entry, non-template pages.
		if r.job.collKey == "" && !r.job.isTemplate && len(listings) > 0 {
			page.listings = listings
		}

		if err := a.writePage(opts, m, page); err != nil {
			return err
		}
	}

	return nil
}

// buildMultiLocale builds all pages for each configured locale with locale-prefixed
// paths. For the default locale, pages are also built at root paths (no prefix).
func (a *App) buildMultiLocale(ctx context.Context, client *Client, opts BuildOptions, imgProc imageProcessor, mediaDL *mediaDownloader, m *minify.M, locales []SiteLocale, allPages []apiPageListItem) error {
	// Find the default locale.
	var defaultLocale string
	for _, l := range locales {
		if l.IsDefault {
			defaultLocale = l.Code
			break
		}
	}

	// 2. Plan fetch jobs (same CMS paths for all locales).
	jobs, _ := a.planFetchJobs(allPages)

	// 3. Build each locale.
	for _, locale := range locales {
		prefix := "/" + locale.Code

		fmt.Fprintf(os.Stderr, "building locale %s (%s)...\n", locale.Code, locale.Label)

		// Fetch content for this locale.
		results := a.fetchAllForLocale(ctx, client, jobs, locale.Code, imgProc, mediaDL)

		// Build prefixed version: /en/about, /nl/about, etc.
		if err := a.writeLocaleResults(opts, m, results, prefix, locales, defaultLocale); err != nil {
			return err
		}

		// For the default locale, also build at root paths (no prefix).
		if locale.IsDefault {
			if err := a.writeLocaleResults(opts, m, results, "", locales, defaultLocale); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeLocaleResults applies locale metadata to fetch results and writes them to disk.
// prefix is the locale URL prefix (e.g. "/en") or "" for the default-locale root build.
func (a *App) writeLocaleResults(opts BuildOptions, m *minify.M, results []fetchResult, prefix string, locales []SiteLocale, defaultLocale string) error {
	// Apply locale metadata and build locale-prefixed paths.
	// We work on copies to avoid mutating the originals (needed when the same
	// results are written twice: once prefixed, once at root for the default locale).
	pages := make([]struct {
		page PageData
		job  fetchJob
	}, len(results))

	manifest := a.layoutManifest()
	for i, r := range results {
		page := r.page // value copy

		page.contentPath = page.Path
		page.Locales = locales
		page.defaultLocale = defaultLocale
		page.localePrefix = prefix
		page.layoutManifest = manifest

		if prefix != "" {
			page.Path = localePrefixPath(prefix, page.Path)
		}

		setEntryLocalePrefix(page.subcollections, prefix)
		pages[i] = struct {
			page PageData
			job  fetchJob
		}{page: page, job: r.job}
	}

	// Assemble locale-scoped listings.
	listings := make(map[string][]PageData)
	for _, p := range pages {
		if p.job.collKey != "" {
			listings[p.job.collKey] = append(listings[p.job.collKey], p.page)
		}
	}

	// Write all pages.
	for _, p := range pages {
		page := p.page

		// Attach listings to non-entry, non-template pages.
		if p.job.collKey == "" && !p.job.isTemplate && len(listings) > 0 {
			page.listings = listings
		}

		if err := a.writePage(opts, m, page); err != nil {
			return err
		}
	}

	return nil
}

// localePrefixPath prepends a locale prefix to a URL path.
// "/" → "/en", "/about" → "/en/about".
func localePrefixPath(prefix, path string) string {
	if path == "/" {
		return prefix
	}
	return prefix + path
}

// planFetchJobs determines all pages that need fetching, avoiding duplicates.
// Returns the jobs slice and the seen set (for caller reference).
func (a *App) planFetchJobs(allPages []apiPageListItem) ([]fetchJob, map[string]bool) {
	var jobs []fetchJob
	seen := make(map[string]bool)

	// Collection entries first — they populate listings.
	for _, coll := range a.collections {
		prefix := coll.basePath + "/"
		for _, item := range allPages {
			if strings.HasPrefix(item.Path, prefix) && item.Path != coll.templateURL && !seen[item.Path] {
				jobs = append(jobs, fetchJob{path: item.Path, slug: item.Slug, collKey: coll.key})
				seen[item.Path] = true
			}
		}
	}

	// Fixed pages.
	for _, pageDef := range a.pages {
		if !seen[pageDef.path] {
			jobs = append(jobs, fetchJob{path: pageDef.path, slug: pathSlug(pageDef.path)})
			seen[pageDef.path] = true
		}
	}

	// Collection listing pages.
	for _, coll := range a.collections {
		if !seen[coll.basePath] {
			jobs = append(jobs, fetchJob{path: coll.basePath, slug: pathSlug(coll.basePath)})
			seen[coll.basePath] = true
		}
	}

	// Template pages (for sync crawling).
	for _, coll := range a.collections {
		if !seen[coll.templateURL] {
			jobs = append(jobs, fetchJob{path: coll.templateURL, slug: pathSlug(coll.templateURL), isTemplate: true})
			seen[coll.templateURL] = true
		}
	}

	return jobs, seen
}

// fetchAllForLocale fetches content + SEO for all jobs concurrently for a specific locale.
func (a *App) fetchAllForLocale(ctx context.Context, client *Client, jobs []fetchJob, locale string, imgProc imageProcessor, dl *mediaDownloader) []fetchResult {
	results := make([]fetchResult, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job fetchJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			page, err := client.GetPage(ctx, job.path, WithLocale(locale))
			if err != nil {
				if !job.isTemplate {
					fmt.Fprintf(os.Stderr, "  [warn] %s: no CMS content, using fallbacks (%v)\n", job.path, err)
				}
				page = NewPageData(job.path, job.slug, locale, nil, nil, nil)
			} else {
				fmt.Fprintf(os.Stderr, "  [ok]   %s: fetched CMS content\n", job.path)
			}

			seo, seoErr := client.GetSEO(ctx, job.path, WithLocale(locale))
			if seoErr == nil {
				page.seo = &seo
			}

			if imgProc != nil {
				page.imgProc = imgProc
				setEntryImageProcessor(page.subcollections, imgProc)
			}

			// Download OG image so <meta property="og:image"> uses a local path.
			if dl != nil && page.seo != nil && page.seo.OGImageURL != "" {
				if local, err := dl.download(page.seo.OGImageURL); err == nil {
					page.seo.OGImageURL = local
				}
			}

			results[i] = fetchResult{job: job, page: page}
		}(i, job)
	}

	wg.Wait()
	return results
}

// buildOnePage fetches content + SEO for a single path, renders it, and writes
// the HTML file. Falls back to empty PageData if the API is unavailable.
func (a *App) buildOnePage(ctx context.Context, client *Client, opts BuildOptions, pagePath string, imgProc imageProcessor, dl *mediaDownloader, m *minify.M, listings map[string][]PageData) error {
	page, err := client.GetPage(ctx, pagePath)
	if err != nil {
		if !strings.Contains(pagePath, "_template") {
			fmt.Fprintf(os.Stderr, "  [warn] %s: no CMS content, using fallbacks (%v)\n", pagePath, err)
		}
		page = NewPageData(pagePath, pathSlug(pagePath), a.config.Locale, nil, nil, nil)
	} else {
		fmt.Fprintf(os.Stderr, "  [ok]   %s: fetched CMS content\n", pagePath)
	}

	// Try to fetch SEO (best-effort).
	seo, seoErr := client.GetSEO(ctx, pagePath)
	if seoErr == nil {
		page.seo = &seo
	}

	// Attach image processor for static media downloads.
	if imgProc != nil {
		page.imgProc = imgProc
		setEntryImageProcessor(page.subcollections, imgProc)
	}

	// Download OG image so <meta property="og:image"> uses a local path.
	if dl != nil && page.seo != nil && page.seo.OGImageURL != "" {
		if local, err := dl.download(page.seo.OGImageURL); err == nil {
			page.seo.OGImageURL = local
		}
	}

	// Attach collection listings so index pages can iterate entries.
	if len(listings) > 0 {
		page.listings = listings
	}

	return a.writePage(opts, m, page)
}

// writePage renders a PageData, optionally minifies, and writes the HTML file.
// When layouts are registered, it also generates fragment files for each
// layout level for SPA-like navigation.
func (a *App) writePage(opts BuildOptions, m *minify.M, page PageData) error {
	output := a.renderPage(page)

	// Strip CMS attributes from production output — the data-cms-* attributes
	// and <meta name="cms-*"> tags are only needed in .template.html files
	// for CMS preview and sync, not in the HTML served to end users.
	output = stripCMSAttributes(output)

	if m != nil {
		minified, err := m.String("text/html", output)
		if err == nil {
			output = minified
		}
		// On minification error, fall through with original output.
	}

	outPath := pathToFile(opts.OutDir, page.Path)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("cms: mkdir %s: %w", filepath.Dir(outPath), err)
	}
	if err := os.WriteFile(outPath, []byte(output), 0o644); err != nil {
		return fmt.Errorf("cms: write %s: %w", outPath, err)
	}

	// Generate layout fragment files for SPA navigation.
	if a.hasLayouts() {
		if err := a.writePageFragments(opts, m, page); err != nil {
			return err
		}
	}

	return nil
}

// pathToFile converts a URL path to a filesystem path.
// "/" → "{outDir}/index.html"
// "/about" → "{outDir}/about/index.html"
// "/blog/post" → "{outDir}/blog/post/index.html"
// "/404" → "{outDir}/404.html" (special case for error pages)
func pathToFile(outDir, urlPath string) string {
	trimmed := strings.Trim(urlPath, "/")
	if trimmed == "" {
		return filepath.Join(outDir, "index.html")
	}
	// Error pages are written as {name}.html instead of {name}/index.html
	// so static hosts (Cloudflare Pages, Netlify) serve them automatically.
	base := filepath.Base(trimmed)
	if base == "404" || base == "500" {
		return filepath.Join(outDir, trimmed+".html")
	}
	return filepath.Join(outDir, trimmed, "index.html")
}

// pathSlug extracts the last segment of a path as a slug.
func pathSlug(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "index"
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// ---------------------------------------------------------------------------
// Template files (for CMS preview and sync)
// ---------------------------------------------------------------------------

// pathToTemplateFile converts a URL path to a template filesystem path.
// "/" → "{outDir}/index.template.html"
// "/about" → "{outDir}/about/index.template.html"
// "/404" → "{outDir}/404.template.html"
func pathToTemplateFile(outDir, urlPath string) string {
	trimmed := strings.Trim(urlPath, "/")
	if trimmed == "" {
		return filepath.Join(outDir, "index.template.html")
	}
	base := filepath.Base(trimmed)
	if base == "404" || base == "500" {
		return filepath.Join(outDir, trimmed+".template.html")
	}
	return filepath.Join(outDir, trimmed, "index.template.html")
}

// ---------------------------------------------------------------------------
// Layout fragments (for SPA navigation)
// ---------------------------------------------------------------------------

// pathToFragmentFile converts a URL path + layout ID to a fragment path.
// "/" → "{outDir}/_root.html"
// "/about" → "{outDir}/about/_root.html"
// "/blog/post" → "{outDir}/blog/post/_root.html"
func pathToFragmentFile(outDir, urlPath, layoutID string) string {
	trimmed := strings.Trim(urlPath, "/")
	name := "_" + layoutID + ".html"
	if trimmed == "" {
		return filepath.Join(outDir, name)
	}
	return filepath.Join(outDir, trimmed, name)
}

// writePageFragments generates fragment HTML files for each layout level.
// Each fragment contains the HTML that goes inside a layout's [data-layout] slot.
func (a *App) writePageFragments(opts BuildOptions, m *minify.M, page PageData) error {
	chain := a.layoutChain(page.contentPathOrPath())
	if len(chain) == 0 {
		return nil
	}

	// Extract title for route metadata.
	title := page.SEO().MetaTitle
	if title == "" {
		title = page.Slug
	}

	for _, layout := range chain {
		frag := a.renderPageFragment(page, layout.id)
		if frag == "" {
			continue
		}
		frag = stripCMSAttributes(frag)

		if m != nil {
			// Wrap in a temporary document for HTML minification, then unwrap.
			minified, err := m.String("text/html", frag)
			if err == nil {
				frag = minified
			}
		}

		// Prepend route metadata as an HTML comment for the SPA router.
		meta, _ := json.Marshal(map[string]string{"t": title})
		frag = "<!--route:" + string(meta) + "-->\n" + frag

		fragPath := pathToFragmentFile(opts.OutDir, page.Path, layout.id)
		if err := os.MkdirAll(filepath.Dir(fragPath), 0o755); err != nil {
			return fmt.Errorf("cms: mkdir %s: %w", filepath.Dir(fragPath), err)
		}
		if err := os.WriteFile(fragPath, []byte(frag), 0o644); err != nil {
			return fmt.Errorf("cms: write fragment %s: %w", fragPath, err)
		}
	}
	return nil
}

// writeRouteManifest writes _routes.json with the layout hierarchy
// for the SPA router to determine which fragments to fetch.
func (a *App) writeRouteManifest(outDir string) error {
	manifest := struct {
		Layouts map[string]string `json:"layouts"`
	}{
		Layouts: a.layoutManifest(),
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("cms: marshal route manifest: %w", err)
	}
	outPath := filepath.Join(outDir, "_routes.json")
	return os.WriteFile(outPath, data, 0o644)
}

// writeTemplateFiles renders each page with empty data (preserving all
// data-cms-* attributes and SubcollectionOr fallback entries) and writes
// the result as .template.html alongside the production files.
//
// These template files are used by the CMS for:
//   - Live preview (the bridge script discovers fields from data-cms-* attrs)
//   - Schema sync (inline HTML in the sync payload)
//
// Production HTML is kept clean — no CMS attributes.
func (a *App) writeTemplateFiles(outDir string) error {
	// Fixed pages.
	for _, p := range a.pages {
		data := NewPageData(p.path, pathSlug(p.path), a.config.Locale, nil, nil, nil)
		html := a.renderPage(data)
		if html == "" {
			continue
		}
		outPath := pathToTemplateFile(outDir, p.path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("cms: mkdir %s: %w", filepath.Dir(outPath), err)
		}
		if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
			return fmt.Errorf("cms: write template %s: %w", outPath, err)
		}
	}

	// Collection listing pages.
	for _, c := range a.collections {
		data := NewPageData(c.basePath, pathSlug(c.basePath), a.config.Locale, nil, nil, nil)
		html := a.renderPage(data)
		if html == "" {
			continue
		}
		outPath := pathToTemplateFile(outDir, c.basePath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("cms: mkdir %s: %w", filepath.Dir(outPath), err)
		}
		if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
			return fmt.Errorf("cms: write template %s: %w", outPath, err)
		}
	}

	// Collection entry templates (e.g. /blog/_template).
	for _, c := range a.collections {
		data := NewPageData(c.templateURL, pathSlug(c.templateURL), a.config.Locale, nil, nil, nil)
		html := a.renderPage(data)
		if html == "" {
			continue
		}
		outPath := pathToTemplateFile(outDir, c.templateURL)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("cms: mkdir %s: %w", filepath.Dir(outPath), err)
		}
		if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
			return fmt.Errorf("cms: write template %s: %w", outPath, err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// CMS attribute stripping
// ---------------------------------------------------------------------------

// Patterns for removing CMS-specific attributes from production HTML.
var (
	// data-cms-field="value", data-cms-type="text", data-cms-entry (boolean), etc.
	cmsAttrRe = regexp.MustCompile(`\s+data-cms-[\w-]+(?:="[^"]*")?`)
	// <meta name="cms-template" content="homepage"/>
	cmsMetaRe = regexp.MustCompile(`\s*<meta\s+name="cms-[^"]*"\s+content="[^"]*"\s*/?>`)
)

// stripCMSAttributes removes all data-cms-* attributes and <meta name="cms-*">
// tags from HTML, producing clean output for production serving.
func stripCMSAttributes(html string) string {
	html = cmsMetaRe.ReplaceAllString(html, "")
	html = cmsAttrRe.ReplaceAllString(html, "")
	return html
}

// ---------------------------------------------------------------------------
// Media downloading
// ---------------------------------------------------------------------------

// Default responsive widths downloaded for each image in static builds.
var defaultSrcSetWidths = []int{400, 800, 1200, 1600}

// Formats to attempt downloading during static build. If the backend does
// not support format conversion (returns 404 or error), the format is
// silently skipped — the <source> element won't be rendered.
var defaultFormats = []string{"avif", "webp"}

// mediaDownloader downloads CMS media assets to the build output directory
// and provides an imageProcessor that rewrites remote URLs to local paths.
type mediaDownloader struct {
	client    *http.Client
	outDir    string            // filesystem dir, e.g., "dist/media"
	webPrefix string            // URL prefix in built HTML, e.g., "/media"
	cache     map[string]string // remote URL -> local web path (or data URI for LQIP)
	mu        sync.Mutex
}

func newMediaDownloader(outDir, webPrefix string) *mediaDownloader {
	return &mediaDownloader{
		client:    &http.Client{},
		outDir:    outDir,
		webPrefix: webPrefix,
		cache:     make(map[string]string),
	}
}

// processor returns an imageProcessor that downloads all variants of an
// image and returns an ImageValue with the resolved map populated.
func (d *mediaDownloader) processor() imageProcessor {
	return func(img ImageValue) ImageValue {
		if img.URL == "" {
			return img
		}
		resolved := make(map[string]string)

		// 1. Download the full-size image (no params).
		if local, err := d.download(img.URL); err == nil {
			resolved[img.URL] = local
		}

		// 2. Download LQIP and base64-inline it.
		lqipURL := buildLQIPURL(img.URL)
		if data, err := d.downloadBase64(lqipURL); err == nil {
			resolved[lqipURL] = data
		}

		// 3. Download srcset width variants (original format).
		sep := "?"
		if strings.Contains(img.URL, "?") {
			sep = "&"
		}
		for _, w := range defaultSrcSetWidths {
			variantURL := img.URL + sep + fmt.Sprintf("w=%d", w)
			if local, err := d.download(variantURL); err == nil {
				resolved[variantURL] = local
			}
		}

		// 4. Download format variants (WebP, AVIF) for <picture> <source> elements.
		//    If the backend doesn't support format conversion, downloads fail
		//    silently and HasFormat() returns false — no <source> is rendered.
		for _, format := range defaultFormats {
			for _, w := range defaultSrcSetWidths {
				variantURL := img.URL + sep + fmt.Sprintf("w=%d&format=%s", w, format)
				if local, err := d.download(variantURL); err == nil {
					resolved[variantURL] = local
				}
			}
		}

		img.resolved = resolved
		img.dl = d // enable lazy downloading for custom widths (e.g. ImageSized)
		return img
	}
}

// download fetches a remote URL, saves it to outDir, and returns the web path.
// Results are cached — repeated calls for the same URL return instantly.
func (d *mediaDownloader) download(remoteURL string) (string, error) {
	d.mu.Lock()
	if local, ok := d.cache[remoteURL]; ok {
		d.mu.Unlock()
		return local, nil
	}
	d.mu.Unlock()

	resp, err := d.client.Get(remoteURL)
	if err != nil {
		return "", fmt.Errorf("cms: download %s: %w", remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("cms: download %s: status %d", remoteURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cms: read %s: %w", remoteURL, err)
	}

	ext := extensionFromResponse(resp, remoteURL)
	filename := hashFilename(remoteURL) + ext

	if err := os.MkdirAll(d.outDir, 0o755); err != nil {
		return "", fmt.Errorf("cms: mkdir %s: %w", d.outDir, err)
	}

	filePath := filepath.Join(d.outDir, filename)
	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		return "", fmt.Errorf("cms: write %s: %w", filePath, err)
	}

	webPath := d.webPrefix + "/" + filename

	d.mu.Lock()
	d.cache[remoteURL] = webPath
	d.mu.Unlock()

	return webPath, nil
}

// downloadBase64 fetches a URL and returns it as a base64 data URI.
// Used for LQIP images that are small enough to inline.
func (d *mediaDownloader) downloadBase64(remoteURL string) (string, error) {
	d.mu.Lock()
	if local, ok := d.cache[remoteURL]; ok {
		d.mu.Unlock()
		return local, nil
	}
	d.mu.Unlock()

	resp, err := d.client.Get(remoteURL)
	if err != nil {
		return "", fmt.Errorf("cms: download %s: %w", remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("cms: download %s: status %d", remoteURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cms: read %s: %w", remoteURL, err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	// Strip parameters (e.g., "image/jpeg; charset=utf-8" → "image/jpeg")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	dataURI := fmt.Sprintf("data:%s;base64,%s", ct, base64.StdEncoding.EncodeToString(body))

	d.mu.Lock()
	d.cache[remoteURL] = dataURI
	d.mu.Unlock()

	return dataURI, nil
}

// hashFilename returns a short deterministic filename from a URL.
// Volatile auth params (sig, exp) are stripped before hashing so the same
// media file + processing params always produce the same filename, even
// when signed URLs are refreshed across rebuilds.
func hashFilename(rawURL string) string {
	h := sha256.Sum256([]byte(stableURL(rawURL)))
	return fmt.Sprintf("%x", h[:8])
}

// stableURL strips volatile auth params (sig, exp) from a URL and returns
// a normalized string. Processing params (w, h, q, format) are kept.
// Query keys are sorted by url.Values.Encode for determinism.
func stableURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Del("sig")
	q.Del("exp")
	u.RawQuery = q.Encode()
	return u.String()
}

// extensionFromResponse determines the file extension from the HTTP response.
func extensionFromResponse(resp *http.Response, url string) string {
	// Try Content-Type header first.
	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		exts, _ := mime.ExtensionsByType(ct)
		if len(exts) > 0 {
			// Prefer common extensions.
			for _, ext := range exts {
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif" || ext == ".gif" || ext == ".svg" {
					return ext
				}
			}
			return exts[0]
		}
	}

	// Fall back to URL path extension.
	path := url
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if ext := filepath.Ext(path); ext != "" {
		return ext
	}

	return ".jpg"
}

// buildLQIPURL constructs the LQIP URL for an image (32px, q20).
func buildLQIPURL(baseURL string) string {
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + "w=32&q=20"
}
