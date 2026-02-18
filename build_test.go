package cms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsonVal marshals a value to json.RawMessage for test API responses.
func jsonVal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func TestBuild_CreatesHTMLFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/", Slug: "home", TemplateID: "t1"},
			})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Hello")},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/" || r.URL.Path == "/api/v1/test/seo//":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "Home"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return "<html><body>" + p.Text("title") + "</body></html>"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "<html><body>Hello</body></html>" {
		t.Errorf("index.html = %q", string(content))
	}
}

func TestBuild_NestedPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("About")},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/blog/first-post":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog/first-post", Slug: "first-post",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("First Post")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/about", testRender(func(p PageData) string { return p.Text("title") }))
	app.Page("/blog/first-post", testRender(func(p PageData) string { return p.Text("title") }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "about", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "About" {
		t.Errorf("about/index.html = %q", string(content))
	}

	content, err = os.ReadFile(filepath.Join(outDir, "blog", "first-post", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "First Post" {
		t.Errorf("blog/first-post/index.html = %q", string(content))
	}
}

func TestBuild_WritesSyncJSON(t *testing.T) {
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
	app.Page("/", testRender(func(p PageData) string { return "" }))

	outDir := t.TempDir()
	syncPath := filepath.Join(outDir, "sync.json")

	err := app.Build(context.Background(), BuildOptions{
		OutDir:   outDir,
		SyncFile: syncPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(syncPath)
	if err != nil {
		t.Fatal(err)
	}

	var payload SyncPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Pages) != 1 || payload.Pages[0].Path != "/" {
		t.Errorf("sync payload = %+v", payload)
	}
}

func TestBuild_CollectionListings_AttachedToIndexPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/", Slug: "home", TemplateID: "t1"},
				{ID: "p2", Path: "/blog/first", Slug: "first", TemplateID: "t2"},
				{ID: "p3", Path: "/blog/second", Slug: "second", TemplateID: "t2"},
			})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Home")},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/blog":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog", Slug: "blog",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Blog")},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/blog/first":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog/first", Slug: "first",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("First Post")},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/blog/second":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog/second", Slug: "second",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Second Post")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return p.Text("title") }))
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string {
			posts := p.Listing("blog")
			result := "Blog:"
			for _, post := range posts {
				result += " " + post.Text("title")
			}
			return result
		}),
		testRender(func(p PageData) string { return p.Text("title") }),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Blog listing should contain listing data.
	content, err := os.ReadFile(filepath.Join(outDir, "blog", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(content)
	if !strings.Contains(html, "First Post") {
		t.Errorf("blog index missing 'First Post': %q", html)
	}
	if !strings.Contains(html, "Second Post") {
		t.Errorf("blog index missing 'Second Post': %q", html)
	}

	// Individual entry pages should exist.
	if _, err := os.ReadFile(filepath.Join(outDir, "blog", "first", "index.html")); err != nil {
		t.Errorf("blog/first/index.html not found: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(outDir, "blog", "second", "index.html")); err != nil {
		t.Errorf("blog/second/index.html not found: %v", err)
	}

	// Home page should NOT have blog listings in its render.
	homeContent, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(homeContent) != "Home" {
		t.Errorf("home page = %q, want 'Home'", string(homeContent))
	}
}

func TestBuild_NoContent_RendersWithEmptyPageData(t *testing.T) {
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
	app.Page("/", testRender(func(p PageData) string {
		title := p.Text("title")
		if title != "" {
			return "unexpected: " + title
		}
		return "<html>empty</html>"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "<html>empty</html>" {
		t.Errorf("index.html = %q", string(content))
	}
}

func TestBuild_CollectionTemplate_UsesEntryRenderFunc(t *testing.T) {
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
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing-page" }),
		testRender(func(p PageData) string { return "template-page" }),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Template page uses entry render func.
	content, err := os.ReadFile(filepath.Join(outDir, "blog", "_template", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "template-page" {
		t.Errorf("template page = %q, want 'template-page'", string(content))
	}

	// Listing page uses listing render func.
	content, err = os.ReadFile(filepath.Join(outDir, "blog", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "listing-page" {
		t.Errorf("listing page = %q, want 'listing-page'", string(content))
	}
}

func TestBuild_Minify_RemovesWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Hello")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return "<!DOCTYPE html>\n<html>\n  <head>\n    <title>Test</title>\n  </head>\n  <body>\n    <h1>" + p.Text("title") + "</h1>\n  </body>\n</html>"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir, Minify: true})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	html := string(content)

	// Minified output should not contain the original indentation.
	if strings.Contains(html, "    <h1>") {
		t.Errorf("expected minified HTML, got indented output: %q", html)
	}
	// But must still contain the actual content.
	if !strings.Contains(html, "Hello") {
		t.Errorf("minified HTML missing content 'Hello': %q", html)
	}
	if !strings.Contains(html, "<!doctype html>") && !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("minified HTML missing doctype: %q", html)
	}
}

func TestBuild_Minify_Disabled_PreservesWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Hello")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return "<!DOCTYPE html>\n<html>\n  <body>\n    <h1>" + p.Text("title") + "</h1>\n  </body>\n</html>"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir, Minify: false})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	// With minify disabled, original whitespace is preserved.
	if !strings.Contains(string(content), "    <h1>") {
		t.Errorf("expected original whitespace preserved, got: %q", string(content))
	}
}

func TestBuild_ConcurrentFetches(t *testing.T) {
	const pageCount = 20

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case strings.HasPrefix(r.URL.Path, "/api/v1/test/pages/page-"):
			slug := strings.TrimPrefix(r.URL.Path, "/api/v1/test/pages/")
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/" + slug, Slug: slug,
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Title " + slug)},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/test/seo/page-"):
			slug := strings.TrimPrefix(r.URL.Path, "/api/v1/test/seo/")
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "SEO " + slug})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	for i := 0; i < pageCount; i++ {
		slug := fmt.Sprintf("page-%d", i)
		app.Page("/"+slug, testRender(func(p PageData) string {
			return p.Text("title")
		}))
	}

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < pageCount; i++ {
		slug := fmt.Sprintf("page-%d", i)
		content, err := os.ReadFile(filepath.Join(outDir, slug, "index.html"))
		if err != nil {
			t.Errorf("page %s not found: %v", slug, err)
			continue
		}
		expected := "Title " + slug
		if string(content) != expected {
			t.Errorf("page %s = %q, want %q", slug, string(content), expected)
		}
	}
}
