package cms

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// isErrorPage
// ---------------------------------------------------------------------------

func TestIsErrorPage(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/404", true},
		{"/500", true},
		{"/en/404", true},
		{"/nl/500", true},
		{"/about", false},
		{"/", false},
		{"/blog/404-reasons", false}, // "404-reasons" is not "404"
	}
	for _, tt := range tests {
		got := isErrorPage(tt.path)
		if got != tt.want {
			t.Errorf("isErrorPage(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// chunkStrings
// ---------------------------------------------------------------------------

func TestChunkStrings_SingleChunk(t *testing.T) {
	input := []string{"a", "b", "c"}
	chunks := chunkStrings(input, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0]) != 3 {
		t.Errorf("chunk[0] has %d items, want 3", len(chunks[0]))
	}
}

func TestChunkStrings_MultipleChunks(t *testing.T) {
	input := []string{"a", "b", "c", "d", "e"}
	chunks := chunkStrings(input, 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 2 {
		t.Errorf("chunk[0] = %d, want 2", len(chunks[0]))
	}
	if len(chunks[2]) != 1 {
		t.Errorf("chunk[2] = %d, want 1", len(chunks[2]))
	}
}

func TestChunkStrings_ExactFit(t *testing.T) {
	input := []string{"a", "b", "c", "d"}
	chunks := chunkStrings(input, 2)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

// ---------------------------------------------------------------------------
// NoSitemap option
// ---------------------------------------------------------------------------

func TestNoSitemap_ExcludesPage(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/", testRender(func(p PageData) string { return "home" }))
	app.Page("/404", testRender(func(p PageData) string { return "not found" }), NoSitemap)
	app.Page("/about", testRender(func(p PageData) string { return "about" }))

	sd := app.collectSitemapURLs(nil, nil, "")

	// /404 is excluded both by NoSitemap and by isErrorPage.
	// / and /about should be present.
	if len(sd.pages) != 2 {
		t.Fatalf("expected 2 pages, got %d: %v", len(sd.pages), sd.pages)
	}
	for _, p := range sd.pages {
		if p.path == "/404" {
			t.Error("404 page should be excluded from sitemap")
		}
	}
}

// ---------------------------------------------------------------------------
// Priority and ChangeFreq options
// ---------------------------------------------------------------------------

func TestPriority_OverridesDefault(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/about", testRender(func(p PageData) string { return "" }), Priority(0.9))

	sd := app.collectSitemapURLs(nil, nil, "")

	if len(sd.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(sd.pages))
	}
	if sd.pages[0].priority != "0.9" {
		t.Errorf("priority = %q, want 0.9", sd.pages[0].priority)
	}
}

func TestChangeFreq_OverridesDefault(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/about", testRender(func(p PageData) string { return "" }), ChangeFreq("monthly"))

	sd := app.collectSitemapURLs(nil, nil, "")

	if len(sd.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(sd.pages))
	}
	if sd.pages[0].changeFreq != "monthly" {
		t.Errorf("changeFreq = %q, want monthly", sd.pages[0].changeFreq)
	}
}

func TestDefaultPriority_HomepageIsHighest(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/", testRender(func(p PageData) string { return "" }))
	app.Page("/about", testRender(func(p PageData) string { return "" }))

	sd := app.collectSitemapURLs(nil, nil, "")

	var homePri, aboutPri string
	for _, p := range sd.pages {
		if p.path == "/" {
			homePri = p.priority
		}
		if p.path == "/about" {
			aboutPri = p.priority
		}
	}
	if homePri != "1.0" {
		t.Errorf("home priority = %q, want 1.0", homePri)
	}
	if aboutPri != "0.8" {
		t.Errorf("about priority = %q, want 0.8", aboutPri)
	}
}

func TestDefaultChangeFreq_HomepageIsDaily(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/", testRender(func(p PageData) string { return "" }))
	app.Page("/about", testRender(func(p PageData) string { return "" }))

	sd := app.collectSitemapURLs(nil, nil, "")

	var homeFreq, aboutFreq string
	for _, p := range sd.pages {
		if p.path == "/" {
			homeFreq = p.changeFreq
		}
		if p.path == "/about" {
			aboutFreq = p.changeFreq
		}
	}
	if homeFreq != "daily" {
		t.Errorf("home changefreq = %q, want daily", homeFreq)
	}
	if aboutFreq != "weekly" {
		t.Errorf("about changefreq = %q, want weekly", aboutFreq)
	}
}

// ---------------------------------------------------------------------------
// lastmod from CMS updated_at
// ---------------------------------------------------------------------------

func TestLastMod_UsesCMSUpdatedAt(t *testing.T) {
	updatedAt := "2026-02-20T14:30:00Z"
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/about", testRender(func(p PageData) string { return "" }))

	allPages := []apiPageListItem{
		{Path: "/about", Slug: "about", UpdatedAt: &updatedAt},
	}

	sd := app.collectSitemapURLs(allPages, nil, "")

	if len(sd.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(sd.pages))
	}
	if sd.pages[0].lastMod != "2026-02-20" {
		t.Errorf("lastmod = %q, want 2026-02-20", sd.pages[0].lastMod)
	}
}

// ---------------------------------------------------------------------------
// collectSitemapURLs — single locale
// ---------------------------------------------------------------------------

func TestCollectSitemapURLs_SingleLocale(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/", testRender(func(p PageData) string { return "" }))
	app.Page("/about", testRender(func(p PageData) string { return "" }))
	app.Page("/404", testRender(func(p PageData) string { return "" }), NoSitemap)
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "" }),
		testRender(func(p PageData) string { return "" }),
	)

	allPages := []apiPageListItem{
		{Path: "/blog/hello", Slug: "hello"},
		{Path: "/blog/world", Slug: "world"},
	}

	sd := app.collectSitemapURLs(allPages, nil, "")

	// Fixed pages: / and /about (404 excluded).
	// Collection listing: /blog.
	if len(sd.pages) != 3 {
		t.Errorf("expected 3 pages, got %d: %v", len(sd.pages), sd.pages)
	}

	// Collection entries.
	if len(sd.collections["blog"]) != 2 {
		t.Errorf("expected 2 blog entries, got %d", len(sd.collections["blog"]))
	}
}

