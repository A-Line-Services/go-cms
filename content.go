package cms

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// SEOData holds SEO metadata for a page.
type SEOData struct {
	MetaTitle       string
	MetaDescription string
	OGImageURL      string
}

// ImageValue represents a CMS image field value.
type ImageValue struct {
	URL string
	Alt string

	// resolved maps remote URLs (including query params) to local paths.
	// Set during static build so Src/SrcSet/LQIP return local file paths
	// instead of CMS media URLs. Nil in non-build mode (no-op).
	resolved map[string]string

	// dl is the media downloader for lazy resolution of URLs not in the
	// pre-downloaded set (e.g. custom widths from ImageSized). Nil in
	// non-build mode.
	dl *mediaDownloader
}

// URLValue represents a CMS URL field value.
// The CMS stores URL fields as { href, text, title, target } objects,
// or as plain strings (legacy/sync default).
type URLValue struct {
	Href   string
	Text   string
	Title  string
	Target string // "_self" or "_blank"
}

// CurrencyValue represents a CMS currency field value.
type CurrencyValue struct {
	Amount float64
}

// imageProcessor transforms an ImageValue during build (e.g. downloading
// images and populating the resolved local paths).
type imageProcessor func(ImageValue) ImageValue

// EntryData represents a single subcollection entry.
type EntryData struct {
	Fields         map[string]any
	Subcollections map[string][]EntryData
	imgProc        imageProcessor
}

// Text returns a field value as a string.
func (e EntryData) Text(key string) string {
	return fieldText(e.Fields, key)
}

// RichText returns a field value as safe HTML.
func (e EntryData) RichText(key string) template.HTML {
	return fieldRichText(e.Fields, key)
}

// Image returns a field value as an ImageValue.
func (e EntryData) Image(key string) ImageValue {
	img := fieldImage(e.Fields, key)
	if e.imgProc != nil {
		img = e.imgProc(img)
	}
	return img
}

// Video returns a field value as a URL string.
func (e EntryData) Video(key string) string {
	return fieldText(e.Fields, key)
}

// URL returns a field value as a URL string.
func (e EntryData) URL(key string) string {
	return fieldText(e.Fields, key)
}

// Number returns a field value as a float64.
func (e EntryData) Number(key string) float64 {
	return fieldNumber(e.Fields, key)
}

// Currency returns a field value as a CurrencyValue.
func (e EntryData) Currency(key string) CurrencyValue {
	return CurrencyValue{Amount: fieldNumber(e.Fields, key)}
}

// Subcollection returns nested entries for the given subcollection key.
func (e EntryData) Subcollection(key string) []EntryData {
	if e.Subcollections == nil {
		return nil
	}
	return e.Subcollections[key]
}

// SubcollectionOr returns nested entries, or a single empty entry when there
// is no CMS data (nil Subcollections map). See PageData.SubcollectionOr.
func (e EntryData) SubcollectionOr(key string) []EntryData {
	if e.Subcollections == nil {
		return []EntryData{emptyEntry()}
	}
	return e.Subcollections[key]
}

// PageData holds resolved CMS content for a single page.
type PageData struct {
	Path   string
	Slug   string
	Locale string

	fields         map[string]any
	subcollections map[string][]EntryData
	seo            *SEOData
	listings       map[string][]PageData
	imgProc        imageProcessor
}

// NewPageData creates a PageData with the given content.
func NewPageData(
	path, slug, locale string,
	fields map[string]any,
	subcollections map[string][]EntryData,
	seo *SEOData,
) PageData {
	return PageData{
		Path:           path,
		Slug:           slug,
		Locale:         locale,
		fields:         fields,
		subcollections: subcollections,
		seo:            seo,
	}
}

// Text returns a field value as a string. Returns "" if not found.
func (p PageData) Text(key string) string {
	return fieldText(p.fields, key)
}

// TextOr returns the CMS text value, or fallback if missing/empty.
func (p PageData) TextOr(key, fallback string) string {
	if v := fieldText(p.fields, key); v != "" {
		return v
	}
	return fallback
}

