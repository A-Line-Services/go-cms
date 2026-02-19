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

// ---------------------------------------------------------------------------
// Media downloader tests
// ---------------------------------------------------------------------------

// fakeJPEG returns a minimal valid JPEG header (not a real image, but enough for tests).
var fakeJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}

func TestMediaDownloader_Download_SavesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "media")
	dl := newMediaDownloader(outDir, "/media")

	webPath, err := dl.download(srv.URL + "/hero.jpg")
	if err != nil {
		t.Fatal(err)
	}

	// Should return a /media/... path.
	if !strings.HasPrefix(webPath, "/media/") {
		t.Errorf("webPath = %q, want /media/ prefix", webPath)
	}

	// File should exist on disk.
	filename := strings.TrimPrefix(webPath, "/media/")
	diskPath := filepath.Join(outDir, filename)
	data, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if len(data) != len(fakeJPEG) {
		t.Errorf("file size = %d, want %d", len(data), len(fakeJPEG))
	}
}

func TestMediaDownloader_Download_CachesResults(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "media")
	dl := newMediaDownloader(outDir, "/media")

	url := srv.URL + "/hero.jpg"
	path1, _ := dl.download(url)
	path2, _ := dl.download(url)

	if path1 != path2 {
		t.Errorf("paths differ: %q vs %q", path1, path2)
	}
	if calls != 1 {
		t.Errorf("server called %d times, want 1", calls)
	}
}

func TestMediaDownloader_DownloadBase64_ReturnsDataURI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	dl := newMediaDownloader(t.TempDir(), "/media")

	dataURI, err := dl.downloadBase64(srv.URL + "/hero.jpg?w=32&q=20")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(dataURI, "data:image/jpeg;base64,") {
		t.Errorf("dataURI = %q, want data:image/jpeg;base64, prefix", dataURI)
	}
}

func TestMediaDownloader_Processor_PopulatesResolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "media")
	dl := newMediaDownloader(outDir, "/media")
	proc := dl.processor()

	img := ImageValue{URL: srv.URL + "/hero.jpg", Alt: "Hero"}
	result := proc(img)

	// Full-size image should be resolved.
	if result.Src() == img.URL {
		t.Error("Src() should return local path, not remote URL")
	}
	if !strings.HasPrefix(result.Src(), "/media/") {
		t.Errorf("Src() = %q, want /media/ prefix", result.Src())
	}

	// LQIP should be a data URI.
	if !strings.HasPrefix(result.LQIP(), "data:image/jpeg;base64,") {
		t.Errorf("LQIP() = %q, want data URI", result.LQIP())
	}

	// SrcSet should have local paths.
	srcset := result.SrcSet(400, 800, 1200, 1600)
	if strings.Contains(srcset, "https://") || strings.Contains(srcset, "http://") {
		t.Errorf("SrcSet still contains remote URLs: %q", srcset)
	}
	if !strings.Contains(srcset, "/media/") {
		t.Errorf("SrcSet should contain /media/ paths: %q", srcset)
	}
}

func TestMediaDownloader_Processor_EmptyURL_Noop(t *testing.T) {
	dl := newMediaDownloader(t.TempDir(), "/media")
	proc := dl.processor()

	img := ImageValue{}
	result := proc(img)

	if result.URL != "" {
		t.Errorf("URL = %q, want empty", result.URL)
	}
}

func TestMediaDownloader_HashFilename_Deterministic(t *testing.T) {
	a := hashFilename("https://cdn.test/hero.jpg?w=400")
	b := hashFilename("https://cdn.test/hero.jpg?w=400")
	if a != b {
		t.Errorf("hashes differ: %q vs %q", a, b)
	}

	c := hashFilename("https://cdn.test/hero.jpg?w=800")
	if a == c {
		t.Errorf("different URLs should produce different hashes")
	}
}

func TestHashFilename_IgnoresSignatureParams(t *testing.T) {
	// Same image with different sig/exp should produce the same hash.
	a := hashFilename("https://api.test/media-file/site/media?sig=AAA&exp=1000&w=400")
	b := hashFilename("https://api.test/media-file/site/media?sig=BBB&exp=2000&w=400")
	if a != b {
		t.Errorf("same image with different sig/exp should hash equally: %q vs %q", a, b)
	}

	// Different processing params should produce different hashes.
	c := hashFilename("https://api.test/media-file/site/media?sig=AAA&exp=1000&w=800")
	if a == c {
		t.Errorf("different widths should produce different hashes")
	}
}

