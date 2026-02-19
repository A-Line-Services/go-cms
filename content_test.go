package cms

import (
	"html/template"
	"strings"
	"testing"
)

func TestPageData_Text_ReturnsValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"title": "Hello World",
	}, nil, nil)

	got := p.Text("title")
	if got != "Hello World" {
		t.Errorf("Text('title') = %q, want %q", got, "Hello World")
	}
}

func TestPageData_Text_MissingKey_ReturnsEmpty(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.Text("title")
	if got != "" {
		t.Errorf("Text('title') = %q, want empty string", got)
	}
}

func TestPageData_Text_NonStringValue_ReturnsStringified(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"count": float64(42),
	}, nil, nil)

	got := p.Text("count")
	if got != "42" {
		t.Errorf("Text('count') = %q, want %q", got, "42")
	}
}

func TestPageData_RichText_ReturnsHTML(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"body": "<p>Hello <strong>World</strong></p>",
	}, nil, nil)

	got := p.RichText("body")
	want := template.HTML("<p>Hello <strong>World</strong></p>")
	if got != want {
		t.Errorf("RichText('body') = %q, want %q", got, want)
	}
}

func TestPageData_RichText_MissingKey_ReturnsEmpty(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.RichText("body")
	if got != "" {
		t.Errorf("RichText('body') = %q, want empty", got)
	}
}

func TestPageData_Image_ReturnsValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"hero": map[string]any{
			"url": "https://cdn.example.com/hero.jpg",
			"alt": "A hero image",
		},
	}, nil, nil)

	img := p.Image("hero")
	if img.URL != "https://cdn.example.com/hero.jpg" {
		t.Errorf("Image('hero').URL = %q, want hero.jpg URL", img.URL)
	}
	if img.Alt != "A hero image" {
		t.Errorf("Image('hero').Alt = %q, want %q", img.Alt, "A hero image")
	}
}

func TestPageData_Image_MissingKey_ReturnsZero(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	img := p.Image("hero")
	if img.URL != "" || img.Alt != "" {
		t.Errorf("Image('hero') = %+v, want zero value", img)
	}
}

func TestPageData_Image_StringValue_TreatedAsURL(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"logo": "https://cdn.example.com/logo.png",
	}, nil, nil)

	img := p.Image("logo")
	if img.URL != "https://cdn.example.com/logo.png" {
		t.Errorf("Image('logo').URL = %q, want the string value", img.URL)
	}
}

func TestPageData_Video_ReturnsURL(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"intro": "https://cdn.example.com/intro.mp4",
	}, nil, nil)

	got := p.Video("intro")
	if got != "https://cdn.example.com/intro.mp4" {
		t.Errorf("Video('intro') = %q", got)
	}
}

func TestPageData_URL_ReturnsValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"cta_link": "https://example.com/signup",
	}, nil, nil)

	got := p.URL("cta_link")
	if got != "https://example.com/signup" {
		t.Errorf("URL('cta_link') = %q", got)
	}
}

func TestPageData_Number_ReturnsFloat(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"price": float64(29.99),
	}, nil, nil)

	got := p.Number("price")
	if got != 29.99 {
		t.Errorf("Number('price') = %f, want 29.99", got)
	}
}

func TestPageData_Number_MissingKey_ReturnsZero(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.Number("price")
	if got != 0 {
		t.Errorf("Number('price') = %f, want 0", got)
	}
}

func TestPageData_Number_StringValue_ParsesFloat(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"rating": "4.5",
	}, nil, nil)

	got := p.Number("rating")
	if got != 4.5 {
		t.Errorf("Number('rating') = %f, want 4.5", got)
	}
}

func TestPageData_Currency_ReturnsValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"price": float64(99.95),
	}, nil, nil)

	got := p.Currency("price")
	if got.Amount != 99.95 {
		t.Errorf("Currency('price').Amount = %f, want 99.95", got.Amount)
	}
}

