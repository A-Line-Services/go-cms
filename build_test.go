package cms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
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

func TestStripCMSAttributes_RemovesSectionAttrs(t *testing.T) {
	input := `<header class="site-header" data-cms-section="header" data-cms-label="Header" data-cms-shared><nav>links</nav></header>`
	got := stripCMSAttributes(input)
	want := `<header class="site-header"><nav>links</nav></header>`
	if got != want {
		t.Errorf("stripCMSAttributes =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestStripCMSAttributes_RemovesSectionAndFieldAttrs(t *testing.T) {
	input := `<section data-cms-section="hero" data-cms-label="Hero"><h1 data-cms-field="title" data-cms-type="text">Welcome</h1></section>`
	got := stripCMSAttributes(input)
	want := `<section><h1>Welcome</h1></section>`
	if got != want {
		t.Errorf("stripCMSAttributes =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestStripCMSAttributes_PreservesFormAttrs(t *testing.T) {
	input := `<form data-cms-form="contact" data-cms-label="Contact Form" class="mt-12">` +
		`<input data-cms-form-field="name" data-cms-label="Full Name" data-cms-type="text" data-cms-required id="name">` +
		`<input data-cms-form-field="email" data-cms-type="email" type="email">` +
		`</form>`
	got := stripCMSAttributes(input)

	// data-cms-form and data-cms-form-field must survive (needed by client JS)
	if !strings.Contains(got, `data-cms-form="contact"`) {
		t.Errorf("expected data-cms-form preserved, got: %q", got)
	}
	if !strings.Contains(got, `data-cms-form-field="name"`) {
		t.Errorf("expected data-cms-form-field='name' preserved, got: %q", got)
	}
	if !strings.Contains(got, `data-cms-form-field="email"`) {
		t.Errorf("expected data-cms-form-field='email' preserved, got: %q", got)
	}

	// Other CMS attrs must still be stripped
	if strings.Contains(got, "data-cms-label") {
		t.Errorf("expected data-cms-label stripped, got: %q", got)
	}
	if strings.Contains(got, "data-cms-type") {
		t.Errorf("expected data-cms-type stripped, got: %q", got)
	}
	if strings.Contains(got, "data-cms-required") {
		t.Errorf("expected data-cms-required stripped, got: %q", got)
	}
}

func TestBuild_SectionAttrs_StrippedFromProduction_PreservedInTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/home", Slug: "home", TemplateID: "t1"},
			})
		case r.URL.Path == "/api/v1/test/pages/home":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/home", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Hello")},
					{Key: "copyright", Locale: "en", Value: jsonVal("2026 Acme")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/home", testRender(func(p PageData) string {
		return `<header data-cms-section="header" data-cms-label="Header" data-cms-shared>` +
			`<h1 data-cms-field="title" data-cms-type="text">` + p.TextOr("title", "Welcome") + `</h1>` +
			`</header>` +
			`<section data-cms-section="hero" data-cms-label="Hero">` +
			`<p>Static content</p>` +
			`</section>` +
			`<footer data-cms-section="footer" data-cms-label="Footer" data-cms-shared>` +
			`<span data-cms-field="copyright" data-cms-type="text">` + p.TextOr("copyright", "2026") + `</span>` +
			`</footer>`
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Production HTML: no data-cms-* attributes at all.
	prod, err := os.ReadFile(filepath.Join(outDir, "home", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	prodHTML := string(prod)

	if strings.Contains(prodHTML, "data-cms-") {
		t.Errorf("production HTML should not contain data-cms-* attributes: %q", prodHTML)
	}
	// Structural elements preserved, content rendered.
	if !strings.Contains(prodHTML, "<header>") {
		t.Errorf("production HTML should contain <header>: %q", prodHTML)
	}
	if !strings.Contains(prodHTML, "Hello") {
		t.Errorf("production HTML should contain rendered title: %q", prodHTML)
	}
	if !strings.Contains(prodHTML, "2026 Acme") {
		t.Errorf("production HTML should contain rendered copyright: %q", prodHTML)
	}

	// Template HTML: preserves all data-cms-* attributes.
	tmpl, err := os.ReadFile(filepath.Join(outDir, "home", "index.template.html"))
	if err != nil {
		t.Fatalf("template file not found: %v", err)
	}
	tmplHTML := string(tmpl)

	if !strings.Contains(tmplHTML, `data-cms-section="header"`) {
		t.Errorf("template should contain data-cms-section='header': %q", tmplHTML)
	}
	if !strings.Contains(tmplHTML, `data-cms-shared`) {
		t.Errorf("template should contain data-cms-shared: %q", tmplHTML)
	}
	if !strings.Contains(tmplHTML, `data-cms-section="hero"`) {
		t.Errorf("template should contain data-cms-section='hero': %q", tmplHTML)
	}
	if !strings.Contains(tmplHTML, `data-cms-section="footer"`) {
		t.Errorf("template should contain data-cms-section='footer': %q", tmplHTML)
	}
	if !strings.Contains(tmplHTML, `data-cms-field="title"`) {
		t.Errorf("template should contain data-cms-field='title': %q", tmplHTML)
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

// ---------------------------------------------------------------------------
// Multi-locale build tests
// ---------------------------------------------------------------------------

// multiLocaleCMS creates a mock CMS that serves two locales (en default, nl)
// with locale-specific content for /, /about, and optionally collection entries.
func multiLocaleCMS(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale := r.URL.Query().Get("locale")
		if locale == "" {
			locale = "en"
		}

		switch {
		case r.URL.Path == "/api/v1/test/locales":
			json.NewEncoder(w).Encode([]apiLocaleResponse{
				{Locale: "en", Label: "English", IsDefault: true},
				{Locale: "nl", Label: "Nederlands", IsDefault: false},
			})
		case r.URL.Path == "/api/v1/test/pages" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/", Slug: "home", TemplateID: "t1"},
				{ID: "p2", Path: "/about", Slug: "about", TemplateID: "t1"},
			})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			title := "Hello"
			if locale == "nl" {
				title = "Hallo"
			}
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: locale, Value: jsonVal(title)},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/about":
			title := "About Us"
			if locale == "nl" {
				title = "Over Ons"
			}
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: locale, Value: jsonVal(title)},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestBuild_MultiLocale_CreatesPrefixedPaths(t *testing.T) {
	srv := multiLocaleCMS(t)
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return "<html lang=" + p.Locale + "><body>" + p.Text("title") + "</body></html>"
	}))
	app.Page("/about", testRender(func(p PageData) string {
		return "<html lang=" + p.Locale + "><body>" + p.Text("title") + "</body></html>"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// EN prefixed paths.
	enHome, err := os.ReadFile(filepath.Join(outDir, "en", "index.html"))
	if err != nil {
		t.Fatal("en/index.html missing:", err)
	}
	if !strings.Contains(string(enHome), "Hello") {
		t.Errorf("en/index.html should contain English content, got: %s", enHome)
	}

	enAbout, err := os.ReadFile(filepath.Join(outDir, "en", "about", "index.html"))
	if err != nil {
		t.Fatal("en/about/index.html missing:", err)
	}
	if !strings.Contains(string(enAbout), "About Us") {
		t.Errorf("en/about/index.html should contain English content, got: %s", enAbout)
	}

	// NL prefixed paths.
	nlHome, err := os.ReadFile(filepath.Join(outDir, "nl", "index.html"))
	if err != nil {
		t.Fatal("nl/index.html missing:", err)
	}
	if !strings.Contains(string(nlHome), "Hallo") {
		t.Errorf("nl/index.html should contain Dutch content, got: %s", nlHome)
	}

	nlAbout, err := os.ReadFile(filepath.Join(outDir, "nl", "about", "index.html"))
	if err != nil {
		t.Fatal("nl/about/index.html missing:", err)
	}
	if !strings.Contains(string(nlAbout), "Over Ons") {
		t.Errorf("nl/about/index.html should contain Dutch content, got: %s", nlAbout)
	}
}

func TestBuild_MultiLocale_DefaultLocaleAlsoAtRoot(t *testing.T) {
	srv := multiLocaleCMS(t)
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string { return p.Text("title") }))
	app.Page("/about", testRender(func(p PageData) string { return p.Text("title") }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Root paths should exist with English content (default locale).
	rootHome, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal("index.html missing:", err)
	}
	if string(rootHome) != "Hello" {
		t.Errorf("root index.html = %q, want Hello", rootHome)
	}

	rootAbout, err := os.ReadFile(filepath.Join(outDir, "about", "index.html"))
	if err != nil {
		t.Fatal("about/index.html missing:", err)
	}
	if string(rootAbout) != "About Us" {
		t.Errorf("root about/index.html = %q, want About Us", rootAbout)
	}

	// NL should NOT have root paths.
	if _, err := os.Stat(filepath.Join(outDir, "over-ons")); !os.IsNotExist(err) {
		t.Error("Dutch content should only be at /nl/, not at root")
	}
}

func TestBuild_MultiLocale_PageDataHasLocaleMetadata(t *testing.T) {
	srv := multiLocaleCMS(t)
	defer srv.Close()

	var capturedPages []PageData

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		capturedPages = append(capturedPages, p)
		return "ok"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Should have 4 renders: en prefixed, en root, nl prefixed
	// Actually: en prefixed + en root + nl prefixed = 3 for the / page
	// (template files are rendered separately and use default locale)
	if len(capturedPages) < 3 {
		t.Fatalf("expected at least 3 page renders for /, got %d", len(capturedPages))
	}

	// Find the prefixed EN render.
	var enPrefixed *PageData
	for i := range capturedPages {
		if capturedPages[i].Path == "/en" && capturedPages[i].Locale == "en" {
			enPrefixed = &capturedPages[i]
			break
		}
	}
	if enPrefixed == nil {
		t.Fatal("no /en page render found")
	}

	if enPrefixed.LocalePrefix() != "/en" {
		t.Errorf("LocalePrefix() = %q, want /en", enPrefixed.LocalePrefix())
	}
	if enPrefixed.ContentPath() != "/" {
		t.Errorf("ContentPath() = %q, want /", enPrefixed.ContentPath())
	}
	if len(enPrefixed.Locales) != 2 {
		t.Errorf("Locales has %d entries, want 2", len(enPrefixed.Locales))
	}
	if !enPrefixed.IsDefaultLocale() {
		t.Error("IsDefaultLocale() should be true for en")
	}

	// Find the NL render.
	var nlPrefixed *PageData
	for i := range capturedPages {
		if capturedPages[i].Path == "/nl" && capturedPages[i].Locale == "nl" {
			nlPrefixed = &capturedPages[i]
			break
		}
	}
	if nlPrefixed == nil {
		t.Fatal("no /nl page render found")
	}

	if nlPrefixed.LocalePrefix() != "/nl" {
		t.Errorf("NL LocalePrefix() = %q, want /nl", nlPrefixed.LocalePrefix())
	}
	if nlPrefixed.IsDefaultLocale() {
		t.Error("NL IsDefaultLocale() should be false")
	}

	// Find root EN render (no prefix).
	var enRoot *PageData
	for i := range capturedPages {
		if capturedPages[i].Path == "/" && capturedPages[i].Locale == "en" && capturedPages[i].localePrefix == "" {
			enRoot = &capturedPages[i]
			break
		}
	}
	if enRoot == nil {
		t.Fatal("no root / page render found")
	}
	if enRoot.LocalePrefix() != "" {
		t.Errorf("Root LocalePrefix() = %q, want empty", enRoot.LocalePrefix())
	}
}

func TestBuild_SingleLocale_NoPrefix(t *testing.T) {
	// Single locale CMS: should behave exactly as before (no prefixed paths).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/locales":
			json.NewEncoder(w).Encode([]apiLocaleResponse{
				{Locale: "en", Label: "English", IsDefault: true},
			})
		case r.URL.Path == "/api/v1/test/pages" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("About")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/about", testRender(func(p PageData) string { return p.Text("title") }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Should have about/index.html (no prefix).
	content, err := os.ReadFile(filepath.Join(outDir, "about", "index.html"))
	if err != nil {
		t.Fatal("about/index.html missing:", err)
	}
	if string(content) != "About" {
		t.Errorf("about/index.html = %q, want About", content)
	}

	// Should NOT have en/about/index.html.
	if _, err := os.Stat(filepath.Join(outDir, "en", "about", "index.html")); !os.IsNotExist(err) {
		t.Error("single locale should not create prefixed paths")
	}
}

func TestBuild_LocalesAPIFailure_FallsBackToSingleLocale(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/locales":
			w.WriteHeader(500) // Simulate API failure
		case r.URL.Path == "/api/v1/test/pages" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("About")},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/about", testRender(func(p PageData) string { return p.Text("title") }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Should fallback to single-locale behavior.
	content, err := os.ReadFile(filepath.Join(outDir, "about", "index.html"))
	if err != nil {
		t.Fatal("about/index.html missing:", err)
	}
	if string(content) != "About" {
		t.Errorf("about/index.html = %q, want About", content)
	}
}

