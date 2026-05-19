package docs

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// SplitFrontmatter parses an incoming markdown document. It expects the
// frontmatter (if any) to be a YAML block delimited by `---` on its
// own line at the very top of the file. It returns the parsed
// frontmatter as a Go map and the body that follows.
//
// If no frontmatter is present, frontmatter is nil and body == md.
func SplitFrontmatter(md string) (frontmatter map[string]any, body string, err error) {
	const delim = "---"
	trimmed := strings.TrimLeft(md, "\r\n\t ")
	if !strings.HasPrefix(trimmed, delim) {
		return nil, md, nil
	}
	rest := trimmed[len(delim):]
	if !strings.HasPrefix(rest, "\n") && !strings.HasPrefix(rest, "\r\n") {
		return nil, md, nil
	}
	// Locate the closing delimiter line.
	closing := findClosingDelimiter(rest)
	if closing < 0 {
		// Unterminated frontmatter — leave document as-is.
		return nil, md, nil
	}
	rawYAML := rest[:closing]
	bodyStart := closing + len(delim)
	// Skip any trailing newline after the closing delimiter.
	for bodyStart < len(rest) && (rest[bodyStart] == '\n' || rest[bodyStart] == '\r') {
		bodyStart++
	}
	body = rest[bodyStart:]

	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(rawYAML), &fm); err != nil {
		return nil, md, err
	}
	return fm, body, nil
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