// ---------------------------------------------------------------------------
// collectSitemapURLs — multi locale
// ---------------------------------------------------------------------------

func TestCollectSitemapURLs_MultiLocale(t *testing.T) {
	app := NewApp(Config{SiteURL: "https://example.com"})
	app.Page("/", testRender(func(p PageData) string { return "" }))
	app.Page("/about", testRender(func(p PageData) string { return "" }))

	locales := []SiteLocale{
		{Code: "en", Label: "English", IsDefault: true},
		{Code: "nl", Label: "Nederlands"},
	}

	sd := app.collectSitemapURLs(nil, locales, "en")

	// 2 pages * 2 locales = 4.
	if len(sd.pages) != 4 {
		t.Errorf("expected 4 pages, got %d: %v", len(sd.pages), sd.pages)
	}

	// Check that both locale prefixes exist.
	hasEN := false
	hasNL := false
	for _, p := range sd.pages {
		if strings.HasPrefix(p.path, "/en") {
			hasEN = true
		}
		if strings.HasPrefix(p.path, "/nl") {
			hasNL = true
		}
	}
	if !hasEN || !hasNL {
		t.Errorf("expected both /en and /nl prefixes in: %v", sd.pages)
	}
}

// ---------------------------------------------------------------------------
// Sitemap XML writing — single file with metadata
// ---------------------------------------------------------------------------