func TestBuild_MultiLocale_CollectionEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale := r.URL.Query().Get("locale")
		if locale == "" {
			locale = "en"
		}
		switch {
		case r.URL.Path == "/api/v1/test/locales":
			json.NewEncoder(w).Encode([]apiLocaleResponse{
				{Locale: "en", Label: "English", IsDefault: true},
				{Locale: "nl", Label: "Nederlands", IsDefault: false},
			})
		case r.URL.Path == "/api/v1/test/pages" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/blog/hello", Slug: "hello", TemplateID: "t1"},
			})
		case r.URL.Path == "/api/v1/test/pages/blog/hello":
			title := "Hello World"
			if locale == "nl" {
				title = "Hallo Wereld"
			}
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog/hello", Slug: "hello",
				Fields: []apiFieldValue{
					{Key: "title", Locale: locale, Value: jsonVal(title)},
				},
			})
		case r.URL.Path == "/api/v1/test/pages/blog":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog", Slug: "blog",
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string {
			var titles []string
			for _, post := range p.Listing("blog") {
				titles = append(titles, post.Path+":"+post.Text("title"))
			}
			return strings.Join(titles, ",")
		}),
		testRender(func(p PageData) string { return p.Text("title") }),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// EN blog entry.
	enEntry, err := os.ReadFile(filepath.Join(outDir, "en", "blog", "hello", "index.html"))
	if err != nil {
		t.Fatal("en/blog/hello/index.html missing:", err)
	}
	if string(enEntry) != "Hello World" {
		t.Errorf("en blog entry = %q", enEntry)
	}

	// NL blog entry.
	nlEntry, err := os.ReadFile(filepath.Join(outDir, "nl", "blog", "hello", "index.html"))
	if err != nil {
		t.Fatal("nl/blog/hello/index.html missing:", err)
	}
	if string(nlEntry) != "Hallo Wereld" {
		t.Errorf("nl blog entry = %q", nlEntry)
	}

	// EN listing should have EN-prefixed paths.
	enListing, err := os.ReadFile(filepath.Join(outDir, "en", "blog", "index.html"))
	if err != nil {
		t.Fatal("en/blog/index.html missing:", err)
	}
	if !strings.Contains(string(enListing), "/en/blog/hello:Hello World") {
		t.Errorf("en listing = %q, want /en/blog/hello:Hello World", enListing)
	}

	// NL listing should have NL-prefixed paths.
	nlListing, err := os.ReadFile(filepath.Join(outDir, "nl", "blog", "index.html"))
	if err != nil {
		t.Fatal("nl/blog/index.html missing:", err)
	}
	if !strings.Contains(string(nlListing), "/nl/blog/hello:Hallo Wereld") {
		t.Errorf("nl listing = %q, want /nl/blog/hello:Hallo Wereld", nlListing)
	}

	// Root listing (default locale) should have unprefixed paths.
	rootListing, err := os.ReadFile(filepath.Join(outDir, "blog", "index.html"))
	if err != nil {
		t.Fatal("blog/index.html missing:", err)
	}
	if !strings.Contains(string(rootListing), "/blog/hello:Hello World") {
		t.Errorf("root listing = %q, want /blog/hello:Hello World", rootListing)
	}
}

