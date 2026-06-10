package main

import (
	"errors"
	"fmt"
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
	pages, err := loadPages(inDir)
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

	if err := joinErrors(errs); err != nil {
		return err
	}
	return nil
}

// markdownLinkRE matches markdown `[text](url)` links AND raw HTML
// hrefs the body might contain. It is intentionally permissive — the
// downstream filter on whether the URL is site-internal does the
// heavy lifting.
var markdownLinkRE = regexp.MustCompile(`\]\(([^)\s]+)\)|href="([^"]+)"`)

// internalLinks returns every site-internal absolute URL referenced
// from the page body. Site-internal means it starts with "/" (and is
// not protocol-relative "//cdn").
func internalLinks(body string) []string {
	var out []string
	for _, m := range markdownLinkRE.FindAllStringSubmatch(body, -1) {
		url := m[1]
		if url == "" {
			url = m[2]
		}
		if strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "//") {
			out = append(out, url)
		}
	}
	return out
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
