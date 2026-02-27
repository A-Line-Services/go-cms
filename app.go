// Package cms provides a Go framework for building static sites powered
// by the A-Line CMS.
//
// Register pages and collections, then call Run() — the framework handles
// CLI commands (build, serve, sync, dev), static site generation, sync
// payload creation, and dev-mode rebuilding.
package cms

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a-h/templ"
)

// Config holds the connection settings for a CMS site.
type Config struct {
	// APIURL is the base URL of the CMS API (e.g. "https://cms.example.com").
	APIURL string

	// SiteSlug identifies the site within the CMS.
	SiteSlug string

	// APIKey is the X-API-Key used for authentication.
	APIKey string

	// Locale is the default locale for content resolution (default: "en").
	Locale string

	// SiteURL is the public URL of the site (e.g. "https://example.com").
	// When set, Build() generates a sitemap.xml and robots.txt.
	// If empty, the build fetches the domain from the CMS automatically.
	SiteURL string

	// BeforeRebuild is called by the dev server before each rebuild.
	// Use this to reload Vite manifests or other state that may have
	// changed on disk since the last build.
	BeforeRebuild func()
}

// RenderFunc creates a templ Component from page data.
type RenderFunc func(PageData) templ.Component

// EmailVariable describes a runtime variable in an email template.
type EmailVariable struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	SampleValue string `json:"sample_value,omitempty"`
}

// PageOption configures optional behavior for a registered page.
type PageOption func(*pageDef)

// NoSitemap excludes a page from the generated sitemap.
var NoSitemap PageOption = func(p *pageDef) { p.noSitemap = true }

// Priority sets a custom sitemap priority for a page (0.0–1.0).
// If not set, a default is derived from the page type.
func Priority(v float64) PageOption {
	return func(p *pageDef) { p.sitemapPriority = &v }
}

// ChangeFreq sets a custom sitemap change frequency for a page.
// Valid values: "always", "hourly", "daily", "weekly", "monthly", "yearly", "never".
// If not set, a default is derived from the page type.
func ChangeFreq(v string) PageOption {
	return func(p *pageDef) { p.sitemapChangeFreq = v }
}

// pageDef is an internal registration for a fixed page.
type pageDef struct {
	path              string
	title             string
	render            RenderFunc
	noSitemap         bool
	sitemapPriority   *float64
	sitemapChangeFreq string
}

// collectionDef is an internal registration for a collection.
type collectionDef struct {
	basePath    string     // URL prefix (e.g. "/blog")
	key         string     // CMS key (e.g. "blog")
	label       string     // Human-readable label
	listing     RenderFunc // renders the listing/index page
	entry       RenderFunc // renders a single entry
	templateURL string     // auto-generated: basePath + "/_template"
}

// emailTemplateDef is an internal registration for an email template.
type emailTemplateDef struct {
	key       string
	label     string
	subject   string
	html      string
	variables []EmailVariable
}

// LayoutFunc wraps page content in a layout shell. The layout receives
// the page data and a body component to render inside its content slot.
type LayoutFunc func(PageData, templ.Component) templ.Component

// layoutDef is an internal registration for a layout that wraps pages
// under a given path prefix.
type layoutDef struct {
	pathPrefix string     // e.g. "/", "/blog", "/docs"
	id         string     // short identifier, e.g. "root", "blog"
	fn         LayoutFunc // the layout component
}

// App is the main framework entry point. Register pages, collections,
// and email templates, then call Run() to dispatch CLI commands.
type App struct {
	config      Config
	pages       []pageDef
	collections []collectionDef
	emails      []emailTemplateDef
	layouts     []layoutDef
}

// NewApp creates a new App with the given configuration.
// If Locale is empty, the default locale is auto-detected from the CMS
// at build time. Falls back to "en" if the CMS is unreachable.
func NewApp(cfg Config) *App {
	return &App{config: cfg}
}