func TestSitemapWrite_SingleFile(t *testing.T) {
	sd := &sitemapData{
		siteURL: "https://example.com",
		pages: []sitemapURLEntry{
			{path: "/", lastMod: "2026-02-25", changeFreq: "daily", priority: "1.0"},
			{path: "/about", lastMod: "2026-02-25", changeFreq: "weekly", priority: "0.8"},
		},
		collections: map[string][]sitemapURLEntry{},
	}

	outDir := t.TempDir()
	if err := sd.write(outDir, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should be a urlset (not a sitemapindex).
	if !strings.Contains(content, "<urlset") {
		t.Error("expected <urlset> element")
	}
	if strings.Contains(content, "<sitemapindex") {
		t.Error("should not contain <sitemapindex> for small site")
	}
	if !strings.Contains(content, "<loc>https://example.com/</loc>") {
		t.Error("missing / URL")
	}
	if !strings.Contains(content, "<loc>https://example.com/about</loc>") {
		t.Error("missing /about URL")
	}
	// Should contain metadata.
	if !strings.Contains(content, "<lastmod>2026-02-25</lastmod>") {
		t.Error("missing lastmod")
	}
	if !strings.Contains(content, "<changefreq>daily</changefreq>") {
		t.Error("missing changefreq daily")
	}
	if !strings.Contains(content, "<changefreq>weekly</changefreq>") {
		t.Error("missing changefreq weekly")
	}
	if !strings.Contains(content, "<priority>1.0</priority>") {
		t.Error("missing priority 1.0")
	}
	if !strings.Contains(content, "<priority>0.8</priority>") {
		t.Error("missing priority 0.8")
	}
}

// ---------------------------------------------------------------------------
// Sitemap XML writing — with hreflang alternates + x-default
// ---------------------------------------------------------------------------

func TestSitemapWrite_MultiLocale_HasHreflang(t *testing.T) {
	sd := &sitemapData{
		siteURL: "https://example.com",
		pages: []sitemapURLEntry{
			{path: "/en", changeFreq: "daily", priority: "1.0"},
			{path: "/nl", changeFreq: "daily", priority: "1.0"},
			{path: "/en/about", changeFreq: "weekly", priority: "0.8"},
			{path: "/nl/about", changeFreq: "weekly", priority: "0.8"},
		},
		collections: map[string][]sitemapURLEntry{},
	}
	locales := []SiteLocale{
		{Code: "en", Label: "English", IsDefault: true},
		{Code: "nl", Label: "Nederlands"},
	}

	outDir := t.TempDir()
	if err := sd.write(outDir, locales); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should contain xhtml namespace.
	if !strings.Contains(content, "xmlns:xhtml") {
		t.Error("expected xhtml namespace for hreflang")
	}
	// Should have hreflang alternate links.
	if !strings.Contains(content, `hreflang="en"`) {
		t.Error("missing en hreflang")
	}
	if !strings.Contains(content, `hreflang="nl"`) {
		t.Error("missing nl hreflang")
	}
	// Should have x-default hreflang.
	if !strings.Contains(content, `hreflang="x-default"`) {
		t.Error("missing x-default hreflang")
	}
	// x-default for the root should point to the unprefixed URL.
	if !strings.Contains(content, `hreflang="x-default" href="https://example.com/"`) {
		t.Errorf("x-default for root should point to unprefixed URL, got:\n%s", content)
	}
}

// ---------------------------------------------------------------------------
// Sitemap XML writing — index with sub-sitemaps
// ---------------------------------------------------------------------------

func TestSitemapWrite_Index_MultipleCollections(t *testing.T) {
	sd := &sitemapData{
		siteURL: "https://example.com",
		pages: []sitemapURLEntry{
			{path: "/", priority: "1.0"},
			{path: "/about", priority: "0.8"},
		},
		collections: map[string][]sitemapURLEntry{
			"blog":     {{path: "/blog/a", priority: "0.6"}, {path: "/blog/b", priority: "0.6"}},
			"products": {{path: "/products/x", priority: "0.6"}, {path: "/products/y", priority: "0.6"}},
		},
	}

	outDir := t.TempDir()
	if err := sd.write(outDir, nil); err != nil {
		t.Fatal(err)
	}

	// Should create a sitemap index.
	indexData, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	indexContent := string(indexData)
	if !strings.Contains(indexContent, "<sitemapindex") {
		t.Error("expected <sitemapindex> for multiple collections")
	}
	if !strings.Contains(indexContent, "sitemap-pages.xml") {
		t.Error("missing sitemap-pages.xml reference")
	}
	if !strings.Contains(indexContent, "sitemap-blog.xml") {
		t.Error("missing sitemap-blog.xml reference")
	}
	if !strings.Contains(indexContent, "sitemap-products.xml") {
		t.Error("missing sitemap-products.xml reference")
	}

	// Sub-sitemaps should exist.
	if _, err := os.Stat(filepath.Join(outDir, "sitemap-pages.xml")); err != nil {
		t.Error("sitemap-pages.xml not found")
	}
	if _, err := os.Stat(filepath.Join(outDir, "sitemap-blog.xml")); err != nil {
		t.Error("sitemap-blog.xml not found")
	}
	if _, err := os.Stat(filepath.Join(outDir, "sitemap-products.xml")); err != nil {
		t.Error("sitemap-products.xml not found")
	}
}

// ---------------------------------------------------------------------------
// robots.txt
// ---------------------------------------------------------------------------

func TestWriteRobotsTxt(t *testing.T) {
	outDir := t.TempDir()
	if err := writeRobotsTxt(outDir, "https://example.com"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "robots.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "User-agent: *") {
		t.Error("missing User-agent directive")
	}
	if !strings.Contains(content, "Allow: /") {
		t.Error("missing Allow directive")
	}
	if !strings.Contains(content, "Sitemap: https://example.com/sitemap.xml") {
		t.Error("missing Sitemap directive")
	}
}

// ---------------------------------------------------------------------------
// makeSitemapURL with hreflang alternates
// ---------------------------------------------------------------------------

func TestMakeSitemapURL_SingleLocale(t *testing.T) {
	sd := &sitemapData{siteURL: "https://example.com"}
	e := sitemapURLEntry{path: "/about", lastMod: "2026-02-25", changeFreq: "weekly", priority: "0.8"}
	u := sd.makeSitemapURL(e, nil)
	if u.Loc != "https://example.com/about" {
		t.Errorf("Loc = %q, want https://example.com/about", u.Loc)
	}
	if u.LastMod != "2026-02-25" {
		t.Errorf("LastMod = %q, want 2026-02-25", u.LastMod)
	}
	if u.ChangeFreq != "weekly" {
		t.Errorf("ChangeFreq = %q, want weekly", u.ChangeFreq)
	}
	if u.Priority != "0.8" {
		t.Errorf("Priority = %q, want 0.8", u.Priority)
	}
	if len(u.Alternates) != 0 {
		t.Errorf("expected no alternates for single locale, got %d", len(u.Alternates))
	}
}

func TestMakeSitemapURL_MultiLocale(t *testing.T) {
	locales := []SiteLocale{
		{Code: "en", Label: "English", IsDefault: true},
		{Code: "nl", Label: "Nederlands"},
	}
	sd := &sitemapData{siteURL: "https://example.com"}
	e := sitemapURLEntry{path: "/en/about", changeFreq: "weekly", priority: "0.8"}
	u := sd.makeSitemapURL(e, locales)

	if u.Loc != "https://example.com/en/about" {
		t.Errorf("Loc = %q", u.Loc)
	}
	// 2 locale alternates + 1 x-default = 3.
	if len(u.Alternates) != 3 {
		t.Fatalf("expected 3 alternates (en, nl, x-default), got %d", len(u.Alternates))
	}

	// Check alternates point to the right URLs.
	var enAlt, nlAlt, xDefault string
	for _, alt := range u.Alternates {
		switch alt.Hreflang {
		case "en":
			enAlt = alt.Href
		case "nl":
			nlAlt = alt.Href
		case "x-default":
			xDefault = alt.Href
		}
	}
	if enAlt != "https://example.com/en/about" {
		t.Errorf("en alternate = %q", enAlt)
	}
	if nlAlt != "https://example.com/nl/about" {
		t.Errorf("nl alternate = %q", nlAlt)
	}
	if xDefault != "https://example.com/about" {
		t.Errorf("x-default = %q, want https://example.com/about", xDefault)
	}
}

// ---------------------------------------------------------------------------
// formatPriority
// ---------------------------------------------------------------------------

func TestFormatPriority(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{1.0, "1.0"},
		{0.8, "0.8"},
		{0.5, "0.5"},
		{0.0, "0.0"},
	}
	for _, tt := range tests {
		got := formatPriority(tt.in)
		if got != tt.want {
			t.Errorf("formatPriority(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Sitemap XML structure validation
// ---------------------------------------------------------------------------

func TestSitemapWrite_ValidXML(t *testing.T) {
	sd := &sitemapData{
		siteURL: "https://example.com",
		pages: []sitemapURLEntry{
			{path: "/", priority: "1.0", changeFreq: "daily"},
			{path: "/about", priority: "0.8", changeFreq: "weekly"},
			{path: "/contact", priority: "0.8", changeFreq: "weekly"},
		},
		collections: map[string][]sitemapURLEntry{},
	}

	outDir := t.TempDir()
	if err := sd.write(outDir, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}

	// Validate it's well-formed XML.
	var us urlSet
	if err := xml.Unmarshal(data, &us); err != nil {
		t.Fatalf("invalid XML: %v\n%s", err, data)
	}
	if len(us.URLs) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(us.URLs))
	}
}

// ---------------------------------------------------------------------------
// Integration: Build() with SiteURL generates sitemap + robots.txt
// ---------------------------------------------------------------------------

func TestBuild_WithSiteURL_GeneratesSitemapAndRobots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/blog/hello", Slug: "hello"},
			})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/", Slug: "home"})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/about", Slug: "about"})
		case r.URL.Path == "/api/v1/test/pages/blog":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/blog", Slug: "blog"})
		case r.URL.Path == "/api/v1/test/pages/blog/hello":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/blog/hello", Slug: "hello"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{
		APIURL:   srv.URL,
		SiteSlug: "test",
		APIKey:   "k",
		SiteURL:  "https://example.com",
	})
	app.Page("/", testRender(func(p PageData) string { return "home" }))
	app.Page("/about", testRender(func(p PageData) string { return "about" }))
	app.Page("/404", testRender(func(p PageData) string { return "not found" }), NoSitemap)
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing" }),
		testRender(func(p PageData) string { return "entry" }),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// sitemap.xml should exist.
	sitemapData, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal("sitemap.xml not found:", err)
	}
	sitemap := string(sitemapData)

	// Should contain the pages (not the 404).
	if !strings.Contains(sitemap, "https://example.com/") {
		t.Error("sitemap missing /")
	}
	if !strings.Contains(sitemap, "https://example.com/about") {
		t.Error("sitemap missing /about")
	}
	if strings.Contains(sitemap, "https://example.com/404") {
		t.Error("sitemap should not contain /404")
	}
	// Should contain blog entry.
	if !strings.Contains(sitemap, "https://example.com/blog/hello") {
		t.Error("sitemap missing /blog/hello")
	}
	// Should not contain _template.
	if strings.Contains(sitemap, "_template") {
		t.Error("sitemap should not contain _template")
	}
	// Should contain metadata.
	if !strings.Contains(sitemap, "<lastmod>") {
		t.Error("sitemap missing lastmod")
	}
	if !strings.Contains(sitemap, "<changefreq>") {
		t.Error("sitemap missing changefreq")
	}
	if !strings.Contains(sitemap, "<priority>") {
		t.Error("sitemap missing priority")
	}

	// robots.txt should exist.
	robotsData, err := os.ReadFile(filepath.Join(outDir, "robots.txt"))
	if err != nil {
		t.Fatal("robots.txt not found:", err)
	}
	robots := string(robotsData)
	if !strings.Contains(robots, "Sitemap: https://example.com/sitemap.xml") {
		t.Error("robots.txt missing Sitemap directive")
	}
}

