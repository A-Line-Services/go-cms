package cms

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Sitemap XML types
// ---------------------------------------------------------------------------

const (
	sitemapNS      = "http://www.sitemaps.org/schemas/sitemap/0.9"
	xhtmlNS        = "http://www.w3.org/1999/xhtml"
	maxSitemapURLs = 50_000
)

type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	NS       string         `xml:"xmlns,attr"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

type sitemapEntry struct {
	Loc string `xml:"loc"`
}

type urlSet struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	XHTML   string       `xml:"xmlns:xhtml,attr,omitempty"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc        string         `xml:"loc"`
	LastMod    string         `xml:"lastmod,omitempty"`
	ChangeFreq string         `xml:"changefreq,omitempty"`
	Priority   string         `xml:"priority,omitempty"`
	Alternates []sitemapXHTML `xml:"xhtml:link,omitempty"`
}

type sitemapXHTML struct {
	Rel      string `xml:"rel,attr"`
	Hreflang string `xml:"hreflang,attr"`
	Href     string `xml:"href,attr"`
}

// ---------------------------------------------------------------------------
// URL collection
// ---------------------------------------------------------------------------

// sitemapURLEntry holds a URL path and its metadata for sitemap generation.
type sitemapURLEntry struct {
	path       string
	lastMod    string
	changeFreq string
	priority   string
}

// sitemapData holds collected URLs grouped for sitemap generation.
type sitemapData struct {
	siteURL       string
	pages         []sitemapURLEntry
	collections   map[string][]sitemapURLEntry
	defaultLocale string
}

// collectSitemapURLs gathers all URLs that should appear in the sitemap.
// It uses the registered pages/collections and the fetched CMS pages to
// determine the final URL list. Pages with noSitemap, template pages, and
// error pages (/404, /500) are excluded.
func (a *App) collectSitemapURLs(allPages []apiPageListItem, locales []SiteLocale, defaultLocale string) *sitemapData {
	sd := &sitemapData{
		siteURL:       strings.TrimRight(a.config.SiteURL, "/"),
		collections:   make(map[string][]sitemapURLEntry),
		defaultLocale: defaultLocale,
	}

	multiLocale := len(locales) > 1
	buildDate := time.Now().Format("2006-01-02")

	// Build a map of CMS page updated_at by path for lastmod.
	pageUpdated := make(map[string]string)
	for _, ap := range allPages {
		if ap.UpdatedAt != nil && *ap.UpdatedAt != "" {
			// Parse and format to date-only.
			if t, err := time.Parse(time.RFC3339, *ap.UpdatedAt); err == nil {
				pageUpdated[ap.Path] = t.Format("2006-01-02")
			} else if t, err := time.Parse(time.RFC3339Nano, *ap.UpdatedAt); err == nil {
				pageUpdated[ap.Path] = t.Format("2006-01-02")
			}
		}
	}

	// Determine locale prefixes.
	type localeInfo struct {
		code   string
		prefix string
	}
	var localeInfos []localeInfo
	if multiLocale {
		for _, l := range locales {
			localeInfos = append(localeInfos, localeInfo{code: l.Code, prefix: "/" + l.Code})
		}
	}

	// Fixed pages.
	for _, p := range a.pages {
		if p.noSitemap || isErrorPage(p.path) {
			continue
		}

		// Determine priority and changefreq.
		pri := defaultPagePriority(p.path)
		freq := "weekly"
		if p.path == "/" {
			freq = "daily"
		}
		if p.sitemapPriority != nil {
			pri = *p.sitemapPriority
		}
		if p.sitemapChangeFreq != "" {
			freq = p.sitemapChangeFreq
		}

		priStr := formatPriority(pri)
		lastMod := buildDate

		// Use CMS updated_at if available for this page.
		if d, ok := pageUpdated[p.path]; ok {
			lastMod = d
		}

		if multiLocale {
			// Default locale uses the unprefixed (root) path as canonical;
			// non-default locales use their prefixed path.
			for _, li := range localeInfos {
				entryPath := localePrefixPath(li.prefix, p.path)
				if li.code == defaultLocale {
					entryPath = p.path
				}
				sd.pages = append(sd.pages, sitemapURLEntry{
					path:       entryPath,
					lastMod:    lastMod,
					changeFreq: freq,
					priority:   priStr,
				})
			}
		} else {
			sd.pages = append(sd.pages, sitemapURLEntry{
				path:       p.path,
				lastMod:    lastMod,
				changeFreq: freq,
				priority:   priStr,
			})
		}
	}

	// Collection listing pages.
	for _, c := range a.collections {
		if isErrorPage(c.basePath) {
			continue
		}
		priStr := formatPriority(0.7)
		lastMod := buildDate

		if multiLocale {
			for _, li := range localeInfos {
				entryPath := localePrefixPath(li.prefix, c.basePath)
				if li.code == defaultLocale {
					entryPath = c.basePath
				}
				sd.pages = append(sd.pages, sitemapURLEntry{
					path:       entryPath,
					lastMod:    lastMod,
					changeFreq: "weekly",
					priority:   priStr,
				})
			}
		} else {
			sd.pages = append(sd.pages, sitemapURLEntry{
				path:       c.basePath,
				lastMod:    lastMod,
				changeFreq: "weekly",
				priority:   priStr,
			})
		}
	}

	// Collection entry pages (from CMS-published pages).
	if allPages != nil {
		for _, c := range a.collections {
			var entryPaths []sitemapURLEntry
			for _, ap := range allPages {
				if !strings.HasPrefix(ap.Path, c.basePath+"/") {
					continue
				}
				// Skip the template page.
				if ap.Path == c.templateURL {
					continue
				}

				lastMod := buildDate
				if d, ok := pageUpdated[ap.Path]; ok {
					lastMod = d
				}

				if multiLocale {
					for _, li := range localeInfos {
						entryPath := localePrefixPath(li.prefix, ap.Path)
						if li.code == defaultLocale {
							entryPath = ap.Path
						}
						entryPaths = append(entryPaths, sitemapURLEntry{
							path:       entryPath,
							lastMod:    lastMod,
							changeFreq: "weekly",
							priority:   formatPriority(0.6),
						})
					}
				} else {
					entryPaths = append(entryPaths, sitemapURLEntry{
						path:       ap.Path,
						lastMod:    lastMod,
						changeFreq: "weekly",
						priority:   formatPriority(0.6),
					})
				}
			}
			if len(entryPaths) > 0 {
				sd.collections[c.key] = entryPaths
			}
		}
	}

	return sd
}