func TestPageData_Subcollection_ReturnsEntries(t *testing.T) {
	entries := []EntryData{
		{Fields: map[string]any{"name": "Alice"}},
		{Fields: map[string]any{"name": "Bob"}},
	}
	p := NewPageData("/", "home", "en", nil, map[string][]EntryData{
		"team": entries,
	}, nil)

	got := p.Subcollection("team")
	if len(got) != 2 {
		t.Fatalf("Subcollection('team') has %d entries, want 2", len(got))
	}
	if got[0].Text("name") != "Alice" {
		t.Errorf("entry[0].Text('name') = %q, want Alice", got[0].Text("name"))
	}
	if got[1].Text("name") != "Bob" {
		t.Errorf("entry[1].Text('name') = %q, want Bob", got[1].Text("name"))
	}
}

func TestPageData_Subcollection_MissingKey_ReturnsNil(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.Subcollection("team")
	if got != nil {
		t.Errorf("Subcollection('team') = %v, want nil", got)
	}
}

func TestPageData_SEO_ReturnsData(t *testing.T) {
	seo := &SEOData{
		MetaTitle:       "My Page Title",
		MetaDescription: "A description",
		OGImageURL:      "https://example.com/og.jpg",
	}
	p := NewPageData("/", "home", "en", nil, nil, seo)

	got := p.SEO()
	if got.MetaTitle != "My Page Title" {
		t.Errorf("SEO().MetaTitle = %q", got.MetaTitle)
	}
	if got.MetaDescription != "A description" {
		t.Errorf("SEO().MetaDescription = %q", got.MetaDescription)
	}
	if got.OGImageURL != "https://example.com/og.jpg" {
		t.Errorf("SEO().OGImageURL = %q", got.OGImageURL)
	}
}

func TestPageData_SEO_Nil_ReturnsZero(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.SEO()
	if got.MetaTitle != "" || got.MetaDescription != "" || got.OGImageURL != "" {
		t.Errorf("SEO() = %+v, want zero", got)
	}
}

func TestPageData_Path_And_Locale(t *testing.T) {
	p := NewPageData("/about", "about", "fr", nil, nil, nil)

	if p.Path != "/about" {
		t.Errorf("Path = %q, want /about", p.Path)
	}
	if p.Slug != "about" {
		t.Errorf("Slug = %q, want about", p.Slug)
	}
	if p.Locale != "fr" {
		t.Errorf("Locale = %q, want fr", p.Locale)
	}
}

func TestEntryData_Text(t *testing.T) {
	e := EntryData{Fields: map[string]any{"title": "Entry Title"}}
	if e.Text("title") != "Entry Title" {
		t.Errorf("EntryData.Text('title') = %q", e.Text("title"))
	}
}

func TestEntryData_Image(t *testing.T) {
	e := EntryData{Fields: map[string]any{
		"photo": map[string]any{"url": "http://img.test/1.jpg", "alt": "Photo"},
	}}
	img := e.Image("photo")
	if img.URL != "http://img.test/1.jpg" {
		t.Errorf("EntryData.Image('photo').URL = %q", img.URL)
	}
}

func TestEntryData_Subcollection(t *testing.T) {
	inner := []EntryData{
		{Fields: map[string]any{"label": "nested"}},
	}
	e := EntryData{
		Fields:         map[string]any{},
		Subcollections: map[string][]EntryData{"items": inner},
	}
	got := e.Subcollection("items")
	if len(got) != 1 || got[0].Text("label") != "nested" {
		t.Errorf("EntryData.Subcollection('items') unexpected: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// *Or fallback methods
// ---------------------------------------------------------------------------

func TestPageData_TextOr_ReturnsCMSValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{"title": "From CMS"}, nil, nil)
	got := p.TextOr("title", "Default Title")
	if got != "From CMS" {
		t.Errorf("TextOr = %q, want 'From CMS'", got)
	}
}

func TestPageData_TextOr_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.TextOr("title", "Default Title")
	if got != "Default Title" {
		t.Errorf("TextOr = %q, want 'Default Title'", got)
	}
}

func TestPageData_TextOr_EmptyStringCMSValue_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{"title": ""}, nil, nil)
	got := p.TextOr("title", "Default Title")
	if got != "Default Title" {
		t.Errorf("TextOr = %q, want 'Default Title' for empty CMS value", got)
	}
}

func TestPageData_RichTextOr_ReturnsCMSValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{"body": "<p>CMS content</p>"}, nil, nil)
	got := p.RichTextOr("body", "<p>Default</p>")
	if got != "<p>CMS content</p>" {
		t.Errorf("RichTextOr = %q", got)
	}
}

