package cms

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDevFileHandler_ServesProductionByDefault(t *testing.T) {
	dir := t.TempDir()

	// Create production and template files.
	os.MkdirAll(filepath.Join(dir, "about"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>Production Home</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "index.template.html"), []byte(`<h1 data-cms-field="title">Template Home</h1>`), 0o644)
	os.WriteFile(filepath.Join(dir, "about", "index.html"), []byte("<h1>Production About</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "about", "index.template.html"), []byte(`<h1 data-cms-field="heading">Template About</h1>`), 0o644)

	handler := devFileHandler(dir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Normal request should get production HTML.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "<h1>Production Home</h1>" {
		t.Errorf("expected production HTML, got: %q", string(body))
	}

	// Normal request to /about should get production HTML.
	resp, err = http.Get(srv.URL + "/about")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "<h1>Production About</h1>" {
		t.Errorf("expected production about HTML, got: %q", string(body))
	}
}

func TestDevFileHandler_ServesTemplateForPreview(t *testing.T) {
	dir := t.TempDir()

	// Create production and template files.
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>Production Home</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "index.template.html"), []byte(`<h1 data-cms-field="title">Template Home</h1>`), 0o644)

	handler := devFileHandler(dir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Request with X-CMS-Preview header should get template HTML.
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("X-CMS-Preview", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != `<h1 data-cms-field="title">Template Home</h1>` {
		t.Errorf("expected template HTML, got: %q", string(body))
	}
}

func TestDevFileHandler_PreviewFallsBackToProduction(t *testing.T) {
	dir := t.TempDir()

	// Create only production file (no template).
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>Production</h1>"), 0o644)

	handler := devFileHandler(dir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Preview request should fall back to production when no template exists.
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("X-CMS-Preview", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "<h1>Production</h1>" {
		t.Errorf("expected fallback to production HTML, got: %q", string(body))
	}
}

func TestDevFileHandler_PreviewNestedPath(t *testing.T) {
	dir := t.TempDir()

	// Create nested path with both files.
	os.MkdirAll(filepath.Join(dir, "blog", "_template"), 0o755)
	os.WriteFile(filepath.Join(dir, "blog", "_template", "index.html"), []byte("<h1>Production Entry</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "blog", "_template", "index.template.html"), []byte(`<h1 data-cms-field="title">Template Entry</h1>`), 0o644)

	handler := devFileHandler(dir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Preview request to nested path.
	req, _ := http.NewRequest("GET", srv.URL+"/blog/_template", nil)
	req.Header.Set("X-CMS-Preview", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != `<h1 data-cms-field="title">Template Entry</h1>` {
		t.Errorf("expected template entry HTML, got: %q", string(body))
	}
}

func TestDevFileHandler_ServesStaticAssets(t *testing.T) {
	dir := t.TempDir()

	// Create a CSS file (static asset, not a clean URL).
	os.WriteFile(filepath.Join(dir, "style.css"), []byte("body { color: red }"), 0o644)

	handler := devFileHandler(dir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Static assets should be served directly regardless of preview header.
	req, _ := http.NewRequest("GET", srv.URL+"/style.css", nil)
	req.Header.Set("X-CMS-Preview", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "body { color: red }" {
		t.Errorf("expected CSS content, got: %q", string(body))
	}
}
