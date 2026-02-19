package cms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noop() RenderFunc {
	return testRender(func(p PageData) string { return "" })
}

func TestSyncPayload_Pages(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())
	app.Page("/about", noop())

	payload := app.SyncPayload()

	if len(payload.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(payload.Pages))
	}
	if payload.Pages[0].Path != "/" || payload.Pages[0].Title != "Home" {
		t.Errorf("page[0] = %+v", payload.Pages[0])
	}
	if payload.Pages[1].Path != "/about" || payload.Pages[1].Title != "About" {
		t.Errorf("page[1] = %+v", payload.Pages[1])
	}
}

func TestSyncPayload_PageTitle_Explicit(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.PageTitle("/", "Homepage", noop())

	payload := app.SyncPayload()

	if payload.Pages[0].Title != "Homepage" {
		t.Errorf("title = %q, want Homepage", payload.Pages[0].Title)
	}
}

func TestSyncPayload_Collections(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Collection("/blog", "Blog Posts", noop(), noop())

	payload := app.SyncPayload()

	if len(payload.Collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(payload.Collections))
	}
	c := payload.Collections[0]
	if c.Key != "blog" || c.Label != "Blog Posts" {
		t.Errorf("collection = %+v", c)
	}
	if c.TemplateURL != "/blog/_template" {
		t.Errorf("TemplateURL = %q, want /blog/_template", c.TemplateURL)
	}
	if c.BasePath != "/blog/:slug" {
		t.Errorf("BasePath = %q, want /blog/:slug", c.BasePath)
	}
}

func TestSyncPayload_CollectionListingInPages(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())
	app.Collection("/blog", "Blog", noop(), noop())

	payload := app.SyncPayload()

	// Should have 2 pages: / and /blog (collection listing auto-added).
	if len(payload.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(payload.Pages))
	}
	if payload.Pages[1].Path != "/blog" {
		t.Errorf("page[1].Path = %q, want /blog", payload.Pages[1].Path)
	}
}

func TestSyncPayload_EmailTemplates(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.EmailTemplate("welcome", "Welcome", "Hi!", "<h1>Welcome</h1>", []EmailVariable{
		{Key: "name", Description: "User name", SampleValue: "Alice"},
	})

	payload := app.SyncPayload()

	if len(payload.EmailTemplates) != 1 {
		t.Fatalf("expected 1 email template, got %d", len(payload.EmailTemplates))
	}
	et := payload.EmailTemplates[0]
	if et.Key != "welcome" || et.Label != "Welcome" {
		t.Errorf("email template = %+v", et)
	}
	if et.Subject != "Hi!" {
		t.Errorf("Subject = %q", et.Subject)
	}
	if et.HTML != "<h1>Welcome</h1>" {
		t.Errorf("HTML = %q", et.HTML)
	}
	if len(et.Variables) != 1 || et.Variables[0].Key != "name" {
		t.Errorf("Variables = %+v", et.Variables)
	}
}

func TestSyncPayload_EmptyCollectionsOmitted(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())

	payload := app.SyncPayload()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	if _, ok := raw["pages"]; !ok {
		t.Error("expected 'pages' key in JSON")
	}
	if _, ok := raw["collections"]; ok {
		t.Error("expected 'collections' to be omitted when empty")
	}
	if _, ok := raw["email_templates"]; ok {
		t.Error("expected 'email_templates' to be omitted when empty")
	}
}

