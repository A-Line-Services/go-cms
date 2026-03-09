package cms

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// SiteLocale represents a configured locale for a CMS site.
type SiteLocale struct {
	// Code is the locale identifier (e.g. "en", "nl", "fr").
	Code string

	// Label is the human-readable name (e.g. "English", "Nederlands").
	Label string

	// IsDefault indicates whether this is the site's default locale.
	IsDefault bool
}

// SEOData holds SEO metadata for a page.
type SEOData struct {
	MetaTitle       string
	MetaDescription string
	OGImageURL      string
	Keywords        string
}

// SiteSEOConfig holds site-wide SEO and business information fetched from the CMS.
// Used to auto-generate JSON-LD structured data (LocalBusiness, Person, Service, WebSite).
type SiteSEOConfig struct {
	SiteName               string
	DefaultOGImageURL      string
	BusinessName           string
	BusinessType           string // Schema.org type, e.g. "LocalBusiness", "InteriorDesigner"
	StreetAddress          string
	AddressLocality        string
	AddressRegion          string
	PostalCode             string
	AddressCountry         string
	Phone                  string
	Email                  string
	GeoLat                 *float64
	GeoLng                 *float64
	PriceRange             string
	FoundingDate           string
	VatID                  string
	ServiceAreas           []string
	OpeningHours           []OpeningHoursSpec
	OwnerName              string
	OwnerJobTitle          string
	OwnerDescription       string
	OwnerImageURL          string
	OwnerSameAs            []string
	Services               []ServiceSpec
	SocialProfiles         []string
	DefaultMetaTitle       string
	DefaultMetaDescription string
	DefaultKeywords        string
}

// OpeningHoursSpec represents one opening hours entry.
type OpeningHoursSpec struct {
	Days   string `json:"days"`
	Opens  string `json:"opens"`
	Closes string `json:"closes"`
}

// ServiceSpec represents a service offered by the business.
type ServiceSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HasBusiness reports whether the config has enough data to generate a LocalBusiness schema.
func (c *SiteSEOConfig) HasBusiness() bool {
	return c != nil && c.BusinessName != ""
}

// HasOwner reports whether the config has enough data to generate a Person schema.
func (c *SiteSEOConfig) HasOwner() bool {
	return c != nil && c.OwnerName != ""
}