// Page registers a static page. Title is auto-derived from the path.
func (a *App) Page(path string, render RenderFunc, opts ...PageOption) {
	pd := pageDef{
		path:   path,
		title:  titleFromPath(path),
		render: render,
	}
	for _, o := range opts {
		o(&pd)
	}
	a.pages = append(a.pages, pd)
}

// PageTitle registers a static page with an explicit title.
func (a *App) PageTitle(path, title string, render RenderFunc, opts ...PageOption) {
	pd := pageDef{
		path:   path,
		title:  title,
		render: render,
	}
	for _, o := range opts {
		o(&pd)
	}
	a.pages = append(a.pages, pd)
}

// Collection registers a collection with a listing page and an entry page.
// basePath is the URL prefix (e.g. "/blog").
// The entry template URL is auto-generated as basePath + "/_template".
// The collection key is derived from basePath (e.g. "/blog" → "blog").
func (a *App) Collection(basePath, label string, listing, entry RenderFunc) {
	key := strings.TrimLeft(basePath, "/")
	if idx := strings.Index(key, "/"); idx >= 0 {
		key = key[:idx]
	}
	a.collections = append(a.collections, collectionDef{
		basePath:    basePath,
		key:         key,
		label:       label,
		listing:     listing,
		entry:       entry,
		templateURL: basePath + "/_template",
	})
}

// EmailTemplate registers an email template for sync.
func (a *App) EmailTemplate(key, label, subject, html string, variables []EmailVariable) {
	a.emails = append(a.emails, emailTemplateDef{
		key:       key,
		label:     label,
		subject:   subject,
		html:      html,
		variables: variables,
	})
}

// Layout registers a layout that wraps all pages whose content path
// starts with pathPrefix. Layouts nest automatically: a layout at
// "/blog" is nested inside "/" when both are registered.
//
// The id is a short identifier used for fragment filenames and the
// data-layout attribute in HTML (e.g. "root", "blog").
//
// When layouts are registered, page RenderFuncs should render only
// their own content — layout wrapping is handled by the framework.
// If no layouts are registered, everything works as before (pages
// compose their own layouts manually).
func (a *App) Layout(pathPrefix, id string, fn LayoutFunc) {
	a.layouts = append(a.layouts, layoutDef{
		pathPrefix: pathPrefix,
		id:         id,
		fn:         fn,
	})
}

// hasLayouts reports whether any layouts have been registered.
func (a *App) hasLayouts() bool {
	return len(a.layouts) > 0
}

// layoutManifest returns the pathPrefix→id mapping for all registered layouts.
func (a *App) layoutManifest() map[string]string {
	if len(a.layouts) == 0 {
		return nil
	}
	m := make(map[string]string, len(a.layouts))
	for _, l := range a.layouts {
		m[l.pathPrefix] = l.id
	}
	return m
}

// layoutChain returns the layout chain for a content path, sorted from
// outermost (shortest prefix) to innermost (longest prefix).
func (a *App) layoutChain(contentPath string) []layoutDef {
	var chain []layoutDef
	for _, l := range a.layouts {
		if l.pathPrefix == "/" ||
			contentPath == l.pathPrefix ||
			strings.HasPrefix(contentPath, l.pathPrefix+"/") {
			chain = append(chain, l)
		}
	}
	sort.Slice(chain, func(i, j int) bool {
		return len(chain[i].pathPrefix) < len(chain[j].pathPrefix)
	})
	return chain
}

// composeWithLayouts wraps a content component in the full layout chain
// for the given page, from innermost to outermost.
func (a *App) composeWithLayouts(p PageData, content templ.Component) templ.Component {
	chain := a.layoutChain(p.contentPathOrPath())
	if len(chain) == 0 {
		return content
	}
	result := content
	for i := len(chain) - 1; i >= 0; i-- {
		result = chain[i].fn(p, result)
	}
	return result
}

// composeFragment wraps a content component in layouts deeper than the
// target layout, producing the HTML that goes inside the target's slot.
//
// For a chain [root, blog] with target "root", this returns
// BlogLayout(content). With target "blog", it returns just content.
func (a *App) composeFragment(p PageData, content templ.Component, layoutID string) templ.Component {
	chain := a.layoutChain(p.contentPathOrPath())
	targetIdx := -1
	for i, l := range chain {
		if l.id == layoutID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return content
	}
	result := content
	for i := len(chain) - 1; i > targetIdx; i-- {
		result = chain[i].fn(p, result)
	}
	return result
}