func TestSyncPayload_JSON_MatchesCMSFormat(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())
	app.Collection("/blog", "Blog", noop(), noop())

	payload := app.SyncPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Pages       []struct{ Path, Title string }          `json:"pages"`
		Collections []struct{ Key, Label, BasePath string } `json:"collections"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Pages) != 2 || parsed.Pages[0].Path != "/" {
		t.Errorf("pages round-trip: %+v", parsed.Pages)
	}
	if len(parsed.Collections) != 1 || parsed.Collections[0].Key != "blog" {
		t.Errorf("collections round-trip: %+v", parsed.Collections)
	}
}

func TestWriteSyncJSON_WritesFile(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.json")

	if err := app.WriteSyncJSON(path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var payload SyncPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Pages) != 1 || payload.Pages[0].Path != "/" {
		t.Errorf("written payload = %+v", payload)
	}
}

func TestWriteSyncJSON_PrettyPrinted(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.json")
	_ = app.WriteSyncJSON(path)

	data, _ := os.ReadFile(path)
	if len(data) > 0 && data[0] != '{' {
		t.Error("expected JSON to start with '{'")
	}
	found := false
	for _, b := range data {
		if b == '\n' {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pretty-printed JSON with newlines")
	}
}

func TestPostSync_SendsPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("API key = %q", r.Header.Get("X-API-Key"))
		}
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		received = body
		w.WriteHeader(200)
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test-site", APIKey: "test-key"})
	app.Page("/", noop())

	err := app.PostSync(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	var payload SyncPayload
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("received invalid JSON: %v", err)
	}
	if len(payload.Pages) != 1 {
		t.Errorf("payload pages = %d", len(payload.Pages))
	}
}

func TestPostSync_SendsFile(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		received = string(body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Write a test sync file.
	dir := t.TempDir()
	syncFile := filepath.Join(dir, "sync.json")
	os.WriteFile(syncFile, []byte(`{"pages":[]}`), 0o644)

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test-site", APIKey: "test-key"})
	err := app.PostSync(context.Background(), syncFile)
	if err != nil {
		t.Fatal(err)
	}

	if received != `{"pages":[]}` {
		t.Errorf("received = %q", received)
	}
}

func TestPostSync_ErrorOnHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	app := NewApp(Config{APIURL: srv.URL, SiteSlug: "test", APIKey: "k"})
	err := app.PostSync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// Inline HTML in sync payload
// ---------------------------------------------------------------------------

func TestSyncPayload_PageHTML_ContainsRenderedTemplate(t *testing.T) {
	render := testRender(func(p PageData) string {
		var b strings.Builder
		b.WriteString(`<html><head>`)
		b.WriteString(`<meta name="cms-template" content="homepage">`)
		b.WriteString(`</head><body>`)
		b.WriteString(`<div data-cms-subcollection="partners" data-cms-label="Partners">`)
		for _, entry := range p.SubcollectionOr("partners") {
			b.WriteString(`<div data-cms-entry>`)
			b.WriteString(`<span data-cms-field="name" data-cms-type="text" data-cms-label="Name">`)
			b.WriteString(entry.TextOr("name", "Partner"))
			b.WriteString(`</span>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</body></html>`)
		return b.String()
	})

	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", render)

	payload := app.SyncPayload()

	if len(payload.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(payload.Pages))
	}
	page := payload.Pages[0]
	if page.HTML == "" {
		t.Fatal("expected non-empty HTML")
	}
	if !strings.Contains(page.HTML, `data-cms-subcollection="partners"`) {
		t.Error("HTML missing subcollection attribute")
	}
	if !strings.Contains(page.HTML, `data-cms-entry`) {
		t.Error("HTML missing entry attribute — SubcollectionOr should render one")
	}
	if !strings.Contains(page.HTML, `data-cms-field="name"`) {
		t.Error("HTML missing field attribute inside entry")
	}
}

func TestSyncPayload_CollectionHTML_ContainsRenderedTemplate(t *testing.T) {
	listing := noop()
	entry := testRender(func(p PageData) string {
		var b strings.Builder
		b.WriteString(`<html><head>`)
		b.WriteString(`<meta name="cms-template" content="blog-entry">`)
		b.WriteString(`<meta name="cms-collection" content="blog">`)
		b.WriteString(`</head><body>`)
		b.WriteString(`<h1 data-cms-field="title" data-cms-type="text">`)
		b.WriteString(p.TextOr("title", "Post Title"))
		b.WriteString(`</h1>`)
		b.WriteString(`</body></html>`)
		return b.String()
	})

	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Collection("/blog", "Blog Posts", listing, entry)

	payload := app.SyncPayload()

	if len(payload.Collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(payload.Collections))
	}
	coll := payload.Collections[0]
	if coll.HTML == "" {
		t.Fatal("expected non-empty collection template HTML")
	}
	if !strings.Contains(coll.HTML, `data-cms-field="title"`) {
		t.Error("collection template HTML missing field attribute")
	}
}

func TestSyncPayload_PageHTML_OmittedWhenEmpty(t *testing.T) {
	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", noop())

	payload := app.SyncPayload()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	// noop() renders empty string, so html should be omitted from JSON
	if strings.Contains(string(data), `"html"`) {
		t.Error("expected html to be omitted when render returns empty string")
	}
}

func TestSyncPayload_SubcollectionOr_GuaranteesEntryInHTML(t *testing.T) {
	// Even with zero CMS data, SubcollectionOr renders one empty entry.
	// This test verifies the contract that makes inline HTML sync work.
	render := testRender(func(p PageData) string {
		var b strings.Builder
		b.WriteString(`<div data-cms-subcollection="team" data-cms-label="Team">`)
		for _, entry := range p.SubcollectionOr("team") {
			b.WriteString(`<div data-cms-entry>`)
			b.WriteString(`<span data-cms-field="name" data-cms-type="text" data-cms-label="Name">`)
			b.WriteString(entry.TextOr("name", "Member"))
			b.WriteString(`</span>`)
			// Nested subcollection
			b.WriteString(`<div data-cms-subcollection="skills" data-cms-label="Skills">`)
			for _, skill := range entry.SubcollectionOr("skills") {
				b.WriteString(`<div data-cms-entry>`)
				b.WriteString(`<span data-cms-field="skill" data-cms-type="text" data-cms-label="Skill">`)
				b.WriteString(skill.TextOr("skill", "Skill"))
				b.WriteString(`</span>`)
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		return b.String()
	})

	app := NewApp(Config{APIURL: "https://cms.test", SiteSlug: "s", APIKey: "k"})
	app.Page("/", render)

	payload := app.SyncPayload()
	html := payload.Pages[0].HTML

	// Top-level subcollection discovered
	if !strings.Contains(html, `data-cms-subcollection="team"`) {
		t.Error("missing top-level subcollection")
	}
	// At least one entry rendered
	if !strings.Contains(html, `data-cms-entry`) {
		t.Error("missing entry element — SubcollectionOr should guarantee one")
	}
	// Nested subcollection discovered inside the entry
	if !strings.Contains(html, `data-cms-subcollection="skills"`) {
		t.Error("missing nested subcollection")
	}
	// Nested entry's field discovered
	if !strings.Contains(html, `data-cms-field="skill"`) {
		t.Error("missing nested field")
	}
}