// ---------------------------------------------------------------------------
// Integration: Build() without SiteURL does NOT generate sitemap
// ---------------------------------------------------------------------------

func TestBuild_WithoutSiteURL_NoSitemap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// No sitemap.xml or robots.txt.
	if _, err := os.Stat(filepath.Join(outDir, "sitemap.xml")); !os.IsNotExist(err) {
		t.Error("sitemap.xml should not exist without SiteURL")
	}
	if _, err := os.Stat(filepath.Join(outDir, "robots.txt")); !os.IsNotExist(err) {
		t.Error("robots.txt should not exist without SiteURL")
	}
}

// ---------------------------------------------------------------------------
// resolveSiteURL: auto-fetch from CMS
// ---------------------------------------------------------------------------

func TestBuild_ResolveSiteURL_FromCMS(t *testing.T) {
	domain := "example.com"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/site":
			json.NewEncoder(w).Encode(apiSiteResponse{
				Name:          "Test",
				Slug:          "test",
				Domain:        &domain,
				DefaultLocale: "en",
			})
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	// No SiteURL in config — should auto-fetch from CMS.
	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// sitemap.xml should exist (domain was fetched from CMS).
	sitemapContent, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal("sitemap.xml not found — expected CMS domain auto-detection:", err)
	}
	if !strings.Contains(string(sitemapContent), "https://example.com/") {
		t.Errorf("sitemap should use CMS domain, got: %s", sitemapContent)
	}
}