// RichTextOr returns the CMS rich text value, or fallback if missing/empty.
func (p PageData) RichTextOr(key string, fallback string) template.HTML {
	if v := fieldText(p.fields, key); v != "" {
		return template.HTML(v)
	}
	return template.HTML(fallback)
}

// ImageOr returns the CMS image value, or fallback if missing/empty URL.
func (p PageData) ImageOr(key string, fallback ImageValue) ImageValue {
	img := fieldImage(p.fields, key)
	if img.URL == "" {
		return fallback
	}
	if p.imgProc != nil {
		img = p.imgProc(img)
	}
	return img
}

// URLOr returns the CMS URL href, or fallback if missing/empty.
func (p PageData) URLOr(key, fallback string) string {
	if v := fieldURL(p.fields, key); v.Href != "" {
		return v.Href
	}
	return fallback
}

// URLValueOr returns the full CMS URL value (href, text, title, target),
// using the provided fallback href and text if the field is missing/empty.
func (p PageData) URLValueOr(key, fallbackHref, fallbackText string) URLValue {
	v := fieldURL(p.fields, key)
	if v.Href == "" {
		v.Href = fallbackHref
	}
	if v.Text == "" {
		v.Text = fallbackText
	}
	if v.Target == "" {
		v.Target = "_self"
	}
	return v
}

// NumberOr returns the CMS number value, or fallback if the key is missing.
func (p PageData) NumberOr(key string, fallback float64) float64 {
	if p.fields == nil {
		return fallback
	}
	if _, ok := p.fields[key]; !ok {
		return fallback
	}
	return fieldNumber(p.fields, key)
}

// TextOr returns the CMS text value, or fallback if missing/empty.
func (e EntryData) TextOr(key, fallback string) string {
	if v := fieldText(e.Fields, key); v != "" {
		return v
	}
	return fallback
}

// RichTextOr returns the CMS rich text value, or fallback if missing/empty.
func (e EntryData) RichTextOr(key string, fallback string) template.HTML {
	if v := fieldText(e.Fields, key); v != "" {
		return template.HTML(v)
	}
	return template.HTML(fallback)
}

// ImageOr returns the CMS image value, or fallback if missing/empty URL.
func (e EntryData) ImageOr(key string, fallback ImageValue) ImageValue {
	img := fieldImage(e.Fields, key)
	if img.URL == "" {
		return fallback
	}
	if e.imgProc != nil {
		img = e.imgProc(img)
	}
	return img
}

// URLOr returns the CMS URL href, or fallback if missing/empty.
func (e EntryData) URLOr(key, fallback string) string {
	if v := fieldURL(e.Fields, key); v.Href != "" {
		return v.Href
	}
	return fallback
}

// URLValueOr returns the full CMS URL value (href, text, title, target),
// using the provided fallback href and text if the field is missing/empty.
func (e EntryData) URLValueOr(key, fallbackHref, fallbackText string) URLValue {
	v := fieldURL(e.Fields, key)
	if v.Href == "" {
		v.Href = fallbackHref
	}
	if v.Text == "" {
		v.Text = fallbackText
	}
	if v.Target == "" {
		v.Target = "_self"
	}
	return v
}

// NumberOr returns the CMS number value, or fallback if the key is missing.
func (e EntryData) NumberOr(key string, fallback float64) float64 {
	if e.Fields == nil {
		return fallback
	}
	if _, ok := e.Fields[key]; !ok {
		return fallback
	}
	return fieldNumber(e.Fields, key)
}

// RichText returns a field value as safe HTML. Returns "" if not found.
func (p PageData) RichText(key string) template.HTML {
	return fieldRichText(p.fields, key)
}

// Image returns a field value as an ImageValue. Returns zero value if not found.
func (p PageData) Image(key string) ImageValue {
	img := fieldImage(p.fields, key)
	if p.imgProc != nil {
		img = p.imgProc(img)
	}
	return img
}

// Video returns a field value as a URL string.
func (p PageData) Video(key string) string {
	return fieldText(p.fields, key)
}