func TestStableURL_StripsVolatileParams(t *testing.T) {
	got := stableURL("https://api.test/media-file/s/m?sig=ABC&exp=1000&w=400&format=webp")
	if strings.Contains(got, "sig=") {
		t.Errorf("stableURL should strip sig: %q", got)
	}
	if strings.Contains(got, "exp=") {
		t.Errorf("stableURL should strip exp: %q", got)
	}
	if !strings.Contains(got, "w=400") {
		t.Errorf("stableURL should keep w: %q", got)
	}
	if !strings.Contains(got, "format=webp") {
		t.Errorf("stableURL should keep format: %q", got)
	}
}

func TestStableURL_NoQueryParams(t *testing.T) {
	got := stableURL("https://cdn.test/image.jpg")
	if got != "https://cdn.test/image.jpg" {
		t.Errorf("stableURL should not alter URL without params: %q", got)
	}
}

func TestMediaDownloader_Download_404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	dl := newMediaDownloader(t.TempDir(), "/media")
	_, err := dl.download(srv.URL + "/missing.jpg")
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestBuild_DownloadMedia_CreatesMediaDir(t *testing.T) {
	// Use a pointer so the handler closure can read srv.URL after start.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "hero", Locale: "en", Value: jsonVal(map[string]any{
						"url": srvURL + "/images/hero.jpg",
						"alt": "Hero image",
					})},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/" || r.URL.Path == "/api/v1/test/seo//":
			json.NewEncoder(w).Encode(apiSEOResponse{})
		case strings.HasPrefix(r.URL.Path, "/images/"):
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(fakeJPEG)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		img := p.Image("hero")
		return fmt.Sprintf("<img src=%q alt=%q>", img.Src(), img.Alt)
	}))

	outDir := t.TempDir()

	err := app.Build(context.Background(), BuildOptions{
		OutDir:        outDir,
		DownloadMedia: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Media directory should exist with files.
	mediaDir := filepath.Join(outDir, "media")
	entries, err := os.ReadDir(mediaDir)
	if err != nil {
		t.Fatalf("media dir not found: %v", err)
	}
	if len(entries) == 0 {
		t.Error("media dir is empty, expected downloaded images")
	}

	// HTML should reference /media/ paths, not the test server.
	html, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), srv.URL) {
		t.Errorf("HTML still contains remote URL: %s", html)
	}
	if !strings.Contains(string(html), "/media/") {
		t.Errorf("HTML should contain /media/ path: %s", html)
	}
}

func TestMediaDownloader_Processor_DownloadsFormatVariants(t *testing.T) {
	// Server supports format conversion: returns different Content-Type for format=webp/avif.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		switch format {
		case "webp":
			w.Header().Set("Content-Type", "image/webp")
		case "avif":
			w.Header().Set("Content-Type", "image/avif")
		default:
			w.Header().Set("Content-Type", "image/jpeg")
		}
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "media")
	dl := newMediaDownloader(outDir, "/media")
	proc := dl.processor()

	img := ImageValue{URL: srv.URL + "/hero.jpg", Alt: "Hero"}
	result := proc(img)

	// Should have WebP format variants.
	if !result.HasFormat("webp") {
		t.Error("expected HasFormat('webp') to be true")
	}
	// Should have AVIF format variants.
	if !result.HasFormat("avif") {
		t.Error("expected HasFormat('avif') to be true")
	}
	// WebP srcset should have local paths.
	webpSrcSet := result.SrcSetFor("webp", 400, 800, 1200, 1600)
	if strings.Contains(webpSrcSet, "http") {
		t.Errorf("WebP SrcSet contains remote URLs: %q", webpSrcSet)
	}
	if !strings.Contains(webpSrcSet, "/media/") {
		t.Errorf("WebP SrcSet should contain local paths: %q", webpSrcSet)
	}
}

func TestMediaDownloader_Processor_FormatVariants_GracefulOnError(t *testing.T) {
	// Server does NOT support format conversion: returns 404 for format params.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "media")
	dl := newMediaDownloader(outDir, "/media")
	proc := dl.processor()

	img := ImageValue{URL: srv.URL + "/hero.jpg", Alt: "Hero"}
	result := proc(img)

	// Original format should still work.
	if !strings.HasPrefix(result.Src(), "/media/") {
		t.Errorf("Src() = %q, want /media/ prefix", result.Src())
	}
	// Format variants should NOT be available.
	if result.HasFormat("webp") {
		t.Error("HasFormat('webp') should be false when backend returns 404")
	}
	if result.HasFormat("avif") {
		t.Error("HasFormat('avif') should be false when backend returns 404")
	}
}