func TestPageData_RichTextOr_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.RichTextOr("body", "<p>Default</p>")
	if got != "<p>Default</p>" {
		t.Errorf("RichTextOr = %q, want default", got)
	}
}

func TestPageData_ImageOr_ReturnsCMSValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"hero": map[string]any{"url": "https://cms.test/hero.jpg", "alt": "CMS hero"},
	}, nil, nil)
	got := p.ImageOr("hero", ImageValue{URL: "/default.jpg", Alt: "default"})
	if got.URL != "https://cms.test/hero.jpg" {
		t.Errorf("ImageOr.URL = %q", got.URL)
	}
}

func TestPageData_ImageOr_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.ImageOr("hero", ImageValue{URL: "/default.jpg", Alt: "default"})
	if got.URL != "/default.jpg" || got.Alt != "default" {
		t.Errorf("ImageOr = %+v, want default", got)
	}
}

func TestPageData_URLOr_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.URLOr("link", "https://example.com")
	if got != "https://example.com" {
		t.Errorf("URLOr = %q", got)
	}
}

func TestPageData_URLOr_ObjectFormat(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"cta": map[string]any{
			"href": "https://app.example.com",
			"text": "Get started",
		},
	}, nil, nil)
	got := p.URLOr("cta", "https://fallback.com")
	if got != "https://app.example.com" {
		t.Errorf("URLOr = %q, want https://app.example.com", got)
	}
}

func TestPageData_URLValueOr_Fallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.URLValueOr("cta", "https://fallback.com", "Click me")
	if got.Href != "https://fallback.com" {
		t.Errorf("URLValueOr.Href = %q", got.Href)
	}
	if got.Text != "Click me" {
		t.Errorf("URLValueOr.Text = %q", got.Text)
	}
}

func TestPageData_URLValueOr_ObjectFormat(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"cta": map[string]any{
			"href":   "https://app.example.com",
			"text":   "Get started",
			"title":  "Sign up",
			"target": "_blank",
		},
	}, nil, nil)
	got := p.URLValueOr("cta", "https://fallback.com", "Click me")
	if got.Href != "https://app.example.com" {
		t.Errorf("Href = %q", got.Href)
	}
	if got.Text != "Get started" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Title != "Sign up" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Target != "_blank" {
		t.Errorf("Target = %q", got.Target)
	}
}

func TestPageData_URLValueOr_LegacyString(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{
		"link": "https://legacy.example.com",
	}, nil, nil)
	got := p.URLValueOr("link", "https://fallback.com", "Fallback text")
	if got.Href != "https://legacy.example.com" {
		t.Errorf("Href = %q", got.Href)
	}
	if got.Text != "Fallback text" {
		t.Errorf("Text = %q, want fallback since legacy string has no text", got.Text)
	}
}

