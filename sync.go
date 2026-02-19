package cms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SyncPayload is the JSON payload sent to the CMS sync endpoint.
// It mirrors the backend's SyncRequest type exactly.
type SyncPayload struct {
	Pages          []SyncPage          `json:"pages"`
	Collections    []SyncCollection    `json:"collections,omitempty"`
	EmailTemplates []SyncEmailTemplate `json:"email_templates,omitempty"`
}

// SyncPage describes a fixed page for sync.
type SyncPage struct {
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
	// HTML is the pre-rendered page HTML for schema discovery.
	// When provided, the CMS parses this HTML to discover template
	// metadata, field definitions, and subcollection schemas instead
	// of crawling the live site.
	HTML string `json:"html,omitempty"`
}

// SyncCollection describes a collection for sync.
type SyncCollection struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	TemplateURL string `json:"template_url"`
	BasePath    string `json:"base_path"`
	// HTML is the pre-rendered template page HTML for schema discovery.
	HTML string `json:"html,omitempty"`
}

// SyncEmailTemplate describes an email template for sync.
type SyncEmailTemplate struct {
	Key       string          `json:"key"`
	Label     string          `json:"label"`
	Subject   string          `json:"subject,omitempty"`
	HTML      string          `json:"html"`
	Variables []EmailVariable `json:"variables,omitempty"`
}

// SyncPayload produces the JSON-serializable sync payload from all
// registered pages, collections, and email templates.
//
// Each page and collection template is rendered with empty PageData so
// the resulting HTML contains all data-cms-* attributes for schema
// discovery. SubcollectionOr ensures at least one entry element is
// present even when no CMS content exists yet, avoiding the
// chicken-and-egg problem with for-range loops over empty data.
func (a *App) SyncPayload() SyncPayload {
	// Fixed pages — render with empty data for schema discovery.
	pages := make([]SyncPage, 0, len(a.pages)+len(a.collections))
	for _, p := range a.pages {
		data := NewPageData(p.path, pathSlug(p.path), a.config.Locale, nil, nil, nil)
		html := a.renderPage(data)
		pages = append(pages, SyncPage{Path: p.path, Title: p.title, HTML: html})
	}

	// Collection listing pages.
	for _, c := range a.collections {
		data := NewPageData(c.basePath, pathSlug(c.basePath), a.config.Locale, nil, nil, nil)
		html := a.renderPage(data)
		pages = append(pages, SyncPage{
			Path:  c.basePath,
			Title: titleFromPath(c.basePath),
			HTML:  html,
		})
	}

	// Collections — render the template page for schema discovery.
	var collections []SyncCollection
	if len(a.collections) > 0 {
		collections = make([]SyncCollection, len(a.collections))
		for i, c := range a.collections {
			data := NewPageData(c.templateURL, pathSlug(c.templateURL), a.config.Locale, nil, nil, nil)
			html := a.renderPage(data)
			collections[i] = SyncCollection{
				Key:         c.key,
				Label:       c.label,
				TemplateURL: c.templateURL,
				BasePath:    c.basePath + "/:" + c.key,
				HTML:        html,
			}
		}
	}

	// Email templates.
	var emailTemplates []SyncEmailTemplate
	if len(a.emails) > 0 {
		emailTemplates = make([]SyncEmailTemplate, len(a.emails))
		for i, et := range a.emails {
			emailTemplates[i] = SyncEmailTemplate{
				Key:       et.key,
				Label:     et.label,
				Subject:   et.subject,
				HTML:      et.html,
				Variables: et.variables,
			}
		}
	}

	return SyncPayload{
		Pages:          pages,
		Collections:    collections,
		EmailTemplates: emailTemplates,
	}
}

// WriteSyncJSON writes the sync payload as pretty-printed JSON to the
// given file path. Parent directories are created if needed.
func (a *App) WriteSyncJSON(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	payload := a.SyncPayload()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// PostSync sends the sync payload to the CMS API.
// If filePath is non-empty, reads an existing JSON file; otherwise
// builds the payload from registered pages/collections.
func (a *App) PostSync(ctx context.Context, filePath string) error {
	var body []byte

	if filePath != "" {
		var err error
		body, err = os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("cms: read sync file: %w", err)
		}
	} else {
		payload := a.SyncPayload()
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("cms: marshal sync payload: %w", err)
		}
	}

	url := fmt.Sprintf("%s/api/v1/%s/sync",
		strings.TrimRight(a.config.APIURL, "/"),
		a.config.SiteSlug,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cms: create sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.config.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cms: sync request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cms: sync returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
