package cms

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNewApp_CreatesConfiguredApp(t *testing.T) {
	cfg := Config{
		APIURL:   "https://cms.example.com",
		SiteSlug: "my-site",
		APIKey:   "test-key",
		Locale:   "en",
	}
	app := NewApp(cfg)

	if app.config.APIURL != "https://cms.example.com" {
		t.Errorf("APIURL = %q", app.config.APIURL)
	}
	if app.config.SiteSlug != "my-site" {
		t.Errorf("SiteSlug = %q", app.config.SiteSlug)
	}
	if app.config.APIKey != "test-key" {
		t.Errorf("APIKey = %q", app.config.APIKey)
	}
	if app.config.Locale != "en" {
		t.Errorf("Locale = %q", app.config.Locale)
	}
}

func TestNewApp_DefaultLocale(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	if app.config.Locale != "en" {
		t.Errorf("default Locale = %q, want 'en'", app.config.Locale)
	}
}

func TestApp_Page_RegistersPage(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return "home" }))
	app.Page("/about", testRender(func(p PageData) string { return "about" }))

	if len(app.pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(app.pages))
	}
	if app.pages[0].path != "/" {
		t.Errorf("page[0].path = %q", app.pages[0].path)
	}
	if app.pages[0].title != "Home" {
		t.Errorf("page[0].title = %q, want Home (auto-derived)", app.pages[0].title)
	}
	if app.pages[1].path != "/about" || app.pages[1].title != "About" {
		t.Errorf("page[1] = {path: %q, title: %q}", app.pages[1].path, app.pages[1].title)
	}
}

func TestApp_PageTitle_ExplicitTitle(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.PageTitle("/", "Homepage", testRender(func(p PageData) string { return "" }))

	if app.pages[0].title != "Homepage" {
		t.Errorf("title = %q, want Homepage", app.pages[0].title)
	}
}

func TestApp_Collection_RegistersCollection(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	noop := testRender(func(p PageData) string { return "" })
	app.Collection("/blog", "Blog Posts", noop, noop)

	if len(app.collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(app.collections))
	}
	c := app.collections[0]
	if c.basePath != "/blog" {
		t.Errorf("basePath = %q", c.basePath)
	}
	if c.key != "blog" {
		t.Errorf("key = %q, want 'blog'", c.key)
	}
	if c.label != "Blog Posts" {
		t.Errorf("label = %q", c.label)
	}
	if c.templateURL != "/blog/_template" {
		t.Errorf("templateURL = %q, want /blog/_template", c.templateURL)
	}
}

func TestApp_Collection_KeyDerived(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	noop := testRender(func(p PageData) string { return "" })

	app.Collection("/docs/articles", "Articles", noop, noop)

	// Key is derived from the first path segment after /
	if app.collections[0].key != "docs" {
		t.Errorf("key = %q, want 'docs'", app.collections[0].key)
	}
}

func TestApp_EmailTemplate_Registers(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.EmailTemplate("welcome", "Welcome Email", "Welcome!", "<h1>Hi</h1>", nil)

	if len(app.emails) != 1 {
		t.Fatalf("expected 1 email template, got %d", len(app.emails))
	}
	et := app.emails[0]
	if et.key != "welcome" || et.label != "Welcome Email" {
		t.Errorf("emailTemplate = %+v", et)
	}
	if et.subject != "Welcome!" {
		t.Errorf("Subject = %q", et.subject)
	}
}

func TestApp_RenderPage_FixedPage(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return "home:" + p.Text("title") }))
	app.Page("/about", testRender(func(p PageData) string { return "about:" + p.Text("title") }))

	data := NewPageData("/about", "about", "en", map[string]any{"title": "About Us"}, nil, nil)
	html := app.renderPage(data)
	if html != "about:About Us" {
		t.Errorf("renderPage = %q", html)
	}
}

