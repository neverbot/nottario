package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SearchEntry is one indexed page. The static/search.js consumer
// filters these client-side; no server-side search runtime.
type SearchEntry struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Section string `json:"section,omitempty"`
	Body    string `json:"body"`
}

// stripTagsRE removes HTML tags from a rendered body so the search
// index stores plain text, not markup. It's intentionally lenient:
// any `<...>` is dropped.
var stripTagsRE = regexp.MustCompile(`<[^>]+>`)

// collapseWSRE flattens runs of whitespace into a single space so the
// search index is compact and excerpting is predictable.
var collapseWSRE = regexp.MustCompile(`\s+`)

// buildSearchIndex strips markdown/HTML to plain text and returns one
// entry per page. The expanded map carries each page's body after
// partial expansion but BEFORE goldmark rendering — close enough to
// human-readable for search.
func buildSearchIndex(pages []*Page, expanded map[string]string) []SearchEntry {
	entries := make([]SearchEntry, 0, len(pages))
	for _, p := range pages {
		body := expanded[p.SourcePath]
		if body == "" {
			body = p.Body
		}
		body = stripTagsRE.ReplaceAllString(body, " ")
		body = collapseWSRE.ReplaceAllString(body, " ")
		body = strings.TrimSpace(body)
		entries = append(entries, SearchEntry{
			URL:     p.URL,
			Title:   p.Frontmatter.Title,
			Section: p.Frontmatter.Section,
			Body:    body,
		})
	}
	return entries
}

func writeSearchIndex(outDir string, entries []SearchEntry) error {
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "search-index.json"), b, 0o644)
}
