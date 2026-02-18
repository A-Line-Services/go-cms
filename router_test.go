package cms

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeTemplFile creates a dummy .templ file in the temp directory.
func writeTemplFile(t *testing.T, base, relPath string) {
	t.Helper()
	full := filepath.Join(base, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("package pages\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findRoute returns the first route matching the given URL pattern, or nil.
func findRoute(routes []ScannedRoute, pattern string) *ScannedRoute {
	for i := range routes {
		if routes[i].URLPattern == pattern {
			return &routes[i]
		}
	}
	return nil
}

// --- Root index ---

func TestScanRoutes_RootIndex(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "index.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}

	r := routes[0]
	if r.URLPattern != "/" {
		t.Errorf("URLPattern = %q, want /", r.URLPattern)
	}
	if r.Type != TypePage {
		t.Errorf("Type = %v, want TypePage", r.Type)
	}
	if r.FilePath != "index.templ" {
		t.Errorf("FilePath = %q, want index.templ", r.FilePath)
	}
}

// --- Named file at root ---

func TestScanRoutes_NamedFile(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "about.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}

	if routes[0].URLPattern != "/about" {
		t.Errorf("URLPattern = %q, want /about", routes[0].URLPattern)
	}
	if routes[0].Type != TypePage {
		t.Errorf("Type = %v, want TypePage", routes[0].Type)
	}
}

// --- Nested index (plain directory, no entry.templ) ---

func TestScanRoutes_NestedIndex(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "docs/index.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}

	if routes[0].URLPattern != "/docs" {
		t.Errorf("URLPattern = %q, want /docs", routes[0].URLPattern)
	}
	if routes[0].Type != TypePage {
		t.Errorf("Type = %v, want TypePage", routes[0].Type)
	}
}

// --- Nested named file ---

func TestScanRoutes_NestedNamedFile(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "docs/getting-started.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}

	if routes[0].URLPattern != "/docs/getting-started" {
		t.Errorf("URLPattern = %q, want /docs/getting-started", routes[0].URLPattern)
	}
	if routes[0].Type != TypePage {
		t.Errorf("Type = %v, want TypePage", routes[0].Type)
	}
}

// --- entry.templ alone → TypeEntry ---

func TestScanRoutes_EntryOnly(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "blog/entry.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}

	if routes[0].URLPattern != "/blog/:slug" {
		t.Errorf("URLPattern = %q, want /blog/:slug", routes[0].URLPattern)
	}
	if routes[0].Type != TypeEntry {
		t.Errorf("Type = %v, want TypeEntry", routes[0].Type)
	}
	if routes[0].FilePath != "blog/entry.templ" {
		t.Errorf("FilePath = %q, want blog/entry.templ", routes[0].FilePath)
	}
}

// --- Listing detected: index.templ + entry.templ in same directory ---

func TestScanRoutes_ListingWithEntry(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "blog/index.templ")
	writeTemplFile(t, dir, "blog/entry.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(routes))
	}

	listing := findRoute(routes, "/blog")
	if listing == nil {
		t.Fatal("no /blog route found")
	}
	if listing.Type != TypeListing {
		t.Errorf("/blog Type = %v, want TypeListing", listing.Type)
	}

	entry := findRoute(routes, "/blog/:slug")
	if entry == nil {
		t.Fatal("no /blog/:slug route found")
	}
	if entry.Type != TypeEntry {
		t.Errorf("/blog/:slug Type = %v, want TypeEntry", entry.Type)
	}
}

// --- Underscored files are ignored ---

func TestScanRoutes_IgnoresUnderscored(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "_layout.templ")
	writeTemplFile(t, dir, "index.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}
	if routes[0].URLPattern != "/" {
		t.Errorf("URLPattern = %q, want /", routes[0].URLPattern)
	}
}

// --- Non-.templ files are ignored ---

func TestScanRoutes_IgnoresNonTemplFiles(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "style.css")
	writeTemplFile(t, dir, "index.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}
}

// --- Empty directory → no routes ---

func TestScanRoutes_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 0 {
		t.Fatalf("got %d routes, want 0", len(routes))
	}
}

// --- Hyphenated filenames ---

func TestScanRoutes_HyphenatedFilename(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "my-cool-page.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}
	if routes[0].URLPattern != "/my-cool-page" {
		t.Errorf("URLPattern = %q, want /my-cool-page", routes[0].URLPattern)
	}
}

// --- Deeply nested entry ---

func TestScanRoutes_NestedEntry(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "docs/tutorials/index.templ")
	writeTemplFile(t, dir, "docs/tutorials/entry.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(routes))
	}

	listing := findRoute(routes, "/docs/tutorials")
	if listing == nil {
		t.Fatal("no /docs/tutorials route found")
	}
	if listing.Type != TypeListing {
		t.Errorf("Type = %v, want TypeListing", listing.Type)
	}

	entry := findRoute(routes, "/docs/tutorials/:slug")
	if entry == nil {
		t.Fatal("no /docs/tutorials/:slug route found")
	}
	if entry.Type != TypeEntry {
		t.Errorf("Type = %v, want TypeEntry", entry.Type)
	}
}

