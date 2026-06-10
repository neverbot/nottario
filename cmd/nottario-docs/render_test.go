package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUrlFor(t *testing.T) {
	cases := map[string]string{
		"index.md":           "/",
		"getting-started.md": "/getting-started/",
		"mcp/tools.md":       "/mcp/tools/",
	}
	for in, want := range cases {
		if got := urlFor(in); got != want {
			t.Errorf("urlFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePageRejectsMissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "bad.md"), []byte("just body\n"), 0o644))
	if _, err := parsePage(dir, "bad.md"); err == nil {
		t.Fatal("expected error for missing front matter")
	}
}

func TestParsePageRejectsMissingTitle(t *testing.T) {
	dir := t.TempDir()
	body := "---\nsection: Foo\n---\nbody\n"
	must(t, os.WriteFile(filepath.Join(dir, "bad.md"), []byte(body), 0o644))
	if _, err := parsePage(dir, "bad.md"); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("want title error, got %v", err)
	}
}

func TestParsePageAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	body := "---\ntitle: Hi\nsection: Start\nnav_order: 3\n---\n# Heading\n\nBody.\n"
	must(t, os.WriteFile(filepath.Join(dir, "ok.md"), []byte(body), 0o644))
	p, err := parsePage(dir, "ok.md")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Frontmatter.Title != "Hi" || p.Frontmatter.Section != "Start" || p.Frontmatter.NavOrder != 3 {
		t.Errorf("front matter mismatch: %+v", p.Frontmatter)
	}
	if !strings.Contains(p.Body, "# Heading") {
		t.Errorf("body lost: %q", p.Body)
	}
}

func TestRenderBodyApplyBaseURL(t *testing.T) {
	prev := baseURL
	baseURL = "/nottario"
	defer func() { baseURL = prev }()
	got, err := renderBody("[link](/getting-started/)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `href="/nottario/getting-started/"`) {
		t.Errorf("base URL not applied: %q", got)
	}
}

func TestRunEndToEnd(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	must(t, os.WriteFile(filepath.Join(in, "index.md"),
		[]byte("---\ntitle: Home\n---\n# Home\n\nWelcome.\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(in, "page.md"),
		[]byte("---\ntitle: Page\nsection: Start\n---\n# Page\n\nBody.\n"), 0o644))
	if err := run(in, out, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"index.html", "page/index.html", "static/docs.css", "search-index.json"} {
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("missing artefact %s: %v", want, err)
		}
	}
}

func TestRunChecksCatchesBrokenLink(t *testing.T) {
	in := t.TempDir()
	must(t, os.WriteFile(filepath.Join(in, "index.md"),
		[]byte("---\ntitle: Home\n---\nSee [docs](/does-not-exist/).\n"), 0o644))
	err := runChecks(in)
	if err == nil || !strings.Contains(err.Error(), "broken internal link") {
		t.Fatalf("expected broken-link error, got %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
