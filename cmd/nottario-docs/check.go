package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// runChecks validates the content corpus without producing output.
// It catches issues a build would catch (front matter, partial
// references) plus issues that only matter at the corpus level
// (internal links pointing nowhere, the latest tag missing from
// whats-new). Returns the first error joined with subsequent ones so
// CI surfaces every fix at once instead of dripping them out.
func runChecks(inDir string) error {
	pages, err := allPages(inDir)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return errors.New("no .md files found under " + inDir)
	}
	known := map[string]bool{}
	for _, p := range pages {
		known[p.URL] = true
	}

	var errs []error
	for _, p := range pages {
		// Partials: expandPartials returns errors for unknown names.
		if _, err := expandPartials(p.Body); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.SourcePath, err))
		}
		// Internal links: every site-internal absolute URL must point
		// at an existing page (anchors and trailing slashes ignored
		// during the lookup).
		for _, link := range internalLinks(p.Body) {
			target := stripFragment(link)
			if !known[normaliseLink(target)] {
				errs = append(errs, fmt.Errorf("%s: broken internal link %q", p.SourcePath, link))
			}
		}
	}

	// llms.txt is hand-written, so nothing forces it to keep up with
	// the corpus. Check it exists and that every page is listed —
	// an unlisted page is invisible to any agent that trusts the
	// index. Runs after the per-page checks so a genuinely broken
	// page surfaces its own error first.
	errs = append(errs, checkLLMsTxt(inDir, pages)...)

	if err := joinErrors(errs); err != nil {
		return err
	}
	return nil
}

// checkLLMsTxt validates the hand-written index at content/llms.txt:
// it must exist, and it must mention the Markdown twin of every page
// in the corpus. It deliberately does not check ordering, prose or
// grouping — those are editorial calls the author makes.
func checkLLMsTxt(inDir string, pages []*Page) []error {
	raw, err := os.ReadFile(filepath.Join(inDir, llmsTxtName))
	if err != nil {
		return []error{fmt.Errorf("%s: %w", llmsTxtName, err)}
	}
	body := string(raw)
	var errs []error
	for _, p := range pages {
		if !strings.Contains(body, markdownURLFor(p.URL)) {
			errs = append(errs, fmt.Errorf("%s: page %q is not listed (expected a link to %q)",
				llmsTxtName, p.URL, markdownURLFor(p.URL)))
		}
	}
	return errs
}

// markdownLinkRE matches markdown `[text](url)` links AND raw HTML
// hrefs the body might contain. It is intentionally permissive — the
// downstream filter on whether the URL is site-internal does the
// heavy lifting.
var markdownLinkRE = regexp.MustCompile(`\]\(([^)\s]+)\)|href="([^"]+)"`)

// internalLinks returns every site-internal absolute URL referenced
// from the page body. Site-internal means it starts with "/" (and is
// not protocol-relative "//cdn"). Asset paths (anything whose last
// segment looks like `name.ext`) are filtered out — they're served
// from copied directories, not from the page corpus.
func internalLinks(body string) []string {
	var out []string
	for _, m := range markdownLinkRE.FindAllStringSubmatch(body, -1) {
		url := m[1]
		if url == "" {
			url = m[2]
		}
		if !strings.HasPrefix(url, "/") || strings.HasPrefix(url, "//") {
			continue
		}
		if isAssetURL(url) {
			continue
		}
		out = append(out, url)
	}
	return out
}

// isAssetURL reports whether the URL points at a static asset (PNG,
// SVG, JSON …) rather than a generated page. The page map only knows
// about generated pages, so checking asset URLs would yield false
// positives. The heuristic: any URL whose last path segment contains
// a `.` is an asset.
func isAssetURL(u string) bool {
	clean := stripFragment(u)
	last := clean
	if i := strings.LastIndex(clean, "/"); i >= 0 {
		last = clean[i+1:]
	}
	return strings.Contains(last, ".")
}

func stripFragment(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		return s[:i]
	}
	return s
}

// normaliseLink turns "/foo" and "/foo/" and "/foo/index.html" into
// the canonical "/foo/" form pages register themselves under.
func normaliseLink(s string) string {
	if s == "" || s == "/" {
		return "/"
	}
	s = strings.TrimSuffix(s, "index.html")
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d validation errors:", len(errs)))
	for _, e := range errs {
		b.WriteString("\n  - ")
		b.WriteString(e.Error())
	}
	return errors.New(b.String())
}
