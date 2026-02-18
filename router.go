package cms

import (
	"os"
	"path/filepath"
	"strings"
)

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