func TestApp_RenderPage_CollectionEntry(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing" }),
		testRender(func(p PageData) string { return "entry:" + p.Text("title") }),
	)

	data := NewPageData("/blog/my-post", "my-post", "en", map[string]any{"title": "My Post"}, nil, nil)
	html := app.renderPage(data)
	if html != "entry:My Post" {
		t.Errorf("renderPage = %q", html)
	}
}

func TestApp_RenderPage_CollectionListing(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing" }),
		testRender(func(p PageData) string { return "entry" }),
	)

	data := NewPageData("/blog", "blog", "en", nil, nil, nil)
	html := app.renderPage(data)
	if html != "listing" {
		t.Errorf("renderPage = %q, want listing", html)
	}
}

func TestApp_RenderPage_CollectionTemplate(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing" }),
		testRender(func(p PageData) string { return "entry-template" }),
	)

	data := NewPageData("/blog/_template", "template", "en", nil, nil, nil)
	html := app.renderPage(data)
	if html != "entry-template" {
		t.Errorf("renderPage = %q, want entry-template", html)
	}
}

func TestApp_RenderPage_UnknownPath(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	data := NewPageData("/unknown", "unknown", "en", nil, nil, nil)
	html := app.renderPage(data)
	if html != "" {
		t.Errorf("renderPage for unknown path = %q, want empty", html)
	}
}

func TestTitleFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "Home"},
		{"/about", "About"},
		{"/contact-us", "Contact Us"},
		{"/blog/my-first-post", "My First Post"},
		{"/docs", "Docs"},
	}
	for _, tt := range tests {
		got := titleFromPath(tt.path)
		if got != tt.want {
			t.Errorf("titleFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestApp_ValidateRoutes_AllRegistered(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	noop := testRender(func(p PageData) string { return "" })
	app.Page("/", noop)
	app.Page("/about", noop)
	app.Collection("/blog", "Blog", noop, noop)

	routes := []ScannedRoute{
		{FilePath: "index.templ", URLPattern: "/", Type: TypePage},
		{FilePath: "about.templ", URLPattern: "/about", Type: TypePage},
		{FilePath: "blog/index.templ", URLPattern: "/blog", Type: TypeListing},
		{FilePath: "blog/[slug].templ", URLPattern: "/blog/:slug", Type: TypeEntry},
	}

	errs := app.ValidateRoutes(routes)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestApp_ValidateRoutes_MissingPage(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	noop := testRender(func(p PageData) string { return "" })
	app.Page("/", noop)
	// /about is NOT registered

	routes := []ScannedRoute{
		{FilePath: "index.templ", URLPattern: "/", Type: TypePage},
		{FilePath: "about.templ", URLPattern: "/about", Type: TypePage},
	}

	errs := app.ValidateRoutes(routes)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "/about") {
		t.Errorf("error should mention /about: %q", errs[0])
	}
}

func TestApp_ValidateRoutes_MissingCollection(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	// No collection registered

	routes := []ScannedRoute{
		{FilePath: "blog/index.templ", URLPattern: "/blog", Type: TypeListing},
		{FilePath: "blog/[slug].templ", URLPattern: "/blog/:slug", Type: TypeEntry},
	}

	errs := app.ValidateRoutes(routes)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestApp_ValidateRoutes_ExtraRegistrations(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	noop := testRender(func(p PageData) string { return "" })
	app.Page("/", noop)
	app.Page("/about", noop)   // registered but not in scanned routes
	app.Page("/contact", noop) // registered but not in scanned routes

	routes := []ScannedRoute{
		{FilePath: "index.templ", URLPattern: "/", Type: TypePage},
	}

	errs := app.ValidateRoutes(routes)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors for unmatched registrations, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if !strings.Contains(e, "no matching file") {
			t.Errorf("expected 'no matching file' error, got: %q", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Route scanning tests
// ---------------------------------------------------------------------------

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

func TestScanRoutes_NonexistentDir(t *testing.T) {
	_, err := ScanRoutes("/tmp/nonexistent-dir-that-should-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}
