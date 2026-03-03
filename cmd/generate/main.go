// Standalone route generator. Run with:
//
//	go run go.a-line.be/cms/cmd/generate [flags]
//
// This avoids the chicken-and-egg problem of needing routes_gen.go
// to compile the site binary that generates routes_gen.go.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cms "go.a-line.be/cms"
)

func main() {
	pagesDir := flag.String("pages", "pages", "pages directory to scan")
	outFile := flag.String("out", "routes_gen.go", "output file")
	pkg := flag.String("package", "main", "Go package for generated file")
	flag.Parse()

	modPath, subdir, err := findModulePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not find go.mod: %v\n", err)
		os.Exit(1)
	}

	importBase := modPath
	if subdir != "" {
		importBase = modPath + "/" + filepath.ToSlash(subdir)
	}

	cfg := cms.GenerateConfig{
		PagesDir:   *pagesDir,
		ModulePath: importBase,
		Package:    *pkg,
	}

	if err := cms.WriteGeneratedRoutes(cfg, *outFile); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s\n", *outFile)
}

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
