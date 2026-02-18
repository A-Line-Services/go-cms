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
