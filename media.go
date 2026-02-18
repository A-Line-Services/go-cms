package cms

import (
	"fmt"
	"strconv"
	"strings"
)

// Src returns the image URL with optional processing parameters appended
// as query params. If no options are given, returns the raw URL.
// During static builds, returns the pre-downloaded local path if available.
func (i ImageValue) Src(opts ...MediaOption) string {
	if i.URL == "" {
		return ""
	}
	url := buildMediaURL(i.URL, opts...)
	if i.resolved != nil {
		if local, ok := i.resolved[url]; ok {
			return local
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