// defaultPagePriority returns a sensible default priority for a fixed page.
func defaultPagePriority(path string) float64 {
	if path == "/" {
		return 1.0
	}
	// Top-level pages get higher priority than deeply nested ones.
	depth := strings.Count(strings.Trim(path, "/"), "/")
	if depth == 0 {
		return 0.8
	}
	return 0.7
}

// formatPriority converts a float to a sitemap priority string.
func formatPriority(p float64) string {
	if p == 1.0 {
		return "1.0"
	}
	return fmt.Sprintf("%.1f", p)
}

// isErrorPage returns true for paths whose last segment is an HTTP error code.
func isErrorPage(path string) bool {
	base := filepath.Base(strings.TrimRight(path, "/"))
	return base == "404" || base == "500"
}

// ---------------------------------------------------------------------------
// Sitemap writing
// ---------------------------------------------------------------------------

// write generates sitemap file(s) in the output directory.
// If the total URL count fits in a single file (<=50,000), a single
// sitemap.xml is written. Otherwise a sitemap index is used with
// separate files for pages and each collection.
func (sd *sitemapData) write(outDir string, locales []SiteLocale) error {
	totalURLs := len(sd.pages)
	for _, entries := range sd.collections {
		totalURLs += len(entries)
	}

	multiLocale := len(locales) > 1

	// Simple case: everything fits in one file.
	if len(sd.collections) == 0 || (totalURLs <= maxSitemapURLs && len(sd.collections) <= 1) {
		var urls []sitemapURL
		for _, e := range sd.pages {
			urls = append(urls, sd.makeSitemapURL(e, locales))
		}
		for _, entries := range sd.collections {
			for _, e := range entries {
				urls = append(urls, sd.makeSitemapURL(e, locales))
			}
		}
		return writeURLSet(filepath.Join(outDir, "sitemap.xml"), urls, multiLocale)
	}

	// Complex case: sitemap index with sub-sitemaps.
	var indexEntries []sitemapEntry

	// Pages sitemap.
	if len(sd.pages) > 0 {
		var urls []sitemapURL
		for _, e := range sd.pages {
			urls = append(urls, sd.makeSitemapURL(e, locales))
		}
		if err := writeURLSet(filepath.Join(outDir, "sitemap-pages.xml"), urls, multiLocale); err != nil {
			return err
		}
		indexEntries = append(indexEntries, sitemapEntry{Loc: sd.siteURL + "/sitemap-pages.xml"})
	}

	// Per-collection sitemaps, split if > 50,000 entries.
	for key, entries := range sd.collections {
		chunks := chunkEntries(entries, maxSitemapURLs)
		for i, chunk := range chunks {
			var urls []sitemapURL
			for _, e := range chunk {
				urls = append(urls, sd.makeSitemapURL(e, locales))
			}

			filename := fmt.Sprintf("sitemap-%s.xml", key)
			if len(chunks) > 1 {
				filename = fmt.Sprintf("sitemap-%s-%d.xml", key, i+1)
			}

			if err := writeURLSet(filepath.Join(outDir, filename), urls, multiLocale); err != nil {
				return err
			}
			indexEntries = append(indexEntries, sitemapEntry{Loc: sd.siteURL + "/" + filename})
		}
	}

	// Write sitemap index.
	idx := sitemapIndex{NS: sitemapNS, Sitemaps: indexEntries}
	return writeXML(filepath.Join(outDir, "sitemap.xml"), idx)
}

