package cms

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Run dispatches CLI commands based on os.Args. Supports:
//
//	build [static|sync]  — generate static HTML and/or sync payload
//	serve                — build and serve static files locally
//	sync [file]          — build and POST sync payload to CMS
//	dev                  — dev server with rebuild endpoint
//
// If no command is given, prints usage and exits.
func (a *App) Run() {
	if len(os.Args) < 2 {
		a.printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "build":
		a.runBuild()
	case "serve":
		a.runServe()
	case "sync":
		a.runSyncCmd()
	case "dev":
		a.runDev()
	case "generate":
		runGenerate()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		a.printUsage()
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// build [static|sync]
// ---------------------------------------------------------------------------

func (a *App) runBuild() {
	args := os.Args[2:]

	// Check if first arg is a subcommand (not a flag).
	var subcommand string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subcommand = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("build", flag.ExitOnError)
	outDir := fs.String("out", "dist", "output directory for static HTML")
	syncFile := fs.String("sync-file", "sync.json", "sync file path")
	downloadMedia := fs.Bool("media", true, "download CMS media to output dir")
	minifyHTML := fs.Bool("minify", true, "minify HTML/CSS/JS output")
	_ = fs.Parse(args)

	switch subcommand {
	case "":
		// Build both static + sync.
		err := a.Build(context.Background(), BuildOptions{
			OutDir:        *outDir,
			SyncFile:      *syncFile,
			DownloadMedia: *downloadMedia,
			Minify:        *minifyHTML,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
			os.Exit(1)
		}
		pageCount := len(a.pages) + len(a.collections)
		fmt.Printf("built %d page(s) to %s\n", pageCount, *outDir)
		fmt.Printf("wrote sync payload to %s\n", *syncFile)

	case "static":
		// Build static HTML only.
		err := a.Build(context.Background(), BuildOptions{
			OutDir:        *outDir,
			DownloadMedia: *downloadMedia,
			Minify:        *minifyHTML,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
			os.Exit(1)
		}
		pageCount := len(a.pages) + len(a.collections)
		fmt.Printf("built %d page(s) to %s\n", pageCount, *outDir)

	case "sync":
		// Build sync.json only.
		if err := a.WriteSyncJSON(*syncFile); err != nil {
			fmt.Fprintf(os.Stderr, "build sync failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote sync payload to %s\n", *syncFile)

	default:
		fmt.Fprintf(os.Stderr, "unknown build target: %s\n", subcommand)
		fmt.Fprintln(os.Stderr, "  build          Build static HTML + sync.json")
		fmt.Fprintln(os.Stderr, "  build static   Build static HTML only")
		fmt.Fprintln(os.Stderr, "  build sync     Build sync.json only")
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// serve
// ---------------------------------------------------------------------------

func (a *App) runServe() {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := fs.String("dir", "dist", "directory to serve")
	port := fs.String("port", envOrDefault("PORT", "8080"), "port to listen on")
	_ = fs.Parse(os.Args[2:])

	serveStatic(*dir, *port)
}

// serveStatic starts an HTTP file server with clean-URL support.
func serveStatic(dir, port string) {
	fileServer := http.FileServer(http.Dir(dir))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try the path as-is first.
		filePath := dir + r.URL.Path
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Try path/index.html for clean URLs.
		indexPath := filePath + "/index.html"
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
		// Try path.html for error pages (e.g. /404 → 404.html).
		htmlPath := filePath + ".html"
		if _, err := os.Stat(htmlPath); err == nil {
			http.ServeFile(w, r, htmlPath)
			return
		}
		// Fallback to default behavior (404 etc.).
		fileServer.ServeHTTP(w, r)
	})

	addr := ":" + port
	fmt.Printf("serving %s on http://localhost%s\n", dir, addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "serve failed: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// sync [file]
// ---------------------------------------------------------------------------

func (a *App) runSyncCmd() {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	_ = fs.Parse(os.Args[2:])

	// If a file argument is provided, POST that file; otherwise build + POST.
	filePath := fs.Arg(0)

	if err := a.PostSync(context.Background(), filePath); err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("sync complete")
}

// ---------------------------------------------------------------------------
// dev
// ---------------------------------------------------------------------------

func (a *App) runDev() {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	port := fs.String("port", envOrDefault("PORT", "3000"), "port to listen on")
	outDir := fs.String("out", ".dev-dist", "build output directory")
	_ = fs.Parse(os.Args[2:])

	// In dev mode, ensure SiteURL is set so sitemap.xml is always generated.
	// If not configured, fall back to the local dev server address.
	if a.config.SiteURL == "" {
		a.config.SiteURL = "http://localhost:" + *port
	}

	opts := BuildOptions{
		OutDir:        *outDir,
		DownloadMedia: true,
	}

	// Sync templates to the CMS so field definitions stay up-to-date.
	if a.config.APIKey != "" {
		fmt.Println("syncing templates...")
		if err := a.PostSync(context.Background(), ""); err != nil {
			fmt.Fprintf(os.Stderr, "sync failed (continuing): %v\n", err)
		} else {
			fmt.Println("sync complete")
		}
	}

	// Initial build.
	fmt.Println("building...")
	if err := a.Build(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "initial build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("initial build complete")

	ds := &devServer{app: a, opts: opts}

	mux := http.NewServeMux()

	// Rebuild endpoint — POST /rebuild triggers a full rebuild.
	mux.HandleFunc("/rebuild", ds.handleRebuild)

	// Serve static files with clean URL and CMS preview support.
	mux.Handle("/", devFileHandler(*outDir))

	addr := ":" + *port
	fmt.Printf("dev server running at http://localhost%s\n", addr)
	fmt.Println("POST /rebuild to trigger rebuild")
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "dev server failed: %v\n", err)
		os.Exit(1)
	}
}

// devServer holds state for the dev mode server.
type devServer struct {
	app  *App
	opts BuildOptions
	mu   sync.Mutex
}

// devFileHandler returns an http.Handler that serves static files from dir
// with clean-URL support. When the X-CMS-Preview header is "true", it serves
// .template.html files (which retain data-cms-* attributes for the CMS
// preview bridge) instead of production index.html files.
//
// Files in the local static/ directory are also served, so changes to
// static assets are reflected immediately without triggering a rebuild.
func devFileHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isPreview := r.Header.Get("X-CMS-Preview") == "true"

		filePath := dir + r.URL.Path
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Serve from static/ directory directly (dev convenience — changes
		// are reflected without a rebuild).
		staticPath := "static" + r.URL.Path
		if info, err := os.Stat(staticPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, staticPath)
			return
		}

		// For clean URLs, resolve to index.html (or index.template.html for previews).
		if isPreview {
			templatePath := filePath + "/index.template.html"
			if _, err := os.Stat(templatePath); err == nil {
				http.ServeFile(w, r, templatePath)
				return
			}
		}
		indexPath := filePath + "/index.html"
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
		// Try path.html for error pages (e.g. /404 → 404.html).
		htmlPath := filePath + ".html"
		if _, err := os.Stat(htmlPath); err == nil {
			http.ServeFile(w, r, htmlPath)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (ds *devServer) handleRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Let the site reload any cached state (e.g. Vite manifest) before
	// rebuilding so that generated HTML references the latest asset hashes.
	if fn := ds.app.config.BeforeRebuild; fn != nil {
		fn()
	}

	fmt.Println("rebuilding...")
	if err := ds.app.Build(context.Background(), ds.opts); err != nil {
		http.Error(w, "rebuild failed: "+err.Error(), http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, "rebuild failed: %v\n", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "rebuild complete")
	fmt.Println("rebuild complete")
}

// ---------------------------------------------------------------------------
// generate
// ---------------------------------------------------------------------------

// runGenerate is a standalone command (not on App) — it reads go.mod to
// discover the module path and generates routes_gen.go from the pages dir.
func runGenerate() {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	pagesDir := fs.String("pages", "pages", "pages directory to scan")
	outFile := fs.String("out", "routes_gen.go", "output file")
	pkg := fs.String("package", "main", "Go package for generated file")
	_ = fs.Parse(os.Args[2:])

	modPath, subdir, err := findModulePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not find go.mod: %v\n", err)
		os.Exit(1)
	}

	// If we're in a subdirectory, the module path for imports needs
	// to include the subdir prefix (e.g. "github.com/foo/bar/examples/basic").
	importBase := modPath
	if subdir != "" {
		importBase = modPath + "/" + filepath.ToSlash(subdir)
	}

	cfg := GenerateConfig{
		PagesDir:   *pagesDir,
		ModulePath: importBase,
		Package:    *pkg,
	}

	if err := WriteGeneratedRoutes(cfg, *outFile); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s\n", *outFile)
}

// findModulePath walks up from the current directory to find go.mod and
// returns (modulePath, subdir) where subdir is the relative path from
// the module root to the current directory (empty string if at root).
func findModulePath() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	dir := cwd
	for {
		gomod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(gomod)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					modPath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
					subdir, _ := filepath.Rel(dir, cwd)
					if subdir == "." {
						subdir = ""
					}
					return modPath, subdir, nil
				}
			}
			return "", "", fmt.Errorf("no module directive found in %s", gomod)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("go.mod not found in %s or any parent directory", cwd)
		}
		dir = parent
	}
}

// envOrDefault returns the environment variable value if set, otherwise fallback.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------------
// usage
// ---------------------------------------------------------------------------

func (a *App) printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: <program> <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  generate             Generate route registrations from pages directory")
	fmt.Fprintln(os.Stderr, "  build [static|sync]  Build static HTML and/or sync payload")
	fmt.Fprintln(os.Stderr, "  serve                Serve static files for local preview")
	fmt.Fprintln(os.Stderr, "  sync [file]          Build and POST sync payload to CMS")
	fmt.Fprintln(os.Stderr, "  dev                  Dev server with rebuild endpoint")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run '<program> <command> -h' for command-specific flags.")
}
