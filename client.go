package cms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// API response types (match CMS public API JSON shape)
// ---------------------------------------------------------------------------

type apiPageListItem struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Slug       string `json:"slug"`
	TemplateID string `json:"template_id"`
}

type apiPageResponse struct {
	ID             string             `json:"id"`
	Path           string             `json:"path"`
	Slug           string             `json:"slug"`
	VersionID      string             `json:"version_id"`
	VersionNumber  int                `json:"version_number"`
	Fields         []apiFieldValue    `json:"fields"`
	Subcollections []apiSubcollection `json:"subcollections,omitempty"`
}

type apiFieldValue struct {
	Key               string          `json:"key"`
	FieldDefinitionID string          `json:"field_definition_id"`
	Locale            string          `json:"locale"`
	Value             json.RawMessage `json:"value"`
}

type apiSubcollection struct {
	SubcollectionID string                  `json:"subcollection_id"`
	Key             string                  `json:"key"`
	Entries         []apiSubcollectionEntry `json:"entries"`
}

type apiSubcollectionEntry struct {
	ID             string             `json:"id"`
	Fields         []apiFieldValue    `json:"fields"`
	Subcollections []apiSubcollection `json:"subcollections,omitempty"`
}

type apiSEOResponse struct {
	MetaTitle       string `json:"meta_title"`
	MetaDescription string `json:"meta_description"`
	OGImageURL      string `json:"og_image_url"`
}

type apiMediaResponse struct {
	URL string `json:"url"`
}

type apiEmailTemplateResponse struct {
	Key       string               `json:"key"`
	Label     string               `json:"label"`
	Subject   string               `json:"subject"`
	Variables json.RawMessage      `json:"variables"`
	Fields    []apiEmailFieldValue `json:"fields"`
}

type apiLocaleResponse struct {
	Locale    string `json:"locale"`
	Label     string `json:"label"`
	IsDefault bool   `json:"is_default"`
}

type apiEmailFieldValue struct {
	Key    string          `json:"key"`
	Locale string          `json:"locale"`
	Value  json.RawMessage `json:"value"`
}

// ---------------------------------------------------------------------------
// Request options
// ---------------------------------------------------------------------------

type requestOptions struct {
	locale  string
	width   int
	height  int
	quality int
	format  string
}

// RequestOption configures an API request.
type RequestOption func(*requestOptions)

// WithLocale overrides the default locale for a single request.
func WithLocale(locale string) RequestOption {
	return func(o *requestOptions) { o.locale = locale }
}

// MediaOption configures media URL parameters.
type MediaOption func(*requestOptions)

// Width sets the resize width for an image.
func Width(w int) MediaOption {
	return func(o *requestOptions) { o.width = w }
}

// Height sets the resize height for an image.
func Height(h int) MediaOption {
	return func(o *requestOptions) { o.height = h }
}

// Quality sets the quality (1-100) for an image.
func Quality(q int) MediaOption {
	return func(o *requestOptions) { o.quality = q }
}