func TestEntryData_URLValueOr(t *testing.T) {
	e := EntryData{Fields: map[string]any{
		"cta": map[string]any{
			"href": "https://signup.example.com",
			"text": "Sign up now",
		},
	}}
	got := e.URLValueOr("cta", "https://fallback.com", "Click")
	if got.Href != "https://signup.example.com" {
		t.Errorf("Href = %q", got.Href)
	}
	if got.Text != "Sign up now" {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestPageData_NumberOr_ReturnsFallback(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)
	got := p.NumberOr("price", 9.99)
	if got != 9.99 {
		t.Errorf("NumberOr = %f", got)
	}
}

func TestPageData_NumberOr_ReturnsCMSValue(t *testing.T) {
	p := NewPageData("/", "home", "en", map[string]any{"price": float64(29.99)}, nil, nil)
	got := p.NumberOr("price", 9.99)
	if got != 29.99 {
		t.Errorf("NumberOr = %f, want 29.99", got)
	}
}

func TestEntryData_TextOr(t *testing.T) {
	e := EntryData{Fields: nil}
	got := e.TextOr("name", "Unknown")
	if got != "Unknown" {
		t.Errorf("EntryData.TextOr = %q", got)
	}
}

func TestEntryData_ImageOr(t *testing.T) {
	e := EntryData{Fields: nil}
	got := e.ImageOr("photo", ImageValue{URL: "/placeholder.jpg", Alt: "placeholder"})
	if got.URL != "/placeholder.jpg" {
		t.Errorf("EntryData.ImageOr.URL = %q", got.URL)
	}
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func TestPageData_Listing_ReturnsEntries(t *testing.T) {
	p := NewPageData("/blog", "blog", "en", nil, nil, nil)
	p.listings = map[string][]PageData{
		"blog": {
			NewPageData("/blog/first", "first", "en", map[string]any{"title": "First"}, nil, nil),
			NewPageData("/blog/second", "second", "en", map[string]any{"title": "Second"}, nil, nil),
		},
	}

	entries := p.Listing("blog")
	if len(entries) != 2 {
		t.Fatalf("Listing('blog') len = %d, want 2", len(entries))
	}
	if entries[0].Text("title") != "First" {
		t.Errorf("entries[0].Text('title') = %q", entries[0].Text("title"))
	}
	if entries[1].Path != "/blog/second" {
		t.Errorf("entries[1].Path = %q", entries[1].Path)
	}
}

func TestPageData_Listing_MissingKey_ReturnsNil(t *testing.T) {
	p := NewPageData("/blog", "blog", "en", nil, nil, nil)
	p.listings = map[string][]PageData{
		"blog": {NewPageData("/blog/a", "a", "en", nil, nil, nil)},
	}

	entries := p.Listing("events")
	if entries != nil {
		t.Errorf("Listing('events') = %v, want nil", entries)
	}
}

func TestPageData_Listing_NilListings_ReturnsNil(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	entries := p.Listing("blog")
	if entries != nil {
		t.Errorf("Listing('blog') = %v, want nil", entries)
	}
}

func TestPageData_Listing_EntriesHaveFieldAccess(t *testing.T) {
	entry := NewPageData("/blog/post", "post", "en", map[string]any{
		"title": "My Post",
		"hero":  map[string]any{"url": "https://cdn.test/hero.jpg", "alt": "Hero"},
	}, nil, &SEOData{MetaTitle: "My Post - Blog"})

	p := NewPageData("/blog", "blog", "en", nil, nil, nil)
	p.listings = map[string][]PageData{"blog": {entry}}

	posts := p.Listing("blog")
	if posts[0].Text("title") != "My Post" {
		t.Errorf("Text = %q", posts[0].Text("title"))
	}
	if posts[0].Image("hero").URL != "https://cdn.test/hero.jpg" {
		t.Errorf("Image.URL = %q", posts[0].Image("hero").URL)
	}
	if posts[0].SEO().MetaTitle != "My Post - Blog" {
		t.Errorf("SEO = %q", posts[0].SEO().MetaTitle)
	}
}

// ---------------------------------------------------------------------------
// SubcollectionOr
// ---------------------------------------------------------------------------

func TestPageData_SubcollectionOr_ReturnsEntries_WhenPresent(t *testing.T) {
	entries := []EntryData{
		{Fields: map[string]any{"name": "Alice"}},
		{Fields: map[string]any{"name": "Bob"}},
	}
	p := NewPageData("/", "home", "en", nil, map[string][]EntryData{
		"team": entries,
	}, nil)

	got := p.SubcollectionOr("team")
	if len(got) != 2 {
		t.Fatalf("SubcollectionOr('team') len = %d, want 2", len(got))
	}
	if got[0].Text("name") != "Alice" {
		t.Errorf("got[0].Text('name') = %q, want Alice", got[0].Text("name"))
	}
}

func TestPageData_SubcollectionOr_ReturnsOneEmpty_WhenMissing(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.SubcollectionOr("team")
	if len(got) != 1 {
		t.Fatalf("SubcollectionOr('team') len = %d, want 1", len(got))
	}
	// The fallback entry should have empty fields — components show defaults.
	if got[0].Text("name") != "" {
		t.Errorf("fallback entry Text('name') = %q, want empty", got[0].Text("name"))
	}
}

func TestPageData_SubcollectionOr_ReturnsEmpty_WhenProductionHasZeroEntries(t *testing.T) {
	// Non-nil subcollections map (= production/CMS data exists) with
	// zero entries should return empty — no phantom fallback entry.
	p := NewPageData("/", "home", "en", nil, map[string][]EntryData{
		"team": {},
	}, nil)

	got := p.SubcollectionOr("team")
	if len(got) != 0 {
		t.Fatalf("SubcollectionOr('team') len = %d, want 0 (production, no entries)", len(got))
	}
}

func TestPageData_SubcollectionOr_FallbackEntry_NestedSubcollectionsWork(t *testing.T) {
	p := NewPageData("/", "home", "en", nil, nil, nil)

	got := p.SubcollectionOr("team")
	// The fallback entry should have initialized subcollections map
	// so nested SubcollectionOr calls also work.
	nested := got[0].SubcollectionOr("skills")
	if len(nested) != 1 {
		t.Fatalf("nested SubcollectionOr len = %d, want 1", len(nested))
	}
	if nested[0].Text("name") != "" {
		t.Errorf("nested fallback Text = %q, want empty", nested[0].Text("name"))
	}
}

func TestEntryData_SubcollectionOr_ReturnsEntries_WhenPresent(t *testing.T) {
	inner := []EntryData{
		{Fields: map[string]any{"skill": "Go"}},
	}
	e := EntryData{
		Fields:         map[string]any{},
		Subcollections: map[string][]EntryData{"skills": inner},
	}

	got := e.SubcollectionOr("skills")
	if len(got) != 1 {
		t.Fatalf("EntryData.SubcollectionOr len = %d, want 1", len(got))
	}
	if got[0].Text("skill") != "Go" {
		t.Errorf("got[0].Text('skill') = %q", got[0].Text("skill"))
	}
}

func TestEntryData_SubcollectionOr_ReturnsOneEmpty_WhenMissing(t *testing.T) {
	e := EntryData{Fields: map[string]any{}}

	got := e.SubcollectionOr("skills")
	if len(got) != 1 {
		t.Fatalf("EntryData.SubcollectionOr len = %d, want 1", len(got))
	}
	if got[0].Text("name") != "" {
		t.Errorf("fallback Text = %q, want empty", got[0].Text("name"))
	}
}

// ---------------------------------------------------------------------------
// ImageValue.SrcSet
// ---------------------------------------------------------------------------

func TestImageValue_SrcSet_GeneratesWidths(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.SrcSet(400, 800, 1200)
	want := "https://cdn.test/hero.jpg?w=400 400w, https://cdn.test/hero.jpg?w=800 800w, https://cdn.test/hero.jpg?w=1200 1200w"
	if got != want {
		t.Errorf("SrcSet =\n  %q\nwant\n  %q", got, want)
	}
}

func TestImageValue_SrcSet_WithExistingQuery(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg?token=abc"}
	got := img.SrcSet(400, 800)
	want := "https://cdn.test/hero.jpg?token=abc&w=400 400w, https://cdn.test/hero.jpg?token=abc&w=800 800w"
	if got != want {
		t.Errorf("SrcSet =\n  %q\nwant\n  %q", got, want)
	}
}

func TestImageValue_SrcSet_EmptyURL(t *testing.T) {
	img := ImageValue{}
	got := img.SrcSet(400, 800)
	if got != "" {
		t.Errorf("SrcSet = %q, want empty", got)
	}
}

func TestImageValue_SrcSet_NoWidths(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.SrcSet()
	if got != "" {
		t.Errorf("SrcSet() = %q, want empty", got)
	}
}

func TestImageValue_LQIP_ReturnsTinyURL(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.LQIP()
	want := "https://cdn.test/hero.jpg?w=32&q=20"
	if got != want {
		t.Errorf("LQIP = %q, want %q", got, want)
	}
}

func TestImageValue_LQIP_WithExistingQuery(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg?token=abc"}
	got := img.LQIP()
	want := "https://cdn.test/hero.jpg?token=abc&w=32&q=20"
	if got != want {
		t.Errorf("LQIP = %q, want %q", got, want)
	}
}

func TestImageValue_LQIP_EmptyURL(t *testing.T) {
	img := ImageValue{}
	got := img.LQIP()
	if got != "" {
		t.Errorf("LQIP = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// ImageValue Src / SrcSet / SrcSetFor / HasFormat tests
// ---------------------------------------------------------------------------

func TestImageValue_Src_NoOptions(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg", Alt: "Hero"}
	if img.Src() != "https://cdn.test/hero.jpg" {
		t.Errorf("Src() = %q", img.Src())
	}
}

func TestImageValue_Src_EmptyURL(t *testing.T) {
	img := ImageValue{}
	if img.Src() != "" {
		t.Errorf("Src() = %q, want empty", img.Src())
	}
}

func TestImageValue_Src_WithWidth(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.Src(Width(400))
	// Should append w query param
	if got != "https://cdn.test/hero.jpg?w=400" {
		t.Errorf("Src(Width(400)) = %q", got)
	}
}

func TestImageValue_Src_WithMultipleOptions(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.Src(Width(800), Height(600), Quality(85), Format("webp"))
	// Should have all params
	want := "https://cdn.test/hero.jpg?w=800&h=600&q=85&format=webp"
	if got != want {
		t.Errorf("Src(...) = %q, want %q", got, want)
	}
}

func TestImageValue_Src_WithExistingQueryString(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg?token=abc"}
	got := img.Src(Width(400))
	want := "https://cdn.test/hero.jpg?token=abc&w=400"
	if got != want {
		t.Errorf("Src(Width(400)) = %q, want %q", got, want)
	}
}

func TestImageValue_Src_Resolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg": "/media/abc123.jpg",
		},
	}
	got := img.Src()
	if got != "/media/abc123.jpg" {
		t.Errorf("Src() resolved = %q, want /media/abc123.jpg", got)
	}
}

func TestImageValue_Src_ResolvedWithWidth(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=800": "/media/abc123_800w.jpg",
		},
	}
	got := img.Src(Width(800))
	if got != "/media/abc123_800w.jpg" {
		t.Errorf("Src(Width(800)) resolved = %q, want /media/abc123_800w.jpg", got)
	}
}

