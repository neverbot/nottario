package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

// docsMarkdown is the configured goldmark instance shared by every
// page render. GFM table + strikethrough + linkify, raw HTML allowed
// (so the rare inline HTML in pages survives), auto heading IDs.
// Syntax highlighting is deliberately not enabled in v1 — chroma
// adds a sizeable dep and we don't have many code blocks yet.
var docsMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// Page is one Markdown source file with its parsed front matter and
// raw body. Partial expansion and HTML rendering operate on this.
type Page struct {
	SourcePath  string
	URL         string
	Frontmatter Frontmatter
	Body        string
}

// Frontmatter is the YAML block at the top of every content page.
type Frontmatter struct {
	Title    string `yaml:"title"`
	Section  string `yaml:"section"`
	NavOrder int    `yaml:"nav_order"`
}

// NavLink is one entry in the side navigation.
type NavLink struct {
	Title string
	URL   string
}

// NavSection groups NavLinks under a section heading.
type NavSection struct {
	Section string
	Pages   []NavLink
}

// PageView is what gets passed to the layout template.
type PageView struct {
	Title    string
	Section  string
	URL      string
	BaseURL  string
	BodyHTML template.HTML
	Nav      []NavSection
}

// parsePage reads one .md file and returns a Page. Returns an error
// if the front matter is missing, malformed, or missing the required
// title field.
func parsePage(contentRoot, relPath string) (*Page, error) {
	raw, err := os.ReadFile(filepath.Join(contentRoot, relPath))
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return nil, fmt.Errorf("%s: missing front matter (must start with ---)", relPath)
	}
	rest := raw[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return nil, fmt.Errorf("%s: unterminated front matter", relPath)
	}
	var fm Frontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return nil, fmt.Errorf("%s: front matter: %w", relPath, err)
	}
	if fm.Title == "" {
		return nil, fmt.Errorf("%s: front matter missing required field 'title'", relPath)
	}
	body := string(rest[end+5:])
	return &Page{
		SourcePath:  relPath,
		URL:         urlFor(relPath),
		Frontmatter: fm,
		Body:        body,
	}, nil
}

// urlFor maps a source path to its public URL.
// "index.md" -> "/", "getting-started.md" -> "/getting-started/",
// "mcp/tools.md" -> "/mcp/tools/".
func urlFor(relPath string) string {
	if relPath == "index.md" {
		return "/"
	}
	trimmed := strings.TrimSuffix(relPath, ".md")
	return "/" + filepath.ToSlash(trimmed) + "/"
}

// loadPages walks contentRoot and returns every .md file as a Page.
func loadPages(contentRoot string) ([]*Page, error) {
	var pages []*Page
	err := filepath.WalkDir(contentRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(contentRoot, path)
		if err != nil {
			return err
		}
		p, err := parsePage(contentRoot, rel)
		if err != nil {
			return err
		}
		pages = append(pages, p)
		return nil
	})
	return pages, err
}

// renderBody converts Markdown to HTML and rewrites internal absolute
// hrefs/srcs with the configured base URL.
func renderBody(body string) (string, error) {
	var buf bytes.Buffer
	if err := docsMarkdown.Convert([]byte(body), &buf); err != nil {
		return "", fmt.Errorf("goldmark: %w", err)
	}
	return rewriteInternalHrefs(buf.String()), nil
}

// internalAttrRE matches href="/path" or src="/path" attributes whose
// path starts with a single slash followed by a letter, digit or "#"
// — i.e. site-internal absolute. Protocol-relative ("//cdn") and
// full URLs ("https://…") don't match.
var internalAttrRE = regexp.MustCompile(`(href|src)="(/[A-Za-z0-9#][^"]*)"`)

func rewriteInternalHrefs(s string) string {
	if baseURL == "" {
		return s
	}
	return internalAttrRE.ReplaceAllStringFunc(s, func(match string) string {
		g := internalAttrRE.FindStringSubmatch(match)
		return g[1] + `="` + withBase(g[2]) + `"`
	})
}

// writePage materialises a Page to disk as outDir/<URL>/index.html.
func writePage(outDir string, p *Page, html string) error {
	target := filepath.Join(outDir, strings.TrimPrefix(p.URL, "/"), "index.html")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(html), 0o644)
}

// renderAll loads, expands, renders, and writes every page, plus the
// client-side search index. Skills bundled with the binary are read
// from disk and surfaced under /skills/ so the docs are guaranteed
// to ship the latest version on every build.
func renderAll(inDir, outDir string) error {
	pages, err := allPages(inDir)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return errors.New("no .md files found under " + inDir)
	}
	nav := buildNav(pages)
	expanded := make(map[string]string, len(pages))
	for _, p := range pages {
		e, err := expandPartials(p.Body)
		if err != nil {
			return fmt.Errorf("%s: %w", p.SourcePath, err)
		}
		expanded[p.SourcePath] = e
		body, err := renderBody(e)
		if err != nil {
			return fmt.Errorf("%s: %w", p.SourcePath, err)
		}
		view := PageView{
			Title:    p.Frontmatter.Title,
			Section:  p.Frontmatter.Section,
			URL:      p.URL,
			BaseURL:  baseURL,
			BodyHTML: template.HTML(body), //nolint:gosec // body is goldmark output, already escaped
			Nav:      nav,
		}
		var out bytes.Buffer
		if err := docsTemplates.ExecuteTemplate(&out, "layout", view); err != nil {
			return fmt.Errorf("%s: render layout: %w", p.SourcePath, err)
		}
		if err := writePage(outDir, p, out.String()); err != nil {
			return err
		}
	}
	return writeSearchIndex(outDir, buildSearchIndex(pages, expanded))
}

// sectionOrder is the order in which sections appear in the side nav.
// Sections not in this list go last in alphabetical order.
var sectionOrder = []string{"Start", "Reference", "Skills", "Operating", "Updates"}

func buildNav(pages []*Page) []NavSection {
	const misc = "Misc"
	groups := map[string][]NavLink{}
	for _, p := range pages {
		sec := p.Frontmatter.Section
		if sec == "" {
			if p.URL == "/" {
				continue // homepage doesn't appear in side nav
			}
			sec = misc
		}
		groups[sec] = append(groups[sec], NavLink{Title: p.Frontmatter.Title, URL: p.URL})
	}
	for sec := range groups {
		links := groups[sec]
		orderOf := map[string]int{}
		titleOf := map[string]string{}
		for _, p := range pages {
			orderOf[p.URL] = p.Frontmatter.NavOrder
			titleOf[p.URL] = p.Frontmatter.Title
		}
		sort.SliceStable(links, func(i, j int) bool {
			ai, aj := orderOf[links[i].URL], orderOf[links[j].URL]
			if ai != aj {
				return ai < aj
			}
			return links[i].URL < links[j].URL
		})
		groups[sec] = links
	}
	known := map[string]bool{}
	for _, s := range sectionOrder {
		known[s] = true
	}
	out := make([]NavSection, 0, len(groups))
	for _, sec := range sectionOrder {
		if len(groups[sec]) > 0 {
			out = append(out, NavSection{Section: sec, Pages: groups[sec]})
		}
	}
	// Trailing sections not in the canonical order, alphabetical.
	extras := make([]string, 0)
	for sec := range groups {
		if !known[sec] {
			extras = append(extras, sec)
		}
	}
	sort.Strings(extras)
	for _, sec := range extras {
		out = append(out, NavSection{Section: sec, Pages: groups[sec]})
	}
	return out
}
