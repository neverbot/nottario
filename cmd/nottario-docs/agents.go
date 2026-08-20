package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// This file carries the agent-facing outputs of the docs site: a
// plain-Markdown twin of every page, and the hand-written llms.txt
// index that points at them.
//
// The llms.txt proposal (https://llmstxt.org) asks for two things. The
// first is a small curated index at /llms.txt — we keep that
// hand-written under docs/site/content/ because the curation IS the
// value; a generated dump of every page would defeat the purpose. The
// second is that "pages with information that agents might need
// provide a clean markdown version of those pages at the same URL as
// the original page […] with the extension replaced by .md". That part
// is mechanical, so the generator emits it.
//
// We deliberately do NOT emit llms-full.txt (one file with every page
// concatenated). It is a Mintlify convention, not part of the
// proposal, and the per-page .md twins already give an agent
// everything without a second copy of the corpus to keep in sync.

// llmsTxtName is the file the site promises at its root. It lives in
// the content directory (the walker only picks up .md, so it is not
// mistaken for a page) and is copied verbatim into the output.
const llmsTxtName = "llms.txt"

// markdownURLFor maps a page URL to the path of its Markdown twin,
// relative to the output directory.
//
//	"/"                -> "index.md"
//	"/getting-started/" -> "getting-started.md"
//	"/skills/tasks/"    -> "skills/tasks.md"
func markdownURLFor(pageURL string) string {
	trimmed := strings.Trim(pageURL, "/")
	if trimmed == "" {
		return "index.md"
	}
	return trimmed + ".md"
}

// writeMarkdown materialises the plain-Markdown twin of a page. body
// is the partial-expanded source, not the rendered HTML: an agent
// asking for .md wants the prose, not a DOM.
func writeMarkdown(outDir string, p *Page, body string) error {
	target := filepath.Join(outDir, filepath.FromSlash(markdownURLFor(p.URL)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(rewriteMarkdownLinks(body)), 0o644)
}

// internalMarkdownLinkRE matches the `](/path)` half of a markdown
// link whose target is site-internal absolute. Protocol-relative
// ("//cdn") and full URLs do not match.
var internalMarkdownLinkRE = regexp.MustCompile(`\]\((/[A-Za-z0-9#][^)\s]*)\)`)

// rewriteMarkdownLinks applies the configured base URL to markdown
// links, mirroring what rewriteInternalHrefs does for rendered HTML.
// Without this the .md twins would link to "/getting-started/" while
// the site actually lives under "/nottario/getting-started/".
func rewriteMarkdownLinks(s string) string {
	if baseURL == "" {
		return s
	}
	return internalMarkdownLinkRE.ReplaceAllStringFunc(s, func(match string) string {
		g := internalMarkdownLinkRE.FindStringSubmatch(match)
		return "](" + withBase(g[1]) + ")"
	})
}

// copyLLMsTxt copies the hand-written llms.txt from the content
// directory to the output root. Missing source is an error: the file
// is a promise the site makes to agents, and silently dropping it on
// a rename is exactly the failure this build step exists to prevent.
func copyLLMsTxt(inDir, outDir string) error {
	b, err := os.ReadFile(filepath.Join(inDir, llmsTxtName))
	if err != nil {
		return fmt.Errorf("read %s: %w", llmsTxtName, err)
	}
	return os.WriteFile(filepath.Join(outDir, llmsTxtName), b, 0o644)
}