// Format sets the output format (webp, avif, png, jpeg) for an image.
func Format(f string) MediaOption {
	return func(o *requestOptions) { o.format = f }
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client fetches content from the CMS public API.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient creates a Client for the given CMS configuration.
func NewClient(cfg Config) *Client {
	if cfg.Locale == "" {
		cfg.Locale = "en"
	}
	return &Client{
		config: cfg,
		http:   &http.Client{},
	}
}

// base returns the API base path: {APIURL}/api/v1/{SiteSlug}
func (c *Client) base() string {
	return fmt.Sprintf("%s/api/v1/%s",
		strings.TrimRight(c.config.APIURL, "/"),
		c.config.SiteSlug,
	)
}

// do performs an authenticated GET request and decodes the JSON response.
func (c *Client) do(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return fmt.Errorf("cms: request creation failed: %w", err)
	}
	req.Header.Set("X-API-Key", c.config.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cms: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("cms: API returned status %d for %s", resp.StatusCode, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("cms: decode failed: %w", err)
	}
	return nil
}

// ListPages returns all published pages for the site.
func (c *Client) ListPages(ctx context.Context) ([]apiPageListItem, error) {
	var items []apiPageListItem
	if err := c.do(ctx, "/pages", &items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListLocales returns all configured locales for the site.
func (c *Client) ListLocales(ctx context.Context) ([]SiteLocale, error) {
	var items []apiLocaleResponse
	if err := c.do(ctx, "/locales", &items); err != nil {
		return nil, err
	}
	locales := make([]SiteLocale, len(items))
	for i, item := range items {
		locales[i] = SiteLocale{
			Code:      item.Locale,
			Label:     item.Label,
			IsDefault: item.IsDefault,
		}
	}
	return locales, nil
}

// GetPage fetches a published page's content by path and returns a PageData.
func (c *Client) GetPage(ctx context.Context, pagePath string, opts ...RequestOption) (PageData, error) {
	o := &requestOptions{locale: c.config.Locale}
	for _, fn := range opts {
		fn(o)
	}

	// Normalize path: ensure it starts with / for the wildcard route.
	// Root path needs "/pages//" so axum's {*path} captures "/".
	normalized := pagePath
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	query := fmt.Sprintf("?locale=%s", o.locale)

	var reqPath string
	if normalized == "/" {
		reqPath = "/pages//" + query
	} else {
		reqPath = "/pages" + normalized + query
	}

	var resp apiPageResponse
	if err := c.do(ctx, reqPath, &resp); err != nil {
		return PageData{}, err
	}

	return c.resolvePageData(resp, o.locale), nil
}

// GetSEO fetches SEO data for a published page.
func (c *Client) GetSEO(ctx context.Context, pagePath string, opts ...RequestOption) (SEOData, error) {
	o := &requestOptions{locale: c.config.Locale}
	for _, fn := range opts {
		fn(o)
	}

	normalized := pagePath
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	query := fmt.Sprintf("?locale=%s", o.locale)

	var seoPath string
	if normalized == "/" {
		seoPath = "/seo//" + query
	} else {
		seoPath = "/seo" + normalized + query
	}

	var resp apiSEOResponse
	if err := c.do(ctx, seoPath, &resp); err != nil {
		return SEOData{}, err
	}

	return SEOData{
		MetaTitle:       resp.MetaTitle,
		MetaDescription: resp.MetaDescription,
		OGImageURL:      resp.OGImageURL,
	}, nil
}

// GetMediaURL fetches a signed media URL with optional image processing params.
func (c *Client) GetMediaURL(ctx context.Context, mediaID string, opts ...MediaOption) (string, error) {
	o := &requestOptions{}
	for _, fn := range opts {
		fn(o)
	}

	path := "/media/" + mediaID
	var params []string
	if o.width > 0 {
		params = append(params, fmt.Sprintf("w=%d", o.width))
	}
	if o.height > 0 {
		params = append(params, fmt.Sprintf("h=%d", o.height))
	}
	if o.quality > 0 {
		params = append(params, fmt.Sprintf("q=%d", o.quality))
	}
	if o.format != "" {
		params = append(params, "format="+o.format)
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var resp apiMediaResponse
	if err := c.do(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.URL, nil
}

// EmailTemplateData holds resolved email template content.
type EmailTemplateData struct {
	Key     string
	Label   string
	Subject string
	fields  map[string]any
}

// Text returns a field value as a string.
func (e EmailTemplateData) Text(key string) string {
	return fieldText(e.fields, key)
}

// GetEmailTemplate fetches a published email template's content.
func (c *Client) GetEmailTemplate(ctx context.Context, key string, opts ...RequestOption) (EmailTemplateData, error) {
	o := &requestOptions{locale: c.config.Locale}
	for _, fn := range opts {
		fn(o)
	}

	query := fmt.Sprintf("?locale=%s", o.locale)

	var resp apiEmailTemplateResponse
	if err := c.do(ctx, "/emails/"+key+query, &resp); err != nil {
		return EmailTemplateData{}, err
	}

	fields := make(map[string]any)
	for _, f := range resp.Fields {
		var val any
		_ = json.Unmarshal(f.Value, &val)
		fields[f.Key] = val
	}

	return EmailTemplateData{
		Key:     resp.Key,
		Label:   resp.Label,
		Subject: resp.Subject,
		fields:  fields,
	}, nil
}

// ---------------------------------------------------------------------------
// Resolvers
// ---------------------------------------------------------------------------

// resolvePageData converts an API response into a PageData with keyed fields.
func (c *Client) resolvePageData(resp apiPageResponse, locale string) PageData {
	fields := make(map[string]any)
	for _, f := range resp.Fields {
		var val any
		_ = json.Unmarshal(f.Value, &val)
		fields[f.Key] = val
	}

	subcollections := resolveSubcollections(resp.Subcollections)

	return NewPageData(resp.Path, resp.Slug, locale, fields, subcollections, nil)
}

func resolveSubcollections(scs []apiSubcollection) map[string][]EntryData {
	// Always return a non-nil map so SubcollectionOr can distinguish
	// "production data with zero entries" (non-nil map) from "no CMS data"
	// (nil map, used in template renders for schema discovery).
	result := make(map[string][]EntryData)
	for _, sc := range scs {
		var entries []EntryData
		for _, e := range sc.Entries {
			fields := make(map[string]any)
			for _, f := range e.Fields {
				var val any
				_ = json.Unmarshal(f.Value, &val)
				fields[f.Key] = val
			}
			entries = append(entries, EntryData{
				Fields:         fields,
				Subcollections: resolveSubcollections(e.Subcollections),
			})
		}
		result[sc.Key] = entries
	}
	return result
}
