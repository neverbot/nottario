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
