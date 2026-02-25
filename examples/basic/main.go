// Example: A-Line CMS static site with Go/templ.
//
// The pages/ directory follows file-based routing conventions:
//
//	pages/
//	  layout.templ        # shared layout (ignored by router)
//	  index.templ         # → /
//	  about.templ         # → /about
//	  blog/
//	    index.templ       # → /blog  (listing)
//	    entry.templ       # → /blog/:slug (entry)
//
// Usage:
//
//	go run . generate            # generate routes_gen.go from pages/
//	go run . build               # build static HTML + sync.json
//	go run . build static        # build static HTML only
//	go run . build sync          # build sync.json only
//	go run . serve               # serve dist/ on :8080
//	go run . sync                # build + POST sync payload to CMS
//	go run . dev                 # dev server with rebuild endpoint
package main

import (
	"os"

	cms "go.a-line.be/cms"
)

func main() {
	app := cms.NewApp(cms.Config{
		APIURL:   envOr("CMS_API_URL", "http://localhost:8000"),
		SiteSlug: envOr("CMS_SITE_SLUG", "example"),
		APIKey:   envOr("CMS_API_KEY", ""),
	})

	// Register routes from the generated routes_gen.go.
	// Run `go run . generate` (or `just generate`) to create/update it.
	RegisterRoutes(app)

	// Dispatch CLI command: build, serve, sync, dev, generate.
	app.Run()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