// URL returns a field value as a URL string.
func (p PageData) URL(key string) string {
	return fieldText(p.fields, key)
}

// Number returns a field value as a float64. Returns 0 if not found.
func (p PageData) Number(key string) float64 {
	return fieldNumber(p.fields, key)
}

// Currency returns a field value as a CurrencyValue.
func (p PageData) Currency(key string) CurrencyValue {
	return CurrencyValue{Amount: fieldNumber(p.fields, key)}
}

// Subcollection returns entries for the given subcollection key.
// Returns nil if no entries exist.
func (p PageData) Subcollection(key string) []EntryData {
	if p.subcollections == nil {
		return nil
	}
	return p.subcollections[key]
}

// SubcollectionOr returns entries for the given subcollection key.
//
// When there is no CMS data at all (subcollections map is nil — template
// renders and sync payloads), returns a single empty entry so template
// components always render at least one [data-cms-entry] element. This
// ensures schema discovery and gives the editor bridge a template to clone.
//
// When CMS data exists (subcollections map is non-nil — production builds),
// returns the actual entries, which may be empty. This keeps production
// HTML clean: no phantom entries with blank fields.
func (p PageData) SubcollectionOr(key string) []EntryData {
	if p.subcollections == nil {
		return []EntryData{emptyEntry()}
	}
	return p.subcollections[key]
}

// Listing returns collection entries attached to this page during build.
// For example, a blog index page can call p.Listing("blog") to get all
// published blog entries as PageData values with their own fields/SEO.
// Returns nil if no listing exists for the given collection key.
func (p PageData) Listing(key string) []PageData {
	if p.listings == nil {
		return nil
	}
	return p.listings[key]
}

// SEO returns the page's SEO data. Returns zero value if none.
func (p PageData) SEO() SEOData {
	if p.seo == nil {
		return SEOData{}
	}
	return *p.seo
}

// setEntryImageProcessor recursively sets the image processor on all
// subcollection entries so nested images are also downloaded.
func setEntryImageProcessor(subcollections map[string][]EntryData, proc imageProcessor) {
	for key, entries := range subcollections {
		for i := range entries {
			entries[i].imgProc = proc
			setEntryImageProcessor(entries[i].Subcollections, proc)
		}
		subcollections[key] = entries
	}
}

// emptyEntry returns an EntryData with an empty Fields map and nil
// Subcollections. The nil Subcollections ensures that nested
// SubcollectionOr calls also return fallback entries (for schema discovery
// in template renders). Production entries from the CMS always have
// non-nil Subcollections (even if empty), so they skip the fallback.
func emptyEntry() EntryData {
	return EntryData{
		Fields:         map[string]any{},
		Subcollections: nil,
	}
}

// ---------------------------------------------------------------------------
// Shared field accessors
// ---------------------------------------------------------------------------

