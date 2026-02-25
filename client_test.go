package cms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockCMS sets up a fake CMS API and returns a configured Client.
func mockCMS(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(Config{
		APIURL:   srv.URL,
		SiteSlug: "test-site",
		APIKey:   "test-key",
		Locale:   "en",
	})
}

func TestClient_GetPage_ReturnsPageData(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/api/v1/test-site/pages/home" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("locale") != "en" {
			t.Errorf("expected locale=en, got %s", r.URL.Query().Get("locale"))
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}

		json.NewEncoder(w).Encode(apiPageResponse{
			ID:            "page-1",
			Path:          "/home",
			Slug:          "home",
			VersionID:     "v-1",
			VersionNumber: 1,
			Fields: []apiFieldValue{
				{Key: "title", Locale: "en", Value: jsonVal("Hello World")},
				{Key: "body", Locale: "en", Value: jsonVal("<p>Content</p>")},
				{Key: "hero", Locale: "en", Value: jsonVal(map[string]any{
					"url": "https://cdn.test/hero.jpg",
					"alt": "Hero",
				})},
			},
		})
	}))

	page, err := client.GetPage(context.Background(), "/home")
	if err != nil {
		t.Fatal(err)
	}

	if page.Path != "/home" {
		t.Errorf("Path = %q", page.Path)
	}
	if page.Slug != "home" {
		t.Errorf("Slug = %q", page.Slug)
	}
	if page.Text("title") != "Hello World" {
		t.Errorf("Text('title') = %q", page.Text("title"))
	}
	if page.Text("body") != "<p>Content</p>" {
		t.Errorf("Text('body') = %q", page.Text("body"))
	}
	img := page.Image("hero")
	if img.URL != "https://cdn.test/hero.jpg" {
		t.Errorf("Image('hero').URL = %q", img.URL)
	}
}

func TestClient_GetPage_SendsAPIKeyHeader(t *testing.T) {
	var gotKey string
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		json.NewEncoder(w).Encode(apiPageResponse{
			Path: "/", Slug: "home",
		})
	}))

	_, _ = client.GetPage(context.Background(), "/")
	if gotKey != "test-key" {
		t.Errorf("X-API-Key = %q, want test-key", gotKey)
	}
}

func TestClient_GetPage_NormalizesPath(t *testing.T) {
	var gotPath string
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(apiPageResponse{Path: "/about", Slug: "about"})
	}))

	_, _ = client.GetPage(context.Background(), "/about")
	if gotPath != "/api/v1/test-site/pages/about" {
		t.Errorf("path = %q, want /api/v1/test-site/pages/about", gotPath)
	}
}

func TestClient_GetPage_RootPath(t *testing.T) {
	var gotPath string
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(apiPageResponse{Path: "/", Slug: ""})
	}))

	_, _ = client.GetPage(context.Background(), "/")
	if gotPath != "/api/v1/test-site/pages//" {
		t.Errorf("path = %q, want /api/v1/test-site/pages//", gotPath)
	}
}

func TestClient_GetPage_404_ReturnsError(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))

	_, err := client.GetPage(context.Background(), "/missing")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestClient_GetPage_WithLocale(t *testing.T) {
	var gotLocale string
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLocale = r.URL.Query().Get("locale")
		json.NewEncoder(w).Encode(apiPageResponse{Path: "/", Slug: "home"})
	}))

	_, _ = client.GetPage(context.Background(), "/", WithLocale("fr"))
	if gotLocale != "fr" {
		t.Errorf("locale = %q, want fr", gotLocale)
	}
}

func TestClient_GetSEO_ReturnsData(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/test-site/seo/about" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apiSEOResponse{
			MetaTitle:       "About — My Site",
			MetaDescription: "Learn more about us",
			OGImageURL:      "https://cdn.test/og.jpg",
		})
	}))

	seo, err := client.GetSEO(context.Background(), "/about")
	if err != nil {
		t.Fatal(err)
	}

	if seo.MetaTitle != "About — My Site" {
		t.Errorf("MetaTitle = %q", seo.MetaTitle)
	}
	if seo.MetaDescription != "Learn more about us" {
		t.Errorf("MetaDescription = %q", seo.MetaDescription)
	}
	if seo.OGImageURL != "https://cdn.test/og.jpg" {
		t.Errorf("OGImageURL = %q", seo.OGImageURL)
	}
}

func TestClient_GetMediaURL_ReturnsURL(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/test-site/media/media-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify image processing params forwarded
		if r.URL.Query().Get("w") != "800" {
			t.Errorf("expected w=800, got %s", r.URL.Query().Get("w"))
		}
		json.NewEncoder(w).Encode(apiMediaResponse{
			URL: "https://cdn.test/signed/hero.jpg?w=800",
		})
	}))

	url, err := client.GetMediaURL(context.Background(), "media-123", Width(800))
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn.test/signed/hero.jpg?w=800" {
		t.Errorf("URL = %q", url)
	}
}

func TestClient_GetEmailTemplate_ReturnsContent(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/test-site/emails/welcome" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apiEmailTemplateResponse{
			Key:     "welcome",
			Label:   "Welcome Email",
			Subject: "Welcome!",
			Fields: []apiEmailFieldValue{
				{Key: "greeting", Locale: "en", Value: jsonVal("Hello, friend!")},
			},
		})
	}))

	et, err := client.GetEmailTemplate(context.Background(), "welcome")
	if err != nil {
		t.Fatal(err)
	}
	if et.Key != "welcome" {
		t.Errorf("Key = %q", et.Key)
	}
	if et.Subject != "Welcome!" {
		t.Errorf("Subject = %q", et.Subject)
	}
	if et.Text("greeting") != "Hello, friend!" {
		t.Errorf("Text('greeting') = %q", et.Text("greeting"))
	}
}

func TestClient_ListPages_ReturnsPaths(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/test-site/pages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]apiPageListItem{
			{ID: "p1", Path: "/", Slug: "home", TemplateID: "t1"},
			{ID: "p2", Path: "/about", Slug: "about", TemplateID: "t1"},
		})
	}))

	pages, err := client.ListPages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Path != "/" || pages[1].Path != "/about" {
		t.Errorf("pages = %+v", pages)
	}
}

func TestClient_ListLocales_ReturnsLocales(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/test-site/locales" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}
		json.NewEncoder(w).Encode([]apiLocaleResponse{
			{Locale: "en", Label: "English", IsDefault: true},
			{Locale: "nl", Label: "Nederlands", IsDefault: false},
		})
	}))

	locales, err := client.ListLocales(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(locales) != 2 {
		t.Fatalf("expected 2 locales, got %d", len(locales))
	}
	if locales[0].Code != "en" || !locales[0].IsDefault {
		t.Errorf("locales[0] = %+v, want en/default", locales[0])
	}
	if locales[1].Code != "nl" || locales[1].IsDefault {
		t.Errorf("locales[1] = %+v, want nl/non-default", locales[1])
	}
	if locales[0].Label != "English" {
		t.Errorf("locales[0].Label = %q, want English", locales[0].Label)
	}
}

func TestClient_ListLocales_SingleLocale(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]apiLocaleResponse{
			{Locale: "en", Label: "English", IsDefault: true},
		})
	}))

	locales, err := client.ListLocales(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(locales) != 1 {
		t.Fatalf("expected 1 locale, got %d", len(locales))
	}
}

func TestClient_ListLocales_Error(t *testing.T) {
	client := mockCMS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	_, err := client.ListLocales(context.Background())
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// jsonVal is defined in build_test.go
