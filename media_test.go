package cms

import (
	"strings"
	"testing"
)

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

// --- Resolved (static build) tests ---

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

// --- SrcSetFor (format-specific srcset) tests ---

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

// --- HasFormat tests ---

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