func fieldText(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Format without trailing zeros for whole numbers
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func fieldRichText(fields map[string]any, key string) template.HTML {
	return template.HTML(fieldText(fields, key))
}

func fieldURL(fields map[string]any, key string) URLValue {
	if fields == nil {
		return URLValue{}
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return URLValue{}
	}
	switch val := v.(type) {
	case map[string]any:
		href, _ := val["href"].(string)
		text, _ := val["text"].(string)
		title, _ := val["title"].(string)
		target, _ := val["target"].(string)
		if target == "" {
			target = "_self"
		}
		return URLValue{Href: href, Text: text, Title: title, Target: target}
	case string:
		// Legacy/sync format: plain string is the href.
		return URLValue{Href: val}
	default:
		return URLValue{}
	}
}

func fieldImage(fields map[string]any, key string) ImageValue {
	if fields == nil {
		return ImageValue{}
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return ImageValue{}
	}
	switch val := v.(type) {
	case map[string]any:
		url, _ := val["url"].(string)
		alt, _ := val["alt"].(string)
		return ImageValue{URL: url, Alt: alt}
	case string:
		// Plain string treated as URL
		return ImageValue{URL: val}
	default:
		return ImageValue{}
	}
}

func fieldNumber(fields map[string]any, key string) float64 {
	if fields == nil {
		return 0
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// ImageValue methods — URL construction and static build resolution
// ---------------------------------------------------------------------------

// Src returns the image URL with optional processing parameters appended
// as query params. If no options are given, returns the raw URL.
// During static builds, returns the pre-downloaded local path if available.
// If the URL wasn't pre-downloaded but a downloader is attached (e.g. custom
// widths from ImageSized), it lazily downloads the variant.
func (i ImageValue) Src(opts ...MediaOption) string {
	if i.URL == "" {
		return ""
	}
	url := buildMediaURL(i.URL, opts...)
	if i.resolved != nil {
		if local, ok := i.resolved[url]; ok {
			return local
		}
		// Lazy download: variant wasn't pre-downloaded (custom width).
		if i.dl != nil {
			if local, err := i.dl.download(url); err == nil {
				i.resolved[url] = local
				return local
			}
		}
	}
	return url
}

// SrcSet generates a responsive srcset string for the given widths.
// Example: "url?w=400 400w, url?w=800 800w, url?w=1200 1200w"
// During static builds, returns local file paths if available.
func (i ImageValue) SrcSet(widths ...int) string {
	if i.URL == "" || len(widths) == 0 {
		return ""
	}

	sep := "?"
	if strings.Contains(i.URL, "?") {
		sep = "&"
	}

	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		remote := i.URL + sep + "w=" + strconv.Itoa(w)
		local := remote
		if i.resolved != nil {
			if l, ok := i.resolved[remote]; ok {
				local = l
			} else if i.dl != nil {
				if l, err := i.dl.download(remote); err == nil {
					i.resolved[remote] = l
					local = l
				}
			}
		}
		parts = append(parts, local+" "+strconv.Itoa(w)+"w")
	}
	return strings.Join(parts, ", ")
}

// SrcSetFor generates a responsive srcset string for a specific output format.
// Format values: "webp", "avif", etc. Returns "" if URL, format, or widths are empty.
// During static builds, resolves to local file paths when available.
func (i ImageValue) SrcSetFor(format string, widths ...int) string {
	if i.URL == "" || format == "" || len(widths) == 0 {
		return ""
	}

	sep := "?"
	if strings.Contains(i.URL, "?") {
		sep = "&"
	}

	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		remote := i.URL + sep + "w=" + strconv.Itoa(w) + "&format=" + format
		local := remote
		if i.resolved != nil {
			if l, ok := i.resolved[remote]; ok {
				local = l
			} else if i.dl != nil {
				if l, err := i.dl.download(remote); err == nil {
					i.resolved[remote] = l
					local = l
				}
			}
		}
		parts = append(parts, local+" "+strconv.Itoa(w)+"w")
	}
	return strings.Join(parts, ", ")
}

// HasFormat reports whether at least one format-specific variant exists
// in the resolved map. Returns false when resolved is nil (non-build mode).
// Useful for conditionally rendering <source> elements in <picture>.
func (i ImageValue) HasFormat(format string) bool {
	if i.resolved == nil || i.URL == "" || format == "" {
		return false
	}
	suffix := "&format=" + format
	for k := range i.resolved {
		if strings.Contains(k, suffix) {
			return true
		}
	}
	return false
}

// LQIP returns a Low Quality Image Placeholder URL (tiny 32px wide, quality 20).
// During static builds, returns a base64 data URI if pre-resolved.
func (i ImageValue) LQIP() string {
	if i.URL == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(i.URL, "?") {
		sep = "&"
	}
	remote := i.URL + sep + "w=32&q=20"
	if i.resolved != nil {
		if local, ok := i.resolved[remote]; ok {
			return local
		}
	}
	return remote
}

// buildMediaURL constructs a URL with processing params.
func buildMediaURL(baseURL string, opts ...MediaOption) string {
	if len(opts) == 0 {
		return baseURL
	}

	o := &requestOptions{}
	for _, fn := range opts {
		fn(o)
	}

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
	if len(params) == 0 {
		return baseURL
	}

	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + strings.Join(params, "&")
}
