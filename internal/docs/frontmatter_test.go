package docs

import (
	"strings"
	"testing"
)

func TestSplitFrontmatter_None(t *testing.T) {
	in := "# Hello\n\nNo frontmatter here."
	fm, body, err := SplitFrontmatter(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	if body != in {
		t.Errorf("body changed unexpectedly")
	}
}

func TestSplitFrontmatter_Parsed(t *testing.T) {
	in := "---\ntitle: My Title\nkind: skill\ntags: [a, b]\n---\n# Hello\n\nBody."
	fm, body, err := SplitFrontmatter(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if TitleFromFrontmatter(fm) != "My Title" {
		t.Errorf("title not parsed: %q", TitleFromFrontmatter(fm))
	}
	if KindFromFrontmatter(fm) != KindSkill {
		t.Errorf("kind not parsed: %q", KindFromFrontmatter(fm))
	}
	if !strings.HasPrefix(body, "# Hello") {
		t.Errorf("body wrong: %q", body)
	}
}

func TestSplitFrontmatter_Unterminated(t *testing.T) {
	in := "---\ntitle: oops\nbody never closes"
	fm, body, err := SplitFrontmatter(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil for unterminated, got %v", fm)
	}
	if body != in {
		t.Errorf("body changed: %q", body)
	}
}

func TestSplitFrontmatter_InvalidYAML(t *testing.T) {
	in := "---\ntitle: : :\n---\nbody"
	_, _, err := SplitFrontmatter(in)
	if err == nil {
		t.Fatal("expected yaml error, got nil")
	}
}

func TestBody(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"none", "# Hi\n\ntext\n", "# Hi\n\ntext\n"},
		{"block", "---\ntitle: T\n---\n\n# Hi\n", "# Hi\n"},
		{"leading blank lines", "\n\n---\ntitle: T\n---\nbody", "body"},
		{"crlf", "---\r\ntitle: T\r\n---\r\n\r\nbody", "body"},
		{"unterminated", "---\ntitle: T\nbody", "---\ntitle: T\nbody"},
		{"only frontmatter", "---\ntitle: T\n---\n", ""},
		// Body does not validate YAML; it only locates the block.
		{"invalid yaml", "---\ntitle: : :\n---\nbody", "body"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Body(c.in); got != c.want {
				t.Errorf("Body(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// BodyOffset feeds Postgres substr(), which counts characters. A
// multi-byte character inside the frontmatter must not shift it.
func TestBodyOffset_CountsCharacters(t *testing.T) {
	cases := []string{
		"# no frontmatter\n",
		"---\ntitle: plain\n---\n\nbody",
		"---\ntitle: Diseño ñandú 🚀\n---\n\nbody ñ",
		"\n---\ndescription: \"a: b\"\n---\nbody",
	}
	for _, md := range cases {
		off := BodyOffset(md)
		got := string([]rune(md)[off:])
		if got != Body(md) {
			t.Errorf("BodyOffset(%q) = %d selects %q, want %q", md, off, got, Body(md))
		}
	}
}