// renderPage finds the appropriate render function for a PageData
// and renders it to an HTML string. When layouts are registered,
// the content is automatically wrapped in the matching layout chain.
func (a *App) renderPage(data PageData) string {
	c := a.findComponent(data)
	if c == nil {
		return ""
	}
	if a.hasLayouts() {
		c = a.composeWithLayouts(data, c)
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return ""
	}
	return buf.String()
}

// renderPageFragment renders only the fragment for a specific layout level.
// The result is the HTML that goes inside [data-layout="<layoutID>"].
func (a *App) renderPageFragment(data PageData, layoutID string) string {
	c := a.findComponent(data)
	if c == nil {
		return ""
	}
	c = a.composeFragment(data, c, layoutID)
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return ""
	}
	return buf.String()
}

// findComponent matches a PageData to its registered render function.
// Uses contentPath (if set) for matching, falling back to Path.
func (a *App) findComponent(data PageData) templ.Component {
	matchPath := data.contentPath
	if matchPath == "" {
		matchPath = data.Path
	}

	// Check fixed pages first.
	for _, p := range a.pages {
		if p.path == matchPath {
			return p.render(data)
		}
	}

	// Check collections.
	for _, c := range a.collections {
		// Template page (for CMS sync crawl).
		if matchPath == c.templateURL {
			return c.entry(data)
		}
		// Listing page.
		if matchPath == c.basePath {
			return c.listing(data)
		}
		// Entry page (any path under basePath/).
		if strings.HasPrefix(matchPath, c.basePath+"/") {
			return c.entry(data)
		}
	}

	return nil
}

// ValidateRoutes checks that scanned filesystem routes match registered
// pages and collections, and vice versa. Returns a list of warnings.
// An empty slice means everything lines up.
func (a *App) ValidateRoutes(routes []ScannedRoute) []string {
	var warnings []string

	// Track which registrations are matched.
	matchedPages := make(map[string]bool)
	matchedCollections := make(map[string]bool)

	for _, r := range routes {
		switch r.Type {
		case TypePage:
			found := false
			for _, p := range a.pages {
				if p.path == r.URLPattern {
					found = true
					matchedPages[p.path] = true
					break
				}
			}
			if !found {
				warnings = append(warnings, fmt.Sprintf(
					"route %s (%s) has no Page() registration", r.URLPattern, r.FilePath))
			}

		case TypeListing:
			found := false
			for _, c := range a.collections {
				if c.basePath == r.URLPattern {
					found = true
					matchedCollections[c.basePath] = true
					break
				}
			}
			if !found {
				warnings = append(warnings, fmt.Sprintf(
					"listing %s (%s) has no Collection() registration", r.URLPattern, r.FilePath))
			}

		case TypeEntry:
			found := false
			for _, c := range a.collections {
				if strings.HasPrefix(r.URLPattern, c.basePath+"/") {
					found = true
					matchedCollections[c.basePath] = true
					break
				}
			}
			if !found {
				warnings = append(warnings, fmt.Sprintf(
					"entry %s (%s) has no Collection() registration", r.URLPattern, r.FilePath))
			}
		}
	}

	// Check for registrations with no matching scanned file.
	for _, p := range a.pages {
		if !matchedPages[p.path] {
			warnings = append(warnings, fmt.Sprintf(
				"Page(%q) has no matching file in pages directory", p.path))
		}
	}
	for _, c := range a.collections {
		if !matchedCollections[c.basePath] {
			warnings = append(warnings, fmt.Sprintf(
				"Collection(%q) has no matching file in pages directory", c.basePath))
		}
	}

	return warnings
}

// testRender creates a RenderFunc from a string-returning function.
// Exported for use in tests and simple integrations.
func testRender(fn func(PageData) string) RenderFunc {
	return func(data PageData) templ.Component {
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			_, err := io.WriteString(w, fn(data))
			return err
		})
	}
}

