// Package markdown renders user-authored markdown into safe HTML for
// the web UI. One engine across every surface that displays markdown
// today (document bodies, task descriptions, comments) so the prose
// chrome stays consistent and the sanitization happens once at the
// backend boundary.
//
// The renderer recognises three cross-domain link chips that are
// Nottario-specific syntax on top of GFM:
//
//	[[task:<short-id>]]          (8-char prefix of the task UUID, or full UUID)
//	[[doc:<path>]]               (logical doc path, project-scoped)
//	[[arch:<slug>]]              (architecture node slug)
//
// When projectID is non-nil the renderer resolves each chip against
// the database and embeds the target's current title (and state, for
// tasks) inline. Unresolved chips render as inert spans with an
// "unknown" hint so the typo is visible in the rendered output rather
// than disappearing silently.
package markdown

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// chipRe matches our three custom chip forms anywhere in the markdown
// source BEFORE goldmark parses it. The pattern is wide enough to
// catch the chip wherever it appears (inline, in tables, in lists) but
// strict enough that it doesn't match double-bracket footnotes from
// other markdown dialects. We pre-process the markdown to replace
// each chip with a stand-in HTML span and then let bluemonday's allow-
// list keep those spans in the final output.
var chipRe = regexp.MustCompile(`\[\[(task|doc|arch):([^\]\s][^\]]*)\]\]`)

// codeRe identifies markdown code regions whose contents must NOT be
// touched by chip substitution: fenced code blocks (```...``` across
// newlines) and inline code spans (`...` on one line). When a user
// writes `[[task:N]]` to describe the chip syntax, that token lives
// inside a code span and should reach goldmark verbatim so it renders
// as literal text. Without this guard the chip pre-pass would replace
// it with HTML, goldmark would then escape that HTML, and the user
// would see <span class="..."> as visible text.
var codeRe = regexp.MustCompile("(?s)(?:```.*?```|`[^`\n]+`)")

// Render parses md as CommonMark + GFM, resolves Nottario link chips
// against pool (skipped when projectID is nil), sanitizes the
// resulting HTML and returns it.
//
// The function is safe to call concurrently. It does several DB
// reads per chip; if performance becomes a concern, the chip cache
// in writeChipsBatch can be promoted to a per-project LRU.
func Render(ctx context.Context, pool *pgxpool.Pool, md string, projectID *uuid.UUID) (string, error) {
	// 1. Pre-process the source: each [[task:…]] / [[doc:…]] / [[arch:…]]
	//    becomes an HTML span literally embedded in the markdown. We
	//    keep the lookups synchronous and batched per Render call so
	//    one document with N chips costs N DB roundtrips total — fine
	//    for the document and task-description sizes we target.
	pre, err := substituteChips(ctx, pool, md, projectID)
	if err != nil {
		return "", err
	}

	// 2. Render markdown → HTML through goldmark with GFM extensions.
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()), // we sanitize below
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(pre), &buf); err != nil {
		return "", fmt.Errorf("goldmark convert: %w", err)
	}

	// 3. Sanitize. The default UGC policy strips scripts, on* handlers,
	//    javascript: URLs etc. We extend it to keep the chip classes
	//    and the language-foo class on <code> blocks (consumed by
	//    highlight.js on the client).
	out := sanitizePolicy().SanitizeBytes(buf.Bytes())
	return string(out), nil
}

