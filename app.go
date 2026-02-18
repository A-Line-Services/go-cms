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
}

// RenderFunc creates a templ Component from page data.
type RenderFunc func(PageData) templ.Component

// EmailVariable describes a runtime variable in an email template.
type EmailVariable struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	SampleValue string `json:"sample_value,omitempty"`
}

// pageDef is an internal registration for a fixed page.
type pageDef struct {
	path   string
	title  string
	render RenderFunc
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

// App is the main framework entry point. Register pages, collections,
// and email templates, then call Run() to dispatch CLI commands.
type App struct {
	config      Config
	pages       []pageDef
	collections []collectionDef
	emails      []emailTemplateDef
}

// NewApp creates a new App with the given configuration.
func NewApp(cfg Config) *App {
	if cfg.Locale == "" {
		cfg.Locale = "en"
	}
	return &App{config: cfg}
}

// Page registers a static page. Title is auto-derived from the path.
func (a *App) Page(path string, render RenderFunc) {
	a.pages = append(a.pages, pageDef{
		path:   path,
		title:  titleFromPath(path),
		render: render,
	})
}

// PageTitle registers a static page with an explicit title.
func (a *App) PageTitle(path, title string, render RenderFunc) {
	a.pages = append(a.pages, pageDef{
		path:   path,
		title:  title,
		render: render,
	})
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

// renderPage finds the appropriate render function for a PageData
// and renders it to an HTML string.
func (a *App) renderPage(data PageData) string {
	c := a.findComponent(data)
	if c == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return ""
	}
	return buf.String()
}

// findComponent matches a PageData to its registered render function.
func (a *App) findComponent(data PageData) templ.Component {
	// Check fixed pages first.
	for _, p := range a.pages {
		if p.path == data.Path {
			return p.render(data)
		}
	}

	// Check collections.
	for _, c := range a.collections {
		// Template page (for CMS sync crawl).
		if data.Path == c.templateURL {
			return c.entry(data)
		}
		// Listing page.
		if data.Path == c.basePath {
			return c.listing(data)
		}
		// Entry page (any path under basePath/).
		if strings.HasPrefix(data.Path, c.basePath+"/") {
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