func TestImageValue_Src_ResolvedMiss_FallsBackToRemote(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			// Only LQIP is resolved, not the base URL
			"https://cdn.test/hero.jpg?w=32&q=20": "/media/abc_lqip.jpg",
		},
	}
	got := img.Src()
	if got != "https://cdn.test/hero.jpg" {
		t.Errorf("Src() unresolved = %q, want remote URL", got)
	}
}

func TestImageValue_SrcSet_Resolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400":  "/media/abc_400w.jpg",
			"https://cdn.test/hero.jpg?w=800":  "/media/abc_800w.jpg",
			"https://cdn.test/hero.jpg?w=1200": "/media/abc_1200w.jpg",
		},
	}
	got := img.SrcSet(400, 800, 1200)
	want := "/media/abc_400w.jpg 400w, /media/abc_800w.jpg 800w, /media/abc_1200w.jpg 1200w"
	if got != want {
		t.Errorf("SrcSet() resolved = %q, want %q", got, want)
	}
}

func TestImageValue_SrcSet_PartiallyResolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400": "/media/abc_400w.jpg",
			// 800 not resolved
		},
	}
	got := img.SrcSet(400, 800)
	// 400w should be local, 800w should be remote
	if !strings.Contains(got, "/media/abc_400w.jpg 400w") {
		t.Errorf("SrcSet() should contain local 400w path, got %q", got)
	}
	if !strings.Contains(got, "https://cdn.test/hero.jpg?w=800 800w") {
		t.Errorf("SrcSet() should contain remote 800w URL, got %q", got)
	}
}

