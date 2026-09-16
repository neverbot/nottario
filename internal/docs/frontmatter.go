package docs

import (
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Documents are stored exactly as written: frontmatter and body
// together, byte for byte. Frontmatter is only *read* on the way in,
// to extract kind, title and description. The systems that must not
// see it (the search index and the HTML renderer) skip it themselves
// with Body / BodyOffset; nothing ever strips it from storage.

// locateFrontmatter finds a YAML frontmatter block delimited by `---`
// lines at the very top of md. It returns the raw YAML and the byte
// index in md where the body starts (past the closing delimiter and
// any newlines after it). ok is false when md has no complete block.
func locateFrontmatter(md string) (rawYAML string, bodyStart int, ok bool) {
	const delim = "---"
	trimmed := strings.TrimLeft(md, "\r\n\t ")
	if !strings.HasPrefix(trimmed, delim) {
		return "", 0, false
	}
	rest := trimmed[len(delim):]
	if !strings.HasPrefix(rest, "\n") && !strings.HasPrefix(rest, "\r\n") {
		return "", 0, false
	}
	closing := findClosingDelimiter(rest)
	if closing < 0 {
		// Unterminated frontmatter: the whole document is body.
		return "", 0, false
	}
	start := closing + len(delim)
	for start < len(rest) && (rest[start] == '\n' || rest[start] == '\r') {
		start++
	}
	restOffset := len(md) - len(rest)
	return rest[:closing], restOffset + start, true
}

// SplitFrontmatter parses the frontmatter of a markdown document and
// returns it together with the body that follows. It does not change
// what is stored: callers keep md whole.
//
// If no frontmatter is present, frontmatter is nil and body == md. A
// block that is present but is not valid YAML is an error.
func SplitFrontmatter(md string) (frontmatter map[string]any, body string, err error) {
	rawYAML, start, ok := locateFrontmatter(md)
	if !ok {
		return nil, md, nil
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(rawYAML), &fm); err != nil {
		return nil, md, err
	}
	return fm, md[start:], nil
}

// Body returns md without its frontmatter block, for renderers that
// display the document. It does not validate the YAML: a stored
// document already passed that check when it was written.
func Body(md string) string {
	if _, start, ok := locateFrontmatter(md); ok {
		return md[start:]
	}
	return md
}

// BodyOffset is where the body starts, counted in characters rather
// than bytes, because it feeds Postgres substr() in the search index
// and substr counts characters. Zero when there is no frontmatter.
func BodyOffset(md string) int {
	if _, start, ok := locateFrontmatter(md); ok {
		return utf8.RuneCountInString(md[:start])
	}
	return 0
}

// findClosingDelimiter returns the byte index in s of a line that is
// exactly `---` (after stripping carriage returns). It returns -1 if
// no such line is found.
func findClosingDelimiter(s string) int {
	scanFrom := 0
	for {
		idx := strings.Index(s[scanFrom:], "\n---")
		if idx < 0 {
			return -1
		}
		pos := scanFrom + idx
		after := pos + len("\n---")
		if after == len(s) || s[after] == '\n' || s[after] == '\r' {
			return after - len("---")
		}
		scanFrom = after
	}
}

// TitleFromFrontmatter pulls a "title" string from a parsed
// frontmatter map. Returns "" if absent or wrong type.
func TitleFromFrontmatter(fm map[string]any) string {
	if v, ok := fm["title"].(string); ok {
		return v
	}
	return ""
}

// DescriptionFromFrontmatter pulls a "description" string from a parsed
// frontmatter map.
func DescriptionFromFrontmatter(fm map[string]any) string {
	if v, ok := fm["description"].(string); ok {
		return v
	}
	return ""
}

// KindFromFrontmatter returns the Kind named in frontmatter, or "" if
// none is given. Validation happens elsewhere.
func KindFromFrontmatter(fm map[string]any) Kind {
	if v, ok := fm["kind"].(string); ok {
		return Kind(v)
	}
	return ""
}
