package cms

import (
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