func TestImageValue_LQIP_Resolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=32&q=20": "data:image/jpeg;base64,/9j/abc123",
		},
	}
	got := img.LQIP()
	if got != "data:image/jpeg;base64,/9j/abc123" {
		t.Errorf("LQIP() resolved = %q, want data URI", got)
	}
}

func TestImageValue_LQIP_ResolvedMiss(t *testing.T) {
	img := ImageValue{
		URL:      "https://cdn.test/hero.jpg",
		resolved: map[string]string{},
	}
	got := img.LQIP()
	if got != "https://cdn.test/hero.jpg?w=32&q=20" {
		t.Errorf("LQIP() unresolved = %q, want remote URL", got)
	}
}

func TestImageValue_SrcSetFor_EmptyURL(t *testing.T) {
	img := ImageValue{}
	got := img.SrcSetFor("webp", 400, 800)
	if got != "" {
		t.Errorf("SrcSetFor empty URL = %q, want empty", got)
	}
}

func TestImageValue_SrcSetFor_EmptyFormat(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.SrcSetFor("", 400, 800)
	if got != "" {
		t.Errorf("SrcSetFor empty format = %q, want empty", got)
	}
}

func TestImageValue_SrcSetFor_NoWidths(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.SrcSetFor("webp")
	if got != "" {
		t.Errorf("SrcSetFor no widths = %q, want empty", got)
	}
}

