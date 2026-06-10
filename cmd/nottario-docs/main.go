// Command nottario-docs renders the static documentation site.
//
// It reads Markdown content from a source directory (default
// docs/site/content), expands a small set of partials against
// handlers in this binary, and writes static HTML to an output
// directory (default docs/site/dist).
//
// Invocation:
//
//	nottario-docs --in docs/site/content --out docs/site/dist
//	nottario-docs --check --in docs/site/content
//	nottario-docs --base-url /nottario   (for GitHub Pages subdirectory hosting)
//
// In --check mode the binary validates content (front matter,
// internal links, partial references) without writing any output,
// and exits non-zero on failure.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

//go:embed static/docs.css
var docsCSS []byte

//go:embed static/search.js
var searchJS []byte

// baseURL is the URL prefix every site-internal absolute path lives
// under. Empty means rooted at "/"; set to "/nottario" to publish
// under a GitHub Pages subdirectory. Read by templates and partials
// via the withBase helper so a fresh build with a different prefix
// is just a flag away.
var baseURL = ""

// withBase prepends baseURL to a site-internal absolute path. The
// input must start with "/"; the output is the same path with the
// prefix stitched in, with no double-slash.
func withBase(p string) string {
	if !strings.HasPrefix(p, "/") {
		return p
	}
	if baseURL == "" {
		return p
	}
	return baseURL + p
}

func main() {
	in := flag.String("in", "docs/site/content", "content source directory")
	out := flag.String("out", "docs/site/dist", "output directory (ignored in --check)")
	check := flag.Bool("check", false, "validate without writing output")
	base := flag.String("base-url", "", "URL prefix (e.g. /nottario) for hosting under a subdirectory; empty for root")
	flag.Parse()

	baseURL = strings.TrimRight(strings.TrimSpace(*base), "/")

	if err := run(*in, *out, *check); err != nil {
		slog.Error("nottario-docs failed", "err", err)
		os.Exit(1)
	}
}

// run is the testable entrypoint. It either validates content
// (check=true) or renders the site to outDir.
func run(inDir, outDir string, check bool) error {
	if check {
		return runChecks(inDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}
	staticDir := filepath.Join(outDir, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		return fmt.Errorf("mkdir static: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "docs.css"), docsCSS, 0o644); err != nil {
		return fmt.Errorf("write docs.css: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "search.js"), searchJS, 0o644); err != nil {
		return fmt.Errorf("write search.js: %w", err)
	}
	return renderAll(inDir, outDir)
}
