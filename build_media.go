package cms

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Default responsive widths downloaded for each image in static builds.
var defaultSrcSetWidths = []int{400, 800, 1200, 1600}

// Formats to attempt downloading during static build. If the backend does
// not support format conversion (returns 404 or error), the format is
// silently skipped — the <source> element won't be rendered.
var defaultFormats = []string{"avif", "webp"}

// mediaDownloader downloads CMS media assets to the build output directory
// and provides an imageProcessor that rewrites remote URLs to local paths.
type mediaDownloader struct {
	client    *http.Client
	outDir    string            // filesystem dir, e.g., "dist/media"
	webPrefix string            // URL prefix in built HTML, e.g., "/media"
	cache     map[string]string // remote URL -> local web path (or data URI for LQIP)
	mu        sync.Mutex
}

func newMediaDownloader(outDir, webPrefix string) *mediaDownloader {
	return &mediaDownloader{
		client:    &http.Client{},
		outDir:    outDir,
		webPrefix: webPrefix,
		cache:     make(map[string]string),
	}
}

// processor returns an imageProcessor that downloads all variants of an
// image and returns an ImageValue with the resolved map populated.
func (d *mediaDownloader) processor() imageProcessor {
	return func(img ImageValue) ImageValue {
		if img.URL == "" {
			return img
		}
		resolved := make(map[string]string)

		// 1. Download the full-size image (no params).
		if local, err := d.download(img.URL); err == nil {
			resolved[img.URL] = local
		}

		// 2. Download LQIP and base64-inline it.
		lqipURL := buildLQIPURL(img.URL)
		if data, err := d.downloadBase64(lqipURL); err == nil {
			resolved[lqipURL] = data
		}

		// 3. Download srcset width variants (original format).
		sep := "?"
		if strings.Contains(img.URL, "?") {
			sep = "&"
		}
		for _, w := range defaultSrcSetWidths {
			variantURL := img.URL + sep + fmt.Sprintf("w=%d", w)
			if local, err := d.download(variantURL); err == nil {
				resolved[variantURL] = local
			}
		}

		// 4. Download format variants (WebP, AVIF) for <picture> <source> elements.
		//    If the backend doesn't support format conversion, downloads fail
		//    silently and HasFormat() returns false — no <source> is rendered.
		for _, format := range defaultFormats {
			for _, w := range defaultSrcSetWidths {
				variantURL := img.URL + sep + fmt.Sprintf("w=%d&format=%s", w, format)
				if local, err := d.download(variantURL); err == nil {
					resolved[variantURL] = local
				}
			}
		}

		img.resolved = resolved
		return img
	}
}

// download fetches a remote URL, saves it to outDir, and returns the web path.
// Results are cached — repeated calls for the same URL return instantly.
func (d *mediaDownloader) download(remoteURL string) (string, error) {
	d.mu.Lock()
	if local, ok := d.cache[remoteURL]; ok {
		d.mu.Unlock()
		return local, nil
	}
	d.mu.Unlock()

	resp, err := d.client.Get(remoteURL)
	if err != nil {
		return "", fmt.Errorf("cms: download %s: %w", remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("cms: download %s: status %d", remoteURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cms: read %s: %w", remoteURL, err)
	}

	ext := extensionFromResponse(resp, remoteURL)
	filename := hashFilename(remoteURL) + ext

	if err := os.MkdirAll(d.outDir, 0o755); err != nil {
		return "", fmt.Errorf("cms: mkdir %s: %w", d.outDir, err)
	}

	filePath := filepath.Join(d.outDir, filename)
	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		return "", fmt.Errorf("cms: write %s: %w", filePath, err)
	}

	webPath := d.webPrefix + "/" + filename

	d.mu.Lock()
	d.cache[remoteURL] = webPath
	d.mu.Unlock()

	return webPath, nil
}

// downloadBase64 fetches a URL and returns it as a base64 data URI.
// Used for LQIP images that are small enough to inline.
func (d *mediaDownloader) downloadBase64(remoteURL string) (string, error) {
	d.mu.Lock()
	if local, ok := d.cache[remoteURL]; ok {
		d.mu.Unlock()
		return local, nil
	}
	d.mu.Unlock()

	resp, err := d.client.Get(remoteURL)
	if err != nil {
		return "", fmt.Errorf("cms: download %s: %w", remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("cms: download %s: status %d", remoteURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cms: read %s: %w", remoteURL, err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	// Strip parameters (e.g., "image/jpeg; charset=utf-8" → "image/jpeg")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	dataURI := fmt.Sprintf("data:%s;base64,%s", ct, base64.StdEncoding.EncodeToString(body))

	d.mu.Lock()
	d.cache[remoteURL] = dataURI
	d.mu.Unlock()

	return dataURI, nil
}

// hashFilename returns a short deterministic filename from a URL.
func hashFilename(url string) string {
	h := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", h[:8])
}

// extensionFromResponse determines the file extension from the HTTP response.
func extensionFromResponse(resp *http.Response, url string) string {
	// Try Content-Type header first.
	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		exts, _ := mime.ExtensionsByType(ct)
		if len(exts) > 0 {
			// Prefer common extensions.
			for _, ext := range exts {
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif" || ext == ".gif" || ext == ".svg" {
					return ext
				}
			}
			return exts[0]
		}
	}

	// Fall back to URL path extension.
	path := url
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if ext := filepath.Ext(path); ext != "" {
		return ext
	}

	return ".jpg"
}

// buildLQIPURL constructs the LQIP URL for an image (32px, q20).
func buildLQIPURL(baseURL string) string {
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + "w=32&q=20"
}