// ---------------------------------------------------------------------------
// CMS attribute stripping tests
// ---------------------------------------------------------------------------

func TestStripCMSAttributes_RemovesDataCmsAttrs(t *testing.T) {
	input := `<div data-cms-field="title" data-cms-type="text">Hello</div>`
	got := stripCMSAttributes(input)
	want := `<div>Hello</div>`
	if got != want {
		t.Errorf("stripCMSAttributes =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestStripCMSAttributes_RemovesBooleanAttrs(t *testing.T) {
	input := `<div data-cms-entry data-cms-subcollection="partners">content</div>`
	got := stripCMSAttributes(input)
	want := `<div>content</div>`
	if got != want {
		t.Errorf("stripCMSAttributes =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestStripCMSAttributes_RemovesCmsMetaTags(t *testing.T) {
	input := `<head>
  <meta name="cms-template" content="homepage"/>
  <title>Test</title>
</head>`
	got := stripCMSAttributes(input)
	if strings.Contains(got, "cms-template") {
		t.Errorf("expected cms meta tag removed, got: %q", got)
	}
	if !strings.Contains(got, "<title>Test</title>") {
		t.Errorf("expected title preserved, got: %q", got)
	}
}

func TestStripCMSAttributes_PreservesNonCmsAttrs(t *testing.T) {
	input := `<div class="hero" id="main" data-cms-field="title" data-testid="hero">Hello</div>`
	got := stripCMSAttributes(input)
	if !strings.Contains(got, `class="hero"`) {
		t.Errorf("expected class preserved, got: %q", got)
	}
	if !strings.Contains(got, `id="main"`) {
		t.Errorf("expected id preserved, got: %q", got)
	}
	if !strings.Contains(got, `data-testid="hero"`) {
		t.Errorf("expected data-testid preserved, got: %q", got)
	}
	if strings.Contains(got, "data-cms-field") {
		t.Errorf("expected data-cms-field removed, got: %q", got)
	}
}

func TestStripCMSAttributes_MultipleAttrsOnElement(t *testing.T) {
	input := `<h1 data-cms-field="heading" data-cms-type="text" class="title">Welcome</h1>`
	got := stripCMSAttributes(input)
	want := `<h1 class="title">Welcome</h1>`
	if got != want {
		t.Errorf("stripCMSAttributes =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestStripCMSAttributes_NoOp_WhenNoCmsAttrs(t *testing.T) {
	input := `<div class="hero"><h1>Hello</h1></div>`
	got := stripCMSAttributes(input)
	if got != input {
		t.Errorf("expected no changes, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// pathToTemplateFile tests
// ---------------------------------------------------------------------------

func TestPathToTemplateFile_Root(t *testing.T) {
	got := pathToTemplateFile("/out", "/")
	want := filepath.Join("/out", "index.template.html")
	if got != want {
		t.Errorf("pathToTemplateFile('/') = %q, want %q", got, want)
	}
}

func TestPathToTemplateFile_Nested(t *testing.T) {
	got := pathToTemplateFile("/out", "/about")
	want := filepath.Join("/out", "about", "index.template.html")
	if got != want {
		t.Errorf("pathToTemplateFile('/about') = %q, want %q", got, want)
	}
}

func TestPathToTemplateFile_DeepNested(t *testing.T) {
	got := pathToTemplateFile("/out", "/blog/_template")
	want := filepath.Join("/out", "blog", "_template", "index.template.html")
	if got != want {
		t.Errorf("pathToTemplateFile('/blog/_template') = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// writeTemplateFiles tests
// ---------------------------------------------------------------------------

func TestBuild_WritesTemplateFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return `<h1 data-cms-field="title" data-cms-type="text">` + p.Text("title") + `</h1>`
	}))
	app.Page("/about", testRender(func(p PageData) string {
		return `<h1 data-cms-field="heading" data-cms-type="text">` + p.Text("heading") + `</h1>`
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Template files should exist alongside production files.
	tmpl, err := os.ReadFile(filepath.Join(outDir, "index.template.html"))
	if err != nil {
		t.Fatalf("index.template.html not found: %v", err)
	}
	// Template files should preserve data-cms-* attributes.
	if !strings.Contains(string(tmpl), "data-cms-field") {
		t.Errorf("template file should contain data-cms-field, got: %q", string(tmpl))
	}

	tmpl2, err := os.ReadFile(filepath.Join(outDir, "about", "index.template.html"))
	if err != nil {
		t.Fatalf("about/index.template.html not found: %v", err)
	}
	if !strings.Contains(string(tmpl2), "data-cms-field") {
		t.Errorf("about template should contain data-cms-field, got: %q", string(tmpl2))
	}
}

func TestBuild_ProductionHTML_NoCMSAttributes(t *testing.T) {
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
					{Key: "title", Locale: "en", Value: jsonVal("Hello World")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return `<html><head><meta name="cms-template" content="homepage"/></head><body><h1 data-cms-field="title" data-cms-type="text">` + p.Text("title") + `</h1></body></html>`
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Production HTML should be clean.
	prod, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	prodHTML := string(prod)

	if strings.Contains(prodHTML, "data-cms-") {
		t.Errorf("production HTML should not contain data-cms-* attributes: %q", prodHTML)
	}
	if strings.Contains(prodHTML, `cms-template`) {
		t.Errorf("production HTML should not contain cms meta tags: %q", prodHTML)
	}
	if !strings.Contains(prodHTML, "Hello World") {
		t.Errorf("production HTML should contain rendered content: %q", prodHTML)
	}

	// Template file should preserve CMS attributes.
	tmpl, err := os.ReadFile(filepath.Join(outDir, "index.template.html"))
	if err != nil {
		t.Fatal(err)
	}
	tmplHTML := string(tmpl)

	if !strings.Contains(tmplHTML, "data-cms-field") {
		t.Errorf("template HTML should contain data-cms-field: %q", tmplHTML)
	}
	if !strings.Contains(tmplHTML, "data-cms-type") {
		t.Errorf("template HTML should contain data-cms-type: %q", tmplHTML)
	}
}

func TestBuild_CollectionTemplateFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string {
			return `<div data-cms-field="title" data-cms-type="text">listing</div>`
		}),
		testRender(func(p PageData) string {
			return `<div data-cms-field="title" data-cms-type="text">entry</div>`
		}),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Collection listing template file.
	listTmpl, err := os.ReadFile(filepath.Join(outDir, "blog", "index.template.html"))
	if err != nil {
		t.Fatalf("blog/index.template.html not found: %v", err)
	}
	if !strings.Contains(string(listTmpl), "data-cms-field") {
		t.Errorf("blog listing template should contain data-cms-field: %q", string(listTmpl))
	}

	// Collection _template template file.
	entryTmpl, err := os.ReadFile(filepath.Join(outDir, "blog", "_template", "index.template.html"))
	if err != nil {
		t.Fatalf("blog/_template/index.template.html not found: %v", err)
	}
	if !strings.Contains(string(entryTmpl), "data-cms-field") {
		t.Errorf("blog entry template should contain data-cms-field: %q", string(entryTmpl))
	}

	// Production listing HTML should be clean.
	listProd, err := os.ReadFile(filepath.Join(outDir, "blog", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listProd), "data-cms-") {
		t.Errorf("production listing HTML should not contain data-cms-*: %q", string(listProd))
	}
}

func TestBuild_DownloadMedia_False_KeepsRemoteURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "hero", Locale: "en", Value: jsonVal(map[string]any{
						"url": "https://cdn.example.com/hero.jpg",
						"alt": "Hero",
					})},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/" || r.URL.Path == "/api/v1/test/seo//":
			json.NewEncoder(w).Encode(apiSEOResponse{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		img := p.Image("hero")
		return fmt.Sprintf("<img src=%q>", img.Src())
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{
		OutDir:        outDir,
		DownloadMedia: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	// HTML should keep remote URLs.
	html, _ := os.ReadFile(filepath.Join(outDir, "index.html"))
	if !strings.Contains(string(html), "https://cdn.example.com/hero.jpg") {
		t.Errorf("HTML should contain remote URL: %s", html)
	}

	// Media dir should NOT exist.
	if _, err := os.Stat(filepath.Join(outDir, "media")); !os.IsNotExist(err) {
		t.Error("media dir should not exist when DownloadMedia=false")
	}
}