// makeSitemapURL creates a sitemapURL with metadata and optional hreflang alternates.
func (sd *sitemapData) makeSitemapURL(entry sitemapURLEntry, locales []SiteLocale) sitemapURL {
	u := sitemapURL{
		Loc:        sd.siteURL + entry.path,
		LastMod:    entry.lastMod,
		ChangeFreq: entry.changeFreq,
		Priority:   entry.priority,
	}
	if len(locales) > 1 {
		// Determine the content path (without locale prefix) so we can
		// build alternate URLs for all locales.
		contentPath := entry.path
		for _, l := range locales {
			prefix := "/" + l.Code
			if entry.path == prefix || strings.HasPrefix(entry.path, prefix+"/") {
				if entry.path == prefix {
					contentPath = "/"
				} else {
					contentPath = strings.TrimPrefix(entry.path, prefix)
				}
				break
			}
		}

		for _, l := range locales {
			altPath := localePrefixPath("/"+l.Code, contentPath)
			u.Alternates = append(u.Alternates, sitemapXHTML{
				Rel:      "alternate",
				Hreflang: l.Code,
				Href:     sd.siteURL + altPath,
			})
		}

		// x-default points to the unprefixed (default locale) URL.
		xDefaultPath := contentPath
		u.Alternates = append(u.Alternates, sitemapXHTML{
			Rel:      "alternate",
			Hreflang: "x-default",
			Href:     sd.siteURL + xDefaultPath,
		})
	}
	return u
}

// ---------------------------------------------------------------------------
// robots.txt
// ---------------------------------------------------------------------------

// writeRobotsTxt generates a robots.txt with a Sitemap reference.
func writeRobotsTxt(outDir, siteURL string) error {
	content := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n",
		strings.TrimRight(siteURL, "/"))
	return os.WriteFile(filepath.Join(outDir, "robots.txt"), []byte(content), 0o644)
}

// ---------------------------------------------------------------------------
// XML helpers
// ---------------------------------------------------------------------------

func writeURLSet(path string, urls []sitemapURL, multiLocale bool) error {
	us := urlSet{NS: sitemapNS, URLs: urls}
	if multiLocale {
		us.XHTML = xhtmlNS
	}
	return writeXML(path, us)
}

func writeXML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("cms: marshal sitemap: %w", err)
	}
	content := xml.Header + string(data) + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func chunkEntries(s []sitemapURLEntry, size int) [][]sitemapURLEntry {
	if len(s) <= size {
		return [][]sitemapURLEntry{s}
	}
	var chunks [][]sitemapURLEntry
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// chunkStrings splits a string slice into chunks of the given size.
func chunkStrings(s []string, size int) [][]string {
	if len(s) <= size {
		return [][]string{s}
	}
	var chunks [][]string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}
