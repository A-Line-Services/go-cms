package cms

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// sitemapData holds collected URLs grouped for sitemap generation.
type sitemapData struct {
	siteURL     string
	pages       []string            // fixed page paths
	collections map[string][]string // collKey → entry paths
}

// collectSitemapURLs gathers all URLs that should appear in the sitemap.
// It uses the registered pages/collections and the fetched CMS pages to
// determine the final URL list. Pages with noSitemap, template pages, and
// error pages (/404, /500) are excluded.
func (a *App) collectSitemapURLs(allPages []apiPageListItem, locales []SiteLocale, defaultLocale string) *sitemapData {
	sd := &sitemapData{
		siteURL:     strings.TrimRight(a.config.SiteURL, "/"),
		collections: make(map[string][]string),
	}

	multiLocale := len(locales) > 1

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
		if multiLocale {
			for _, li := range localeInfos {
				sd.pages = append(sd.pages, localePrefixPath(li.prefix, p.path))
			}
		} else {
			sd.pages = append(sd.pages, p.path)
		}
	}

	// Collection listing pages.
	for _, c := range a.collections {
		if isErrorPage(c.basePath) {
			continue
		}
		if multiLocale {
			for _, li := range localeInfos {
				sd.pages = append(sd.pages, localePrefixPath(li.prefix, c.basePath))
			}
		} else {
			sd.pages = append(sd.pages, c.basePath)
		}
	}

	// Collection entry pages (from CMS-published pages).
	if allPages != nil {
		for _, c := range a.collections {
			var entryPaths []string
			for _, ap := range allPages {
				if !strings.HasPrefix(ap.Path, c.basePath+"/") {
					continue
				}
				// Skip the template page.
				if ap.Path == c.templateURL {
					continue
				}
				if multiLocale {
					for _, li := range localeInfos {
						entryPaths = append(entryPaths, localePrefixPath(li.prefix, ap.Path))
					}
				} else {
					entryPaths = append(entryPaths, ap.Path)
				}
			}
			if len(entryPaths) > 0 {
				sd.collections[c.key] = entryPaths
			}
		}
	}

	return sd
}

// isErrorPage returns true for paths whose last segment is an HTTP error code.
func isErrorPage(path string) bool {
	base := filepath.Base(strings.TrimRight(path, "/"))
	return base == "404" || base == "500"
}

// ---------------------------------------------------------------------------
// Sitemap writing
// ---------------------------------------------------------------------------

// writeSitemap generates sitemap file(s) in the output directory.
// If the total URL count fits in a single file (≤50,000), a single
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
		for _, p := range sd.pages {
			urls = append(urls, sd.makeSitemapURL(p, locales))
		}
		for _, entries := range sd.collections {
			for _, p := range entries {
				urls = append(urls, sd.makeSitemapURL(p, locales))
			}
		}
		return writeURLSet(filepath.Join(outDir, "sitemap.xml"), urls, multiLocale)
	}

	// Complex case: sitemap index with sub-sitemaps.
	var indexEntries []sitemapEntry

	// Pages sitemap.
	if len(sd.pages) > 0 {
		var urls []sitemapURL
		for _, p := range sd.pages {
			urls = append(urls, sd.makeSitemapURL(p, locales))
		}
		if err := writeURLSet(filepath.Join(outDir, "sitemap-pages.xml"), urls, multiLocale); err != nil {
			return err
		}
		indexEntries = append(indexEntries, sitemapEntry{Loc: sd.siteURL + "/sitemap-pages.xml"})
	}

	// Per-collection sitemaps, split if > 50,000 entries.
	for key, entries := range sd.collections {
		chunks := chunkStrings(entries, maxSitemapURLs)
		for i, chunk := range chunks {
			var urls []sitemapURL
			for _, p := range chunk {
				urls = append(urls, sd.makeSitemapURL(p, locales))
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

// makeSitemapURL creates a sitemapURL with optional hreflang alternates.
func (sd *sitemapData) makeSitemapURL(path string, locales []SiteLocale) sitemapURL {
	u := sitemapURL{Loc: sd.siteURL + path}
	if len(locales) > 1 {
		// Determine the content path (without locale prefix) so we can
		// build alternate URLs for all locales.
		contentPath := path
		for _, l := range locales {
			prefix := "/" + l.Code
			if strings.HasPrefix(path, prefix+"/") || path == prefix {
				if path == prefix {
					contentPath = "/"
				} else {
					contentPath = strings.TrimPrefix(path, prefix)
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