func TestBuild_MultiLocale_URLAutoPrefixing(t *testing.T) {
	srv := multiLocaleCMS(t)
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		return p.URLOr("nav", "/features")
	}))
	app.Page("/about", testRender(func(p PageData) string { return "ok" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// EN prefixed: internal links should be prefixed.
	enHome, _ := os.ReadFile(filepath.Join(outDir, "en", "index.html"))
	if string(enHome) != "/en/features" {
		t.Errorf("en/index.html = %q, want /en/features", enHome)
	}

	// NL prefixed: internal links should use /nl prefix.
	nlHome, _ := os.ReadFile(filepath.Join(outDir, "nl", "index.html"))
	if string(nlHome) != "/nl/features" {
		t.Errorf("nl/index.html = %q, want /nl/features", nlHome)
	}

	// Root (default): internal links should have no prefix.
	rootHome, _ := os.ReadFile(filepath.Join(outDir, "index.html"))
	if string(rootHome) != "/features" {
		t.Errorf("root index.html = %q, want /features", rootHome)
	}
}

func TestBuild_MultiLocale_HreflangInOutput(t *testing.T) {
	srv := multiLocaleCMS(t)
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Page("/", testRender(func(p PageData) string {
		// Simulate what SEOHead would produce for hreflang.
		var links string
		if len(p.Locales) > 1 {
			for _, loc := range p.Locales {
				links += fmt.Sprintf(`<link rel="alternate" hreflang="%s" href="%s"/>`, loc.Code, p.PrefixedAlternatePath(loc.Code))
			}
			links += fmt.Sprintf(`<link rel="alternate" hreflang="x-default" href="%s"/>`, p.ContentPath())
		}
		return "<head>" + links + "</head>"
	}))
	app.Page("/about", testRender(func(p PageData) string { return "ok" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	enHome, _ := os.ReadFile(filepath.Join(outDir, "en", "index.html"))
	html := string(enHome)

	if !strings.Contains(html, `hreflang="en" href="/en"`) {
		t.Errorf("missing en hreflang in: %s", html)
	}
	if !strings.Contains(html, `hreflang="nl" href="/nl"`) {
		t.Errorf("missing nl hreflang in: %s", html)
	}
	if !strings.Contains(html, `hreflang="x-default" href="/"`) {
		t.Errorf("missing x-default hreflang in: %s", html)
	}

	// Root page should also have hreflang tags.
	rootHome, _ := os.ReadFile(filepath.Join(outDir, "index.html"))
	rootHTML := string(rootHome)
	if !strings.Contains(rootHTML, `hreflang="en" href="/en"`) {
		t.Errorf("root page missing en hreflang in: %s", rootHTML)
	}
}

// ---------------------------------------------------------------------------
// Layout fragment & route manifest build tests
// ---------------------------------------------------------------------------

// testLayout creates a LayoutFunc that wraps content in identifiable markers.
// Duplicated from app_test.go for use in build tests.
func testLayoutBuild(id string) LayoutFunc {
	return func(p PageData, body templ.Component) templ.Component {
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			io.WriteString(w, "<"+id+`-layout data-layout="`+id+`">`)
			if err := body.Render(ctx, w); err != nil {
				return err
			}
			io.WriteString(w, "</"+id+"-layout>")
			return nil
		})
	}
}

func TestPathToFragmentFile_Root(t *testing.T) {
	got := pathToFragmentFile("/out", "/", "root")
	want := filepath.Join("/out", "_root.html")
	if got != want {
		t.Errorf("pathToFragmentFile('/', 'root') = %q, want %q", got, want)
	}
}

func TestPathToFragmentFile_Nested(t *testing.T) {
	got := pathToFragmentFile("/out", "/about", "root")
	want := filepath.Join("/out", "about", "_root.html")
	if got != want {
		t.Errorf("pathToFragmentFile('/about', 'root') = %q, want %q", got, want)
	}
}

func TestPathToFragmentFile_DeepNested(t *testing.T) {
	got := pathToFragmentFile("/out", "/blog/my-post", "blog")
	want := filepath.Join("/out", "blog", "my-post", "_blog.html")
	if got != want {
		t.Errorf("pathToFragmentFile('/blog/my-post', 'blog') = %q, want %q", got, want)
	}
}

func TestBuild_WithLayouts_GeneratesFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/", Slug: "home",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("Home")},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/" || r.URL.Path == "/api/v1/test/seo//":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "Home Page"})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("About")},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/about":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "About Us"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/", testRender(func(p PageData) string { return "<h1>" + p.Text("title") + "</h1>" }))
	app.Page("/about", testRender(func(p PageData) string { return "<h1>" + p.Text("title") + "</h1>" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Full HTML files should exist with layout wrapping.
	homeHTML, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(homeHTML), `<root-layout data-layout="root">`) {
		t.Errorf("index.html should contain layout wrapping, got: %q", homeHTML)
	}
	if !strings.Contains(string(homeHTML), "<h1>Home</h1>") {
		t.Errorf("index.html should contain content, got: %q", homeHTML)
	}

	// Fragment files should exist.
	homeFrag, err := os.ReadFile(filepath.Join(outDir, "_root.html"))
	if err != nil {
		t.Fatal("_root.html fragment missing:", err)
	}
	fragStr := string(homeFrag)
	// Fragment should contain route metadata.
	if !strings.Contains(fragStr, "<!--route:") {
		t.Errorf("fragment should contain route metadata, got: %q", fragStr)
	}
	// Fragment should contain the content but NOT the layout wrapping.
	if strings.Contains(fragStr, "<root-layout") {
		t.Errorf("root fragment should NOT contain root layout wrapping, got: %q", fragStr)
	}
	if !strings.Contains(fragStr, "<h1>Home</h1>") {
		t.Errorf("fragment should contain page content, got: %q", fragStr)
	}

	// About page fragment.
	aboutFrag, err := os.ReadFile(filepath.Join(outDir, "about", "_root.html"))
	if err != nil {
		t.Fatal("about/_root.html fragment missing:", err)
	}
	if !strings.Contains(string(aboutFrag), "<h1>About</h1>") {
		t.Errorf("about fragment should contain content, got: %q", aboutFrag)
	}
}

func TestBuild_WithLayouts_NestedFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{
				{ID: "p1", Path: "/blog/post", Slug: "post", TemplateID: "t1"},
			})
		case r.URL.Path == "/api/v1/test/pages/blog":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog", Slug: "blog",
			})
		case r.URL.Path == "/api/v1/test/pages/blog/post":
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/blog/post", Slug: "post",
				Fields: []apiFieldValue{
					{Key: "title", Locale: "en", Value: jsonVal("My Post")},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/blog/post":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "My Post"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Layout("/blog", "blog", testLayoutBuild("blog"))
	app.Collection("/blog", "Blog",
		testRender(func(p PageData) string { return "listing" }),
		testRender(func(p PageData) string { return "entry:" + p.Text("title") }),
	)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Blog entry should have both _root.html and _blog.html fragments.
	rootFrag, err := os.ReadFile(filepath.Join(outDir, "blog", "post", "_root.html"))
	if err != nil {
		t.Fatal("blog/post/_root.html missing:", err)
	}
	rootFragStr := string(rootFrag)
	// Root fragment should contain blog layout wrapping entry content.
	if !strings.Contains(rootFragStr, `<blog-layout data-layout="blog">`) {
		t.Errorf("root fragment should contain blog layout, got: %q", rootFragStr)
	}
	if !strings.Contains(rootFragStr, "entry:My Post") {
		t.Errorf("root fragment should contain entry content, got: %q", rootFragStr)
	}

	blogFrag, err := os.ReadFile(filepath.Join(outDir, "blog", "post", "_blog.html"))
	if err != nil {
		t.Fatal("blog/post/_blog.html missing:", err)
	}
	blogFragStr := string(blogFrag)
	// Blog fragment should contain just the entry content, no layout wrapping.
	if strings.Contains(blogFragStr, "<blog-layout") {
		t.Errorf("blog fragment should NOT contain blog layout, got: %q", blogFragStr)
	}
	if strings.Contains(blogFragStr, "<root-layout") {
		t.Errorf("blog fragment should NOT contain root layout, got: %q", blogFragStr)
	}
	if !strings.Contains(blogFragStr, "entry:My Post") {
		t.Errorf("blog fragment should contain entry content, got: %q", blogFragStr)
	}
}