func TestImageValue_SrcSetFor_RemoteURLs(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	got := img.SrcSetFor("webp", 400, 800)
	want := "https://cdn.test/hero.jpg?w=400&format=webp 400w, https://cdn.test/hero.jpg?w=800&format=webp 800w"
	if got != want {
		t.Errorf("SrcSetFor remote = %q, want %q", got, want)
	}
}

func TestImageValue_SrcSetFor_ExistingQueryString(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg?token=abc"}
	got := img.SrcSetFor("avif", 400)
	want := "https://cdn.test/hero.jpg?token=abc&w=400&format=avif 400w"
	if got != want {
		t.Errorf("SrcSetFor with qs = %q, want %q", got, want)
	}
}

func TestImageValue_SrcSetFor_Resolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400&format=webp":  "/media/abc_400w.webp",
			"https://cdn.test/hero.jpg?w=800&format=webp":  "/media/abc_800w.webp",
			"https://cdn.test/hero.jpg?w=1200&format=webp": "/media/abc_1200w.webp",
		},
	}
	got := img.SrcSetFor("webp", 400, 800, 1200)
	want := "/media/abc_400w.webp 400w, /media/abc_800w.webp 800w, /media/abc_1200w.webp 1200w"
	if got != want {
		t.Errorf("SrcSetFor resolved = %q, want %q", got, want)
	}
}

func TestImageValue_SrcSetFor_PartiallyResolved(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400&format=webp": "/media/abc_400w.webp",
			// 800w not resolved
		},
	}
	got := img.SrcSetFor("webp", 400, 800)
	if !strings.Contains(got, "/media/abc_400w.webp 400w") {
		t.Errorf("SrcSetFor should contain local 400w, got %q", got)
	}
	if !strings.Contains(got, "https://cdn.test/hero.jpg?w=800&format=webp 800w") {
		t.Errorf("SrcSetFor should contain remote 800w, got %q", got)
	}
}

func TestImageValue_HasFormat_NilResolved(t *testing.T) {
	img := ImageValue{URL: "https://cdn.test/hero.jpg"}
	if img.HasFormat("webp") {
		t.Error("HasFormat should be false with nil resolved map")
	}
}

func TestImageValue_HasFormat_EmptyURL(t *testing.T) {
	img := ImageValue{resolved: map[string]string{}}
	if img.HasFormat("webp") {
		t.Error("HasFormat should be false with empty URL")
	}
}

func TestImageValue_HasFormat_EmptyFormat(t *testing.T) {
	img := ImageValue{
		URL:      "https://cdn.test/hero.jpg",
		resolved: map[string]string{"x?w=400&format=webp": "/m/a.webp"},
	}
	if img.HasFormat("") {
		t.Error("HasFormat should be false with empty format")
	}
}

func TestImageValue_HasFormat_True(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400&format=webp": "/media/abc.webp",
		},
	}
	if !img.HasFormat("webp") {
		t.Error("HasFormat('webp') should be true")
	}
}

func TestImageValue_HasFormat_False(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400": "/media/abc.jpg",
		},
	}
	if img.HasFormat("avif") {
		t.Error("HasFormat('avif') should be false when no avif variants")
	}
}

func TestImageValue_HasFormat_DistinguishesFormats(t *testing.T) {
	img := ImageValue{
		URL: "https://cdn.test/hero.jpg",
		resolved: map[string]string{
			"https://cdn.test/hero.jpg?w=400&format=webp": "/media/abc.webp",
		},
	}
	if img.HasFormat("avif") {
		t.Error("HasFormat('avif') should be false when only webp exists")
	}
	if !img.HasFormat("webp") {
		t.Error("HasFormat('webp') should be true")
	}
}