// --- Multiple collections ---

func TestScanRoutes_MultipleCollections(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "blog/index.templ")
	writeTemplFile(t, dir, "blog/entry.templ")
	writeTemplFile(t, dir, "products/index.templ")
	writeTemplFile(t, dir, "products/entry.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 4 {
		t.Fatalf("got %d routes, want 4", len(routes))
	}

	if r := findRoute(routes, "/blog"); r == nil || r.Type != TypeListing {
		t.Errorf("/blog: %+v", r)
	}
	if r := findRoute(routes, "/blog/:slug"); r == nil || r.Type != TypeEntry {
		t.Errorf("/blog/:slug: %+v", r)
	}
	if r := findRoute(routes, "/products"); r == nil || r.Type != TypeListing {
		t.Errorf("/products: %+v", r)
	}
	if r := findRoute(routes, "/products/:slug"); r == nil || r.Type != TypeEntry {
		t.Errorf("/products/:slug: %+v", r)
	}
}

// --- Full realistic structure ---

func TestScanRoutes_ComplexStructure(t *testing.T) {
	dir := t.TempDir()
	writeTemplFile(t, dir, "index.templ")
	writeTemplFile(t, dir, "about.templ")
	writeTemplFile(t, dir, "contact.templ")
	writeTemplFile(t, dir, "_layout.templ")
	writeTemplFile(t, dir, "blog/index.templ")
	writeTemplFile(t, dir, "blog/entry.templ")
	writeTemplFile(t, dir, "docs/index.templ")
	writeTemplFile(t, dir, "docs/getting-started.templ")

	routes, err := ScanRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].URLPattern < routes[j].URLPattern
	})

	expected := []struct {
		pattern string
		typ     RouteType
	}{
		{"/", TypePage},
		{"/about", TypePage},
		{"/blog", TypeListing},
		{"/blog/:slug", TypeEntry},
		{"/contact", TypePage},
		{"/docs", TypePage},
		{"/docs/getting-started", TypePage},
	}

	if len(routes) != len(expected) {
		for _, r := range routes {
			t.Logf("  %s (%v) [%s]", r.URLPattern, r.Type, r.FilePath)
		}
		t.Fatalf("got %d routes, want %d", len(routes), len(expected))
	}

	for i, exp := range expected {
		if routes[i].URLPattern != exp.pattern {
			t.Errorf("route[%d].URLPattern = %q, want %q", i, routes[i].URLPattern, exp.pattern)
		}
		if routes[i].Type != exp.typ {
			t.Errorf("route[%d].Type = %v, want %v (for %s)", i, routes[i].Type, exp.typ, exp.pattern)
		}
	}
}

// --- Directory that doesn't exist ---

func TestScanRoutes_NonexistentDir(t *testing.T) {
	_, err := ScanRoutes("/tmp/nonexistent-dir-that-should-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}