func TestBuild_WithLayouts_FragmentMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/" || r.URL.Path == "/api/v1/test/pages//":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/", Slug: "home"})
		case r.URL.Path == "/api/v1/test/seo/" || r.URL.Path == "/api/v1/test/seo//":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "Welcome Home"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	frag, err := os.ReadFile(filepath.Join(outDir, "_root.html"))
	if err != nil {
		t.Fatal(err)
	}
	fragStr := string(frag)

	// Should contain route metadata with title from SEO.
	if !strings.Contains(fragStr, `<!--route:{"t":"Welcome Home"}-->`) {
		t.Errorf("fragment should contain route metadata with SEO title, got: %q", fragStr)
	}
}

func TestBuild_WithLayouts_FragmentMetadata_FallsBackToSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/about":
			json.NewEncoder(w).Encode(apiPageResponse{Path: "/about", Slug: "about"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/about", testRender(func(p PageData) string { return "about" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	frag, err := os.ReadFile(filepath.Join(outDir, "about", "_root.html"))
	if err != nil {
		t.Fatal(err)
	}

	// No SEO title → should fall back to slug.
	if !strings.Contains(string(frag), `<!--route:{"t":"about"}-->`) {
		t.Errorf("fragment should fall back to slug for title, got: %q", frag)
	}
}

func TestBuild_WithLayouts_WritesRouteManifest(t *testing.T) {
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
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Layout("/blog", "blog", testLayoutBuild("blog"))
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// _routes.json should exist.
	data, err := os.ReadFile(filepath.Join(outDir, "_routes.json"))
	if err != nil {
		t.Fatal("_routes.json missing:", err)
	}

	var manifest struct {
		Layouts map[string]string `json:"layouts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal("invalid JSON:", err)
	}

	if manifest.Layouts["/"] != "root" {
		t.Errorf("layouts['/'] = %q, want 'root'", manifest.Layouts["/"])
	}
	if manifest.Layouts["/blog"] != "blog" {
		t.Errorf("layouts['/blog'] = %q, want 'blog'", manifest.Layouts["/blog"])
	}
}

func TestBuild_NoLayouts_NoRouteManifest(t *testing.T) {
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
	// No layouts registered.
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// _routes.json should NOT exist.
	if _, err := os.Stat(filepath.Join(outDir, "_routes.json")); !os.IsNotExist(err) {
		t.Error("_routes.json should not exist when no layouts registered")
	}
}

func TestBuild_NoLayouts_NoFragments(t *testing.T) {
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
	app.Page("/", testRender(func(p PageData) string { return "home" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// No fragment files should exist.
	if _, err := os.Stat(filepath.Join(outDir, "_root.html")); !os.IsNotExist(err) {
		t.Error("fragment files should not exist when no layouts registered")
	}
}

func TestBuild_WithLayouts_CMSAttributesStrippedInFragments(t *testing.T) {
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
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/", testRender(func(p PageData) string {
		return `<h1 data-cms-field="title" data-cms-type="text">Welcome</h1>`
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	frag, err := os.ReadFile(filepath.Join(outDir, "_root.html"))
	if err != nil {
		t.Fatal(err)
	}
	fragStr := string(frag)

	// CMS attributes should be stripped from fragments.
	if strings.Contains(fragStr, "data-cms-") {
		t.Errorf("fragment should not contain data-cms-* attributes, got: %q", fragStr)
	}
	// But data-layout is NOT a CMS attribute and should survive.
	if !strings.Contains(fragStr, "<h1>Welcome</h1>") {
		t.Errorf("fragment should contain clean content, got: %q", fragStr)
	}
}

func TestBuild_WithLayouts_DataLayoutSurvivesStripping(t *testing.T) {
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
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/", testRender(func(p PageData) string { return "content" }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	html, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	// data-layout is NOT a CMS attribute — should survive stripping.
	if !strings.Contains(string(html), `data-layout="root"`) {
		t.Errorf("data-layout should survive stripCMSAttributes, got: %q", html)
	}
}

func TestBuild_WithLayouts_PageDataHasLayoutManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/test/pages" && r.Method == "GET":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	var capturedManifests []map[string]string

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Layout("/blog", "blog", testLayoutBuild("blog"))
	app.Page("/", testRender(func(p PageData) string {
		capturedManifests = append(capturedManifests, p.LayoutManifest())
		return "home"
	}))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// The render func is called multiple times (build + template file).
	// At least one invocation should have the layout manifest set.
	var found bool
	for _, m := range capturedManifests {
		if m != nil && m["/"] == "root" && m["/blog"] == "blog" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one render with layout manifest set, got: %v", capturedManifests)
	}
}

func TestBuild_MultiLocale_WithLayouts_GeneratesFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale := r.URL.Query().Get("locale")
		if locale == "" {
			locale = "en"
		}
		switch {
		case r.URL.Path == "/api/v1/test/locales":
			json.NewEncoder(w).Encode([]apiLocaleResponse{
				{Locale: "en", Label: "English", IsDefault: true},
				{Locale: "nl", Label: "Nederlands", IsDefault: false},
			})
		case r.URL.Path == "/api/v1/test/pages" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode([]apiPageListItem{})
		case r.URL.Path == "/api/v1/test/pages/about":
			title := "About"
			if locale == "nl" {
				title = "Over"
			}
			json.NewEncoder(w).Encode(apiPageResponse{
				Path: "/about", Slug: "about",
				Fields: []apiFieldValue{
					{Key: "title", Locale: locale, Value: jsonVal(title)},
				},
			})
		case r.URL.Path == "/api/v1/test/seo/about":
			json.NewEncoder(w).Encode(apiSEOResponse{MetaTitle: "About"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	app.Layout("/", "root", testLayoutBuild("root"))
	app.Page("/about", testRender(func(p PageData) string { return p.Text("title") }))

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// EN prefixed fragment.
	enFrag, err := os.ReadFile(filepath.Join(outDir, "en", "about", "_root.html"))
	if err != nil {
		t.Fatal("en/about/_root.html missing:", err)
	}
	if !strings.Contains(string(enFrag), "About") {
		t.Errorf("EN fragment should contain 'About', got: %q", enFrag)
	}

	// NL prefixed fragment.
	nlFrag, err := os.ReadFile(filepath.Join(outDir, "nl", "about", "_root.html"))
	if err != nil {
		t.Fatal("nl/about/_root.html missing:", err)
	}
	if !strings.Contains(string(nlFrag), "Over") {
		t.Errorf("NL fragment should contain 'Over', got: %q", nlFrag)
	}

	// Root (default locale) fragment.
	rootFrag, err := os.ReadFile(filepath.Join(outDir, "about", "_root.html"))
	if err != nil {
		t.Fatal("about/_root.html missing:", err)
	}
	if !strings.Contains(string(rootFrag), "About") {
		t.Errorf("Root fragment should contain 'About', got: %q", rootFrag)
	}

	// Route manifest should exist.
	manifest, err := os.ReadFile(filepath.Join(outDir, "_routes.json"))
	if err != nil {
		t.Fatal("_routes.json missing:", err)
	}
	if !strings.Contains(string(manifest), `"root"`) {
		t.Errorf("manifest missing 'root': %q", manifest)
	}
}

// ---------------------------------------------------------------------------
// Error page path convention tests (404.html, 500.html)
// ---------------------------------------------------------------------------

func TestPathToFile_ErrorPages(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/404", filepath.Join("/out", "404.html")},
		{"/500", filepath.Join("/out", "500.html")},
		{"/en/404", filepath.Join("/out", "en", "404.html")},
		{"/nl/500", filepath.Join("/out", "nl", "500.html")},
	}
	for _, tt := range tests {
		got := pathToFile("/out", tt.path)
		if got != tt.want {
			t.Errorf("pathToFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestPathToTemplateFile_ErrorPages(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/404", filepath.Join("/out", "404.template.html")},
		{"/500", filepath.Join("/out", "500.template.html")},
	}
	for _, tt := range tests {
		got := pathToTemplateFile("/out", tt.path)
		if got != tt.want {
			t.Errorf("pathToTemplateFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestBuild_404Page_WritesAs404HTML(t *testing.T) {
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
	app.Page("/404", testRender(func(p PageData) string { return "<html>Not Found</html>" }), NoSitemap)

	outDir := t.TempDir()
	err := app.Build(context.Background(), BuildOptions{OutDir: outDir})
	if err != nil {
		t.Fatal(err)
	}

	// Should be written as 404.html (not 404/index.html).
	content, err := os.ReadFile(filepath.Join(outDir, "404.html"))
	if err != nil {
		t.Fatal("404.html not found:", err)
	}
	if string(content) != "<html>Not Found</html>" {
		t.Errorf("404.html = %q", content)
	}

	// Should NOT exist as 404/index.html.
	if _, err := os.Stat(filepath.Join(outDir, "404", "index.html")); !os.IsNotExist(err) {
		t.Error("404/index.html should not exist")
	}
}

func TestLocalePrefixPath(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		want   string
	}{
		{"/en", "/", "/en"},
		{"/en", "/about", "/en/about"},
		{"/nl", "/blog/post", "/nl/blog/post"},
		{"/fr", "/a/b/c", "/fr/a/b/c"},
	}
	for _, tt := range tests {
		got := localePrefixPath(tt.prefix, tt.path)
		if got != tt.want {
			t.Errorf("localePrefixPath(%q, %q) = %q, want %q", tt.prefix, tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Static directory copying tests
// ---------------------------------------------------------------------------

func TestCopyStaticDir_CopiesFiles(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "static")
	dstDir := filepath.Join(t.TempDir(), "dist")

	// Create source files.
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "favicon.svg"), []byte("<svg/>"), 0o644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "sub", "style.css"), []byte("body{}"), 0o644)

	if err := copyStaticDir(srcDir, dstDir); err != nil {
		t.Fatal(err)
	}

	// Check files were copied.
	data, err := os.ReadFile(filepath.Join(dstDir, "favicon.svg"))
	if err != nil {
		t.Fatal("favicon.svg not copied:", err)
	}
	if string(data) != "<svg/>" {
		t.Errorf("favicon.svg = %q", data)
	}

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "style.css"))
	if err != nil {
		t.Fatal("sub/style.css not copied:", err)
	}
	if string(data) != "body{}" {
		t.Errorf("sub/style.css = %q", data)
	}
}

func TestCopyStaticDir_MissingDir_NoError(t *testing.T) {
	dstDir := t.TempDir()
	err := copyStaticDir("/nonexistent/static", dstDir)
	if err != nil {
		t.Errorf("expected no error for missing dir, got: %v", err)
	}
}

func TestCopyStaticDir_EmptyDir_NoError(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "static")
	dstDir := t.TempDir()
	os.MkdirAll(srcDir, 0o755)

	err := copyStaticDir(srcDir, dstDir)
	if err != nil {
		t.Errorf("expected no error for empty dir, got: %v", err)
	}
}