// sanitizePolicy returns a bluemonday policy tuned for markdown
// output. UGC base + an allowance for the .chip / .chip-* classes
// the chip transformer emits and for the language-* classes goldmark
// puts on code fences.
func sanitizePolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Allow classes used by our chip spans and by goldmark for fenced
	// code blocks. bluemonday's UGC policy strips class by default.
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^(chip|chip-(task|doc|arch)|chip-state-(todo|doing|done)|chip-missing|language-[a-zA-Z0-9_-]+)( (chip|chip-(task|doc|arch)|chip-state-(todo|doing|done)|chip-missing|language-[a-zA-Z0-9_-]+))*$`)).OnElements("span", "code", "a")
	p.AllowAttrs("data-kind", "data-id", "data-path", "data-slug").OnElements("a", "span")
	// Allow target="_blank" on autolinks would be nice for external
	// URLs but we deliberately don't add it here: every link rendered
	// today is in-app or a fully-typed external URL, and forcing new
	// tabs is a UX choice we should make in the component, not via
	// HTML.
	return p
}

// substituteChips walks the markdown source and replaces each chip
// token with a small HTML span carrying the resolved title (and task
// state). When pool is nil or projectID is nil we still substitute
// the token for an inert "no project context" span, so a chip in a
// global doc never disappears.
func substituteChips(ctx context.Context, pool *pgxpool.Pool, md string, projectID *uuid.UUID) (string, error) {
	if !chipRe.MatchString(md) {
		return md, nil
	}
	// Walk all matches collecting their bounds; then replace in
	// reverse order so earlier indexes stay valid.
	matches := chipRe.FindAllStringSubmatchIndex(md, -1)
	if len(matches) == 0 {
		return md, nil
	}
	// Find code regions whose interior must be left alone. A chip
	// inside `…` or ```…``` is documentation of the chip syntax,
	// not an actual reference; skipping these makes the chip syntax
	// itself render as `[[task:N]]` verbatim.
	codeRanges := codeRe.FindAllStringIndex(md, -1)
	insideCode := func(start, end int) bool {
		for _, r := range codeRanges {
			if start >= r[0] && end <= r[1] {
				return true
			}
		}
		return false
	}
	// We iterate the chips ascending so the cache below shares
	// resolution within one Render call. Then we build the output
	// with a single strings.Builder.
	type chip struct {
		start, end int
		kind, ref  string
		html       string
	}
	chips := make([]chip, 0, len(matches))
	for _, m := range matches {
		if insideCode(m[0], m[1]) {
			continue
		}
		// m: [matchStart, matchEnd, group1Start, group1End, group2Start, group2End]
		chips = append(chips, chip{
			start: m[0], end: m[1],
			kind: md[m[2]:m[3]],
			ref:  strings.TrimSpace(md[m[4]:m[5]]),
		})
	}
	if len(chips) == 0 {
		return md, nil
	}
	// Resolve each chip; degrade gracefully on errors.
	for i := range chips {
		chips[i].html = resolveChipHTML(ctx, pool, chips[i].kind, chips[i].ref, projectID)
	}
	// Stitch the output: walk md, splicing in chip HTML at each match.
	var b strings.Builder
	b.Grow(len(md) + 64*len(chips))
	cursor := 0
	for _, c := range chips {
		if c.start > cursor {
			b.WriteString(md[cursor:c.start])
		}
		b.WriteString(c.html)
		cursor = c.end
	}
	if cursor < len(md) {
		b.WriteString(md[cursor:])
	}
	return b.String(), nil
}

// resolveChipHTML produces the inline HTML for one chip. The result
// is intentionally compact (no surrounding paragraph, no newlines) so
// goldmark inlines it cleanly inside paragraphs, lists and table
// cells.
func resolveChipHTML(ctx context.Context, pool *pgxpool.Pool, kind, ref string, projectID *uuid.UUID) string {
	// Missing project context → inert span so the typo doesn't vanish.
	if pool == nil || projectID == nil {
		return missingChip(kind, ref, "no project context")
	}
	switch kind {
	case "task":
		return resolveTaskChip(ctx, pool, ref, *projectID)
	case "doc":
		return resolveDocChip(ctx, pool, ref, *projectID)
	case "arch":
		return resolveArchChip(ctx, pool, ref, *projectID)
	}
	return missingChip(kind, ref, "unknown kind")
}

// missingChip is the fallback rendering when a chip cannot be
// resolved. It's still a <span> so the sanitizer keeps it; the
// "chip-missing" class lets the prose CSS render it muted with a
// strike-like treatment.
func missingChip(kind, ref, reason string) string {
	return fmt.Sprintf(
		`<span class="chip chip-missing" title="%s">[%s:%s]</span>`,
		escapeAttr(reason), escapeText(kind), escapeText(ref),
	)
}

// chipLink emits a resolved chip as an anchor with semantic data-*
// attributes the client can hook later if needed.
func chipLink(kind, href, ref, label string, stateClass string) string {
	cls := "chip chip-" + kind
	if stateClass != "" {
		cls += " " + stateClass
	}
	return fmt.Sprintf(
		`<a class="%s" href="%s" data-kind="%s" data-ref="%s">%s</a>`,
		escapeAttr(cls), escapeAttr(href), escapeAttr(kind), escapeAttr(ref), escapeText(label),
	)
}

// escapeText is a tiny HTML text-content escaper. We deliberately
// don't pull in html.EscapeString to keep this file dependency-free,
// and the input charset is restricted to slug/path/id-like strings.
func escapeText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// escapeAttr extends escapeText with quote escaping.
func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// pathEscape wraps url.PathEscape but is also reused for doc paths
// inside chip hrefs.
func pathEscape(s string) string { return url.PathEscape(s) }