// ---------------------------------------------------------------------------
// Route scanning
// ---------------------------------------------------------------------------

// RouteType categorizes a discovered route.
type RouteType int

const (
	// TypePage is a static page.
	TypePage RouteType = iota
	// TypeListing is a collection index/listing page (directory also has an entry.templ).
	TypeListing
	// TypeEntry is a dynamic collection entry page.
	TypeEntry
)

// String returns a human-readable name for the route type.
func (t RouteType) String() string {
	switch t {
	case TypePage:
		return "page"
	case TypeListing:
		return "listing"
	case TypeEntry:
		return "entry"
	default:
		return "unknown"
	}
}

// ScannedRoute represents a route discovered from the filesystem.
type ScannedRoute struct {
	// FilePath is the relative path to the .templ file (e.g. "blog/index.templ").
	FilePath string

	// URLPattern is the URL route pattern (e.g. "/blog/:slug").
	URLPattern string

	// Type categorizes the route as page, listing, or entry.
	Type RouteType
}

// ScanRoutes walks a directory of .templ files and derives URL routes
// from the filesystem structure.
//
// Convention:
//   - index.templ         → parent directory path (/ for root)
//   - name.templ          → /name
//   - entry.templ         → /:slug (dynamic entry, TypeEntry)
//   - layout.templ        → ignored (shared layout component)
//   - _name.templ         → ignored (partials, helpers)
//   - non-.templ files    → ignored
//
// When a directory contains both index.templ and entry.templ, the
// index is classified as TypeListing and the entry as TypeEntry.
// The entry URL pattern is the parent directory path + "/:slug".
func ScanRoutes(dir string) ([]ScannedRoute, error) {
	// First pass: identify directories that have an entry.templ file.
	entryDirs := make(map[string]bool)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "entry.templ" {
			relDir, _ := filepath.Rel(dir, filepath.Dir(path))
			entryDirs[filepath.ToSlash(relDir)] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Second pass: build routes from .templ files.
	var routes []ScannedRoute

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".templ" {
			return nil
		}

		name := d.Name()

		// Skip underscore-prefixed files (partials, helpers).
		if strings.HasPrefix(name, "_") {
			return nil
		}
		// Skip layout.templ (shared layout component, not a route).
		if name == "layout.templ" {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)

		relDir, _ := filepath.Rel(dir, filepath.Dir(path))
		relDir = filepath.ToSlash(relDir)

		baseName := strings.TrimSuffix(name, ".templ")

		var urlPattern string
		var routeType RouteType

		switch {
		case baseName == "index":
			// index.templ → parent directory path.
			urlPattern = dirToURL(relDir)
			if urlPattern == "" {
				urlPattern = "/"
			}
			if entryDirs[relDir] {
				// Directory also has entry.templ → this index is a listing.
				routeType = TypeListing
			} else {
				routeType = TypePage
			}

		case baseName == "entry":
			// entry.templ → dynamic entry route.
			urlPattern = dirToURL(relDir) + "/:slug"
			routeType = TypeEntry

		default:
			// name.templ → /name
			urlPattern = dirToURL(relDir) + "/" + baseName
			routeType = TypePage
		}

		routes = append(routes, ScannedRoute{
			FilePath:   rel,
			URLPattern: urlPattern,
			Type:       routeType,
		})
		return nil
	})

	return routes, err
}

// dirToURL converts a relative directory path to a URL prefix.
// "." → "", "blog" → "/blog", "blog/posts" → "/blog/posts".
func dirToURL(relDir string) string {
	if relDir == "." || relDir == "" {
		return ""
	}
	return "/" + relDir
}

// titleFromPath derives a human-readable title from a URL path.
// "/" → "Home", "/about" → "About", "/contact-us" → "Contact Us".
func titleFromPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "Home"
	}
	parts := strings.Split(trimmed, "/")
	last := parts[len(parts)-1]
	words := strings.Split(last, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
