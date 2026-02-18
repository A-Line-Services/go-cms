package cms

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	if opts.DownloadMedia {
		mediaDir := filepath.Join(opts.OutDir, "media")
		dl := newMediaDownloader(mediaDir, "/media")
		imgProc = dl.processor()
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

	// 1. List all published pages (single call, needed to discover entries).
	allPages, listErr := client.ListPages(ctx)
	if listErr != nil {
		allPages = nil
	}

	// 2. Plan all pages to fetch.
	jobs, seen := a.planFetchJobs(allPages)

	// 3. Fetch all page content + SEO concurrently.
	results := a.fetchAll(ctx, client, jobs, imgProc)

	// 4. Assemble listings from entry results.
	listings := make(map[string][]PageData)
	for _, r := range results {
		if r.job.collKey != "" {
			listings[r.job.collKey] = append(listings[r.job.collKey], r.page)
		}
	}

	// 5. Write all pages.
	for _, r := range results {
		page := r.page

		// Attach listings to non-entry, non-template pages.
		if r.job.collKey == "" && !r.job.isTemplate && len(listings) > 0 {
			page.listings = listings
		}

		if err := a.writePage(opts, m, page); err != nil {
			return err
		}
	}

	// Write sync payload if requested.
	if opts.SyncFile != "" {
		if err := a.WriteSyncJSON(opts.SyncFile); err != nil {
			return fmt.Errorf("cms: write sync file: %w", err)
		}
	}

	_ = seen // used during planning to deduplicate
	return nil
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

// fetchAll fetches content + SEO for all jobs concurrently, up to 10 at a time.
func (a *App) fetchAll(ctx context.Context, client *Client, jobs []fetchJob, imgProc imageProcessor) []fetchResult {
	results := make([]fetchResult, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job fetchJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			page, err := client.GetPage(ctx, job.path)
			if err != nil {
				if !job.isTemplate {
					fmt.Fprintf(os.Stderr, "  [warn] %s: no CMS content, using fallbacks (%v)\n", job.path, err)
				}
				page = NewPageData(job.path, job.slug, a.config.Locale, nil, nil, nil)
			} else {
				fmt.Fprintf(os.Stderr, "  [ok]   %s: fetched CMS content\n", job.path)
			}

			seo, seoErr := client.GetSEO(ctx, job.path)
			if seoErr == nil {
				page.seo = &seo
			}

			if imgProc != nil {
				page.imgProc = imgProc
				setEntryImageProcessor(page.subcollections, imgProc)
			}

			results[i] = fetchResult{job: job, page: page}
		}(i, job)
	}

	wg.Wait()
	return results
}

// buildOnePage fetches content + SEO for a single path, renders it, and writes
// the HTML file. Falls back to empty PageData if the API is unavailable.
func (a *App) buildOnePage(ctx context.Context, client *Client, opts BuildOptions, pagePath string, imgProc imageProcessor, m *minify.M, listings map[string][]PageData) error {
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

	// Attach collection listings so index pages can iterate entries.
	if len(listings) > 0 {
		page.listings = listings
	}

	return a.writePage(opts, m, page)
}

// writePage renders a PageData, optionally minifies, and writes the HTML file.
func (a *App) writePage(opts BuildOptions, m *minify.M, page PageData) error {
	output := a.renderPage(page)

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
	return nil
}

// pathToFile converts a URL path to a filesystem path.
// "/" → "{outDir}/index.html"
// "/about" → "{outDir}/about/index.html"
// "/blog/post" → "{outDir}/blog/post/index.html"
func pathToFile(outDir, urlPath string) string {
	trimmed := strings.Trim(urlPath, "/")
	if trimmed == "" {
		return filepath.Join(outDir, "index.html")
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
