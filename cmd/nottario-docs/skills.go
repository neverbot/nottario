package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// skillsRoot is the on-disk path to the skill bundle that ships in
// the binary. The generator runs from the repo root, so this is a
// relative path. Kept var-not-const so tests can swap it.
var skillsRoot = "internal/skill/files"

// skillMapping is the manually-curated list of skill files that get
// surfaced on the docs site, in display order, with friendly titles.
// New skill files do not appear automatically — adding one to the
// bundle is also a docs decision (does it deserve its own page,
// what's its title and nav order). Missing the live `.md` on disk is
// a build error so the docs cannot silently fall out of sync.
type skillMapping struct {
	source string // path relative to skillsRoot, e.g. "domains/tasks.md"
	slug   string // last URL segment, e.g. "tasks"
	title  string // friendly title shown in nav and as <title>
	order  int    // nav_order within the Skills section
}

var skillPages = []skillMapping{
	{source: "skill.md", slug: "overview", title: "Skill: overview", order: 1},
	{source: "domains/tasks.md", slug: "tasks", title: "Skill: tasks domain", order: 2},
	{source: "domains/cycles.md", slug: "cycles", title: "Skill: cycles domain", order: 3},
	{source: "domains/docs.md", slug: "docs", title: "Skill: documents domain", order: 4},
	{source: "domains/architecture.md", slug: "architecture", title: "Skill: architecture domain", order: 5},
	{source: "references/identity.md", slug: "identity", title: "Skill: identity reference", order: 6},
}

// loadSkillPages reads every entry in skillPages from disk and turns
// it into a docs Page. The skill front matter (name/description) is
// dropped — the docs renderer uses its own front matter shape.
//
// Pages are returned sorted by skillMapping order so the side nav
// reads top-to-bottom.
func loadSkillPages(root string) ([]*Page, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil // tolerable in tests; CI fails earlier via docs-build
		}
		return nil, err
	}
	out := make([]*Page, 0, len(skillPages))
	for _, m := range skillPages {
		raw, err := os.ReadFile(filepath.Join(root, m.source))
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", m.source, err)
		}
		body := stripFrontMatter(raw)
		out = append(out, &Page{
			SourcePath: filepath.Join("skills", m.slug+".md"),
			URL:        "/skills/" + m.slug + "/",
			Frontmatter: Frontmatter{
				Title:    m.title,
				Section:  "Skills",
				NavOrder: m.order,
			},
			Body: string(body),
		})
	}
	return out, nil
}

// stripFrontMatter removes the leading YAML front-matter block (if
// any) from a markdown file. We keep only the body because the docs
// renderer carries its own front-matter shape, synthesised from the
// skillMapping table.
func stripFrontMatter(raw []byte) []byte {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return raw
	}
	rest := raw[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return raw // malformed; leave it for the writer to choke on
	}
	return bytes.TrimLeft(rest[end+5:], "\n")
}

// allPages returns the union of hand-written content pages and the
// synthesised skill pages. Both renderAll and runChecks call this so
// they walk the same corpus.
func allPages(contentRoot string) ([]*Page, error) {
	content, err := loadPages(contentRoot)
	if err != nil {
		return nil, err
	}
	skills, err := loadSkillPages(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("load skill pages: %w", err)
	}
	return append(content, skills...), nil
}