// HasServices reports whether the config has any services defined.
func (c *SiteSEOConfig) HasServices() bool {
	return c != nil && len(c.Services) > 0
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

// FileValue represents a CMS file field value (e.g. a downloadable PDF).
// The CMS stores file fields as { url, filename } objects.
type FileValue struct {
	URL      string
	Filename string
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

// URLAttrs returns conditional HTML attributes for a URL field value.
// It adds title when non-empty and target="_blank" + rel="noopener noreferrer"
// when the target is "_blank". Use with templ spread: { v.Attrs()... }
func (v URLValue) Attrs() templ.Attributes {
	attrs := templ.Attributes{}
	if v.Title != "" {
		attrs["title"] = v.Title
	}
	if v.Target == "_blank" {
		attrs["target"] = "_blank"
		attrs["rel"] = "noopener noreferrer"
	}
	return attrs
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
	localePrefix   string
	rtLinkClass    string
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

// File returns a field value as a FileValue (downloadable file).
func (e EntryData) File(key string) FileValue {
	return fieldFile(e.Fields, key)
}

// FileOr returns the CMS file value, or fallback if missing/empty URL.
func (e EntryData) FileOr(key string, fallback FileValue) FileValue {
	f := fieldFile(e.Fields, key)
	if f.URL == "" {
		return fallback
	}
	return f
}

// Number returns a field value as a float64.
func (e EntryData) Number(key string) float64 {
	return fieldNumber(e.Fields, key)
}

// Currency returns a field value as a CurrencyValue.
func (e EntryData) Currency(key string) CurrencyValue {
	return CurrencyValue{Amount: fieldNumber(e.Fields, key)}
}

// Toggle returns a boolean field value. Returns false if not found.
// Used with data-cms-toggle to conditionally show/hide elements.
func (e EntryData) Toggle(key string) bool {
	return fieldBool(e.Fields, key)
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

	// Locales lists all configured locales for the site.
	// Non-empty only when the site has multiple locales.
	Locales []SiteLocale

	fields         map[string]any
	subcollections map[string][]EntryData
	seo            *SEOData
	listings       map[string][]PageData
	imgProc        imageProcessor

	// contentPath is the CMS path without locale prefix (e.g. "/about").
	// Used by findComponent() to match against registered pages/collections.
	// Empty in single-locale mode (Path is used directly).
	contentPath string

	// localePrefix is the URL prefix for this page's locale.
	// "" for the default-locale root build, "/en" or "/nl" for prefixed builds.
	localePrefix string

	// defaultLocale is the site's default locale code.
	defaultLocale string

	// layoutManifest maps layout path prefixes to layout IDs.
	// Set by the build pipeline when layouts are registered.
	layoutManifest map[string]string

	// siteName is the site-level name used as a title suffix (e.g. "My Company").
	// When set, SEOHead renders: <title>{MetaTitle} | {SiteName}</title>
	siteName string

	// defaultOGImageURL is the site-level fallback OG image URL.
	// Used by SEOHead when the page has no page-level og:image.
	defaultOGImageURL string

	// siteURL is the public URL of the site (e.g. "https://example.com").
	// Used for canonical URLs and og:url. No trailing slash.
	siteURL string

	// seoConfig holds site-wide SEO and business info for JSON-LD schemas.
	seoConfig *SiteSEOConfig

	// rtLinkClass is the CSS class injected onto <a> tags in rich text HTML.
	rtLinkClass string
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

// LocalePrefix returns the URL prefix for this page's locale.
// Returns "" for the default-locale root build, "/en" or "/nl" for prefixed builds.
func (p PageData) LocalePrefix() string {
	return p.localePrefix
}

// ContentPath returns the CMS content path (without locale prefix).
// Falls back to Path in single-locale mode.
func (p PageData) ContentPath() string {
	if p.contentPath != "" {
		return p.contentPath
	}
	return p.Path
}

// IsDefaultLocale reports whether this page is rendered in the site's default locale.
func (p PageData) IsDefaultLocale() bool {
	return p.defaultLocale == "" || p.Locale == p.defaultLocale
}

// AlternatePath returns the URL path for this page in a different locale.
// For the site's default locale, returns the unprefixed root path (e.g. "/about").
// For other locales, returns the prefixed path (e.g. "/nl/about").
func (p PageData) AlternatePath(locale string) string {
	cp := p.ContentPath()
	if locale == p.defaultLocale {
		return cp
	}
	if cp == "/" {
		return "/" + locale
	}
	return "/" + locale + cp
}

// PrefixedAlternatePath returns the locale-prefixed URL path, even for the default locale.
// Useful for hreflang tags where every locale needs an explicit prefixed path.
func (p PageData) PrefixedAlternatePath(locale string) string {
	cp := p.ContentPath()
	if cp == "/" {
		return "/" + locale
	}
	return "/" + locale + cp
}

// LocaleHref prefixes an internal path with the current locale prefix.
// External URLs (starting with "http", "//", or "#") are returned unchanged.
func (p PageData) LocaleHref(path string) string {
	return prefixInternalHref(path, p.localePrefix)
}

// LayoutManifest returns the registered layout prefix→ID mapping.
// Returns nil when no layouts are registered.
func (p PageData) LayoutManifest() map[string]string {
	return p.layoutManifest
}

// HasLayouts reports whether layouts are configured for this build.
func (p PageData) HasLayouts() bool {
	return len(p.layoutManifest) > 0
}

// contentPathOrPath returns contentPath if set, otherwise Path.
// Used internally for layout chain matching.
func (p PageData) contentPathOrPath() string {
	if p.contentPath != "" {
		return p.contentPath
	}
	return p.Path
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
// When RichTextLinkClass is configured, injects the class onto <a> tags.
func (p PageData) RichTextOr(key string, fallback string) template.HTML {
	v := fieldText(p.fields, key)
	if v == "" {
		v = fallback
	}
	if p.rtLinkClass != "" {
		v = addRichTextLinkClass(v, p.rtLinkClass)
	}
	return template.HTML(v)
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
// Internal paths (starting with "/") are auto-prefixed with the locale prefix.
func (p PageData) URLOr(key, fallback string) string {
	href := fallback
	if v := fieldURL(p.fields, key); v.Href != "" {
		href = v.Href
	}
	return prefixInternalHref(href, p.localePrefix)
}

// URLValueOr returns the full CMS URL value (href, text, title, target),
// using the provided fallback href and text if the field is missing/empty.
// Internal href paths are auto-prefixed with the locale prefix.
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
	v.Href = prefixInternalHref(v.Href, p.localePrefix)
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

// Toggle returns a boolean field value. Returns false if not found.
// Used with data-cms-toggle to conditionally show/hide elements.
func (p PageData) Toggle(key string) bool {
	return fieldBool(p.fields, key)
}

// ToggleOr returns the CMS boolean value, or fallback if the key is missing.
// When fields is nil (template/sync renders), always returns true so that the
// element and its children are rendered for CMS schema discovery.
func (p PageData) ToggleOr(key string, fallback bool) bool {
	if p.fields == nil {
		return true // always render for schema discovery
	}
	if _, ok := p.fields[key]; !ok {
		return fallback
	}
	return fieldBool(p.fields, key)
}

// TextOr returns the CMS text value, or fallback if missing/empty.
func (e EntryData) TextOr(key, fallback string) string {
	if v := fieldText(e.Fields, key); v != "" {
		return v
	}
	return fallback
}

// RichTextOr returns the CMS rich text value, or fallback if missing/empty.
// When RichTextLinkClass is configured, injects the class onto <a> tags.
func (e EntryData) RichTextOr(key string, fallback string) template.HTML {
	v := fieldText(e.Fields, key)
	if v == "" {
		v = fallback
	}
	if e.rtLinkClass != "" {
		v = addRichTextLinkClass(v, e.rtLinkClass)
	}
	return template.HTML(v)
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
// Internal paths are auto-prefixed with the locale prefix.
func (e EntryData) URLOr(key, fallback string) string {
	href := fallback
	if v := fieldURL(e.Fields, key); v.Href != "" {
		href = v.Href
	}
	return prefixInternalHref(href, e.localePrefix)
}

// URLValueOr returns the full CMS URL value (href, text, title, target),
// using the provided fallback href and text if the field is missing/empty.
// Internal href paths are auto-prefixed with the locale prefix.
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
	v.Href = prefixInternalHref(v.Href, e.localePrefix)
	return v
}

// ToggleOr returns the CMS boolean value, or fallback if the key is missing.
// When Fields is nil (template/sync renders), always returns true so that the
// element and its children are rendered for CMS schema discovery.
func (e EntryData) ToggleOr(key string, fallback bool) bool {
	if e.Fields == nil {
		return true // always render for schema discovery
	}
	if _, ok := e.Fields[key]; !ok {
		return fallback
	}
	return fieldBool(e.Fields, key)
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

// File returns a field value as a FileValue (downloadable file).
func (p PageData) File(key string) FileValue {
	return fieldFile(p.fields, key)
}

// FileOr returns the CMS file value, or fallback if missing/empty URL.
func (p PageData) FileOr(key string, fallback FileValue) FileValue {
	f := fieldFile(p.fields, key)
	if f.URL == "" {
		return fallback
	}
	return f
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

// SiteName returns the site-level name (used as title suffix).
// Returns "" if not configured.
func (p PageData) SiteName() string {
	return p.siteName
}

// DefaultOGImageURL returns the site-level fallback OG image URL.
// Returns "" if not configured.
func (p PageData) DefaultOGImageURL() string {
	return p.defaultOGImageURL
}

// SiteURL returns the site's public URL (e.g. "https://example.com").
// No trailing slash. Returns "" if not known.
func (p PageData) SiteURL() string {
	return p.siteURL
}

// CanonicalURL returns the full canonical URL for this page.
// Returns "" if SiteURL is not set.
func (p PageData) CanonicalURL() string {
	if p.siteURL == "" {
		return ""
	}
	path := p.Path
	// Add trailing slash for non-root paths to match static file serving.
	if path != "/" && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return p.siteURL + path
}

// SEOConfig returns the site-wide SEO and business information.
// Returns nil if not fetched.
func (p PageData) SEOConfig() *SiteSEOConfig {
	return p.seoConfig
}

// EffectiveOGImageURL returns the page's OG image URL if set,
// otherwise falls back to the site-level default OG image URL.
func (p PageData) EffectiveOGImageURL() string {
	if p.SEO().OGImageURL != "" {
		return p.SEO().OGImageURL
	}
	return p.defaultOGImageURL
}

// EffectiveTitle returns the meta title with the site name suffix appended
// when both are set. Format: "Page Title | Site Name".
// Falls back through: page meta title → site-wide default meta title → site name.
func (p PageData) EffectiveTitle() string {
	title := p.SEO().MetaTitle
	if title == "" && p.seoConfig != nil {
		title = p.seoConfig.DefaultMetaTitle
	}
	if title != "" && p.siteName != "" {
		return title + " | " + p.siteName
	}
	if title != "" {
		return title
	}
	return p.siteName
}

// EffectiveDescription returns the page's meta description if set,
// otherwise falls back to the site-wide default meta description.
func (p PageData) EffectiveDescription() string {
	if p.SEO().MetaDescription != "" {
		return p.SEO().MetaDescription
	}
	if p.seoConfig != nil {
		return p.seoConfig.DefaultMetaDescription
	}
	return ""
}

// EffectiveKeywords returns the page's keywords if set,
// otherwise falls back to the site-wide default keywords.
func (p PageData) EffectiveKeywords() string {
	if p.SEO().Keywords != "" {
		return p.SEO().Keywords
	}
	if p.seoConfig != nil {
		return p.seoConfig.DefaultKeywords
	}
	return ""
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

// addRichTextLinkClass injects a CSS class onto <a> tags in rich text HTML.
// The CMS editor adds class="rte-link" to all links, so this replaces that
// with class="rte-link <linkClass>" to apply site-configured styling.
func addRichTextLinkClass(html, linkClass string) string {
	return strings.ReplaceAll(html, `class="rte-link"`, `class="rte-link `+linkClass+`"`)
}

// RichTextLinkClass returns the configured CSS class for rich text links.
// Returns "" if not configured. Useful for sites that need to know the
// class for custom styling.
func (p PageData) RichTextLinkClass() string {
	return p.rtLinkClass
}

// setEntryRichTextLinkClass recursively sets the rich text link class on all
// subcollection entries.
func setEntryRichTextLinkClass(subcollections map[string][]EntryData, class string) {
	for key, entries := range subcollections {
		for i := range entries {
			entries[i].rtLinkClass = class
			setEntryRichTextLinkClass(entries[i].Subcollections, class)
		}
		subcollections[key] = entries
	}
}

// setEntryLocalePrefix recursively sets the locale prefix on all
// subcollection entries so nested URL fields are auto-prefixed.
func setEntryLocalePrefix(subcollections map[string][]EntryData, prefix string) {
	for key, entries := range subcollections {
		for i := range entries {
			entries[i].localePrefix = prefix
			setEntryLocalePrefix(entries[i].Subcollections, prefix)
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

func fieldFile(fields map[string]any, key string) FileValue {
	if fields == nil {
		return FileValue{}
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return FileValue{}
	}
	switch val := v.(type) {
	case map[string]any:
		url, _ := val["url"].(string)
		filename, _ := val["filename"].(string)
		return FileValue{URL: url, Filename: filename}
	case string:
		// Plain string treated as URL
		return FileValue{URL: val}
	default:
		return FileValue{}
	}
}

func fieldBool(fields map[string]any, key string) bool {
	if fields == nil {
		return false
	}
	v, ok := fields[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val == "true" || val == "1"
	default:
		return false
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
// Locale-aware URL prefixing
// ---------------------------------------------------------------------------

// prefixInternalHref prepends a locale prefix to internal paths.
// External URLs (http, //, mailto:, #, empty) are returned unchanged.
// A path like "/about" with prefix "/en" becomes "/en/about".
func prefixInternalHref(href, prefix string) string {
	if prefix == "" || href == "" {
		return href
	}
	// Don't prefix external URLs, anchors, mailto, tel, etc.
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "//") || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return href
	}
	// Only prefix paths starting with /
	if !strings.HasPrefix(href, "/") {
		return href
	}
	if href == "/" {
		return prefix
	}
	return prefix + href
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

// PreloadSrcSet attempts to resolve format-specific variants and returns the
// srcset string if at least one variant was successfully resolved locally.
// Returns "" if the format is not supported by the backend (all downloads fail).
// Unlike HasFormat, this triggers downloads as a side effect, making it safe
// to call before HasFormat — use this in <link rel="preload"> where no prior
// SrcSetFor call has populated the resolved map yet.
func (i ImageValue) PreloadSrcSet(format string, widths ...int) string {
	srcset := i.SrcSetFor(format, widths...)
	if !i.HasFormat(format) {
		return ""
	}
	return srcset
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

// srcSetEntryOpts builds per-entry MediaOptions for a given width, scaling
// height proportionally from the base crop options.
func srcSetEntryOpts(w int, format string, base *requestOptions) []MediaOption {
	opts := []MediaOption{Width(w)}
	if format != "" {
		opts = append(opts, Format(format))
	}
	if base.height > 0 && base.width > 0 {
		opts = append(opts, Height(w*base.height/base.width))
	}
	opts = append(opts, Crop())
	if base.gravity != "" {
		opts = append(opts, Gravity(base.gravity))
	}
	if base.quality > 0 {
		opts = append(opts, Quality(base.quality))
	}
	return opts
}

// srcSetCropped generates a cropped srcset in a specific format (or "" for original).
func (i ImageValue) srcSetCropped(format string, widths []int, base *requestOptions) string {
	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		src := i.Src(srcSetEntryOpts(w, format, base)...)
		parts = append(parts, src+" "+strconv.Itoa(w)+"w")
	}
	return strings.Join(parts, ", ")
}

// SrcSetWith generates a responsive srcset with additional options applied to
// each width entry. When crop options (Height, Crop, Gravity) are present, the
// height is scaled proportionally at each width to maintain the aspect ratio.
// Falls back to standard SrcSet when no crop is requested.
func (i ImageValue) SrcSetWith(widths []int, opts ...MediaOption) string {
	if i.URL == "" || len(widths) == 0 {
		return ""
	}
	base := &requestOptions{}
	for _, fn := range opts {
		fn(base)
	}
	if !base.crop {
		return i.SrcSet(widths...)
	}
	return i.srcSetCropped("", widths, base)
}

// SrcSetForWith generates a format-specific responsive srcset with crop options.
// Like SrcSetWith but forces a specific output format (e.g. "avif", "webp").
// Falls back to standard SrcSetFor when no crop is requested.
func (i ImageValue) SrcSetForWith(format string, widths []int, opts ...MediaOption) string {
	if i.URL == "" || format == "" || len(widths) == 0 {
		return ""
	}
	base := &requestOptions{}
	for _, fn := range opts {
		fn(base)
	}
	if !base.crop {
		return i.SrcSetFor(format, widths...)
	}
	return i.srcSetCropped(format, widths, base)
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
	if o.crop {
		params = append(params, "crop=true")
	}
	if o.gravity != "" {
		params = append(params, "gravity="+o.gravity)
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
