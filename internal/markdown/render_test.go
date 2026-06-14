// Package markdown_test covers the markdown renderer end-to-end:
// CommonMark + GFM, the three Nottario chip kinds against real DB
// rows, missing-chip fallbacks, the code-fence skip (so the chip
// syntax can be documented inline), and the bluemonday sanitizer.
package markdown_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/markdown"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// ----- Pure renderer tests (no chips, no DB) -----

func TestRender_CommonMarkAndGFM(t *testing.T) {
	ctx := context.Background()
	// No pool, no project: chip resolution is bypassed.
	html, err := markdown.Render(ctx, nil, "# Hello\n\nA *paragraph* with **bold** and `code`.\n", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"<h1", "Hello", "<em>paragraph</em>", "<strong>bold</strong>", "<code>code</code>"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in: %s", want, html)
		}
	}
}

func TestRender_GFMTablesAndAutolinks(t *testing.T) {
	ctx := context.Background()
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n\nVisit https://example.com\n\n~~struck~~\n"
	html, err := markdown.Render(ctx, nil, src, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"<table>", "<th>a</th>", "<td>1</td>",
		`href="https://example.com"`,
		"<del>struck</del>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in: %s", want, html)
		}
	}
}

func TestRender_NoProjectContextChip(t *testing.T) {
	ctx := context.Background()
	// pool nil → every chip renders as "no project context" missing chip.
	html, err := markdown.Render(ctx, nil, "see [[task:abc12345]]", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(html, `class="chip chip-missing"`) {
		t.Errorf("expected chip-missing class, got: %s", html)
	}
	if !strings.Contains(html, "no project context") {
		t.Errorf("expected hint title, got: %s", html)
	}
}

func TestRender_ChipInsideCodeFenceIsLiteral(t *testing.T) {
	ctx := context.Background()
	src := "Inline `[[task:N]]` and a fenced block:\n```\n[[task:M]]\n```\n"
	html, err := markdown.Render(ctx, nil, src, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Both occurrences must reach the renderer verbatim — the literal
	// brackets and colon survive into the HTML, no chip span emitted.
	if strings.Contains(html, "chip-missing") {
		t.Errorf("chip-missing leaked into code region: %s", html)
	}
	if !strings.Contains(html, "[[task:N]]") || !strings.Contains(html, "[[task:M]]") {
		t.Errorf("expected literal chip syntax to survive code regions, got: %s", html)
	}
}

func TestRender_SanitizerStripsDangerousPayload(t *testing.T) {
	ctx := context.Background()
	src := `<script>alert(1)</script>` +
		`<a href="javascript:alert(1)">x</a>` +
		`<img src=x onerror="alert(1)">` +
		`<span class="chip chip-task">kept-class</span>` +
		"\n\n```go\npackage x\n```\n"
	html, err := markdown.Render(ctx, nil, src, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, bad := range []string{"<script", "javascript:", "onerror"} {
		if strings.Contains(html, bad) {
			t.Errorf("sanitizer leaked %q: %s", bad, html)
		}
	}
	// The class-allowed chip-task survives on <span> (the policy
	// whitelists chip classes on span / code / a only).
	if !strings.Contains(html, `<span class="chip chip-task">`) {
		t.Errorf("chip-task class stripped on span: %s", html)
	}
	// Goldmark's language-* class on code fences also survives.
	if !strings.Contains(html, `class="language-go"`) {
		t.Errorf("language-go class stripped: %s", html)
	}
}

// ----- Chip resolution against a real DB -----

func seedFixture(t *testing.T) (context.Context, *fixture, func()) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	u, _, err := identity.UpsertFromGithub(ctx, pool, 5000, "md", "MD", "")
	if err != nil {
		cancel()
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "MD", "", "", "", u.ID)
	if err != nil {
		cancel()
		t.Fatalf("project: %v", err)
	}
	return ctx, &fixture{
		pool:      pool,
		userID:    u.ID,
		projectID: p.ID,
	}, cancel
}

type fixture struct {
	pool      *pgxpool.Pool
	userID    uuid.UUID
	projectID uuid.UUID
}

func TestRender_TaskChipResolves(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	roles, _ := identity.ListRoles(ctx, fx.pool, fx.projectID)
	if len(roles) == 0 {
		t.Fatal("no roles seeded")
	}
	tk, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "Wire the thing",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	short := tk.ID.String()[:8]

	src := "See [[task:" + short + "]] for context."
	html, err := markdown.Render(ctx, fx.pool, src, &fx.projectID)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		`class="chip chip-task chip-state-todo"`,
		`#` + short, `Wire the thing`,
		`/projects/` + fx.projectID.String() + `/board/kanban#task=`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("task chip missing %q in: %s", want, html)
		}
	}
}

func TestRender_DocChipResolves(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	zero := 0
	path := "projects/" + fx.projectID.String() + "/context/glossary.md"
	_, err := docs.Write(ctx, fx.pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &fx.projectID, Path: path,
		ContentMD: "# Glossary\n", Message: "init", ExpectedVersion: &zero,
	}, docs.Authorship{UserID: &fx.userID})
	if err != nil {
		t.Fatalf("docs.Write: %v", err)
	}

	src := "Reference: [[doc:" + path + "]]"
	html, err := markdown.Render(ctx, fx.pool, src, &fx.projectID)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		`class="chip chip-doc"`,
		`Glossary`,
		`/projects/` + fx.projectID.String() + `/docs#path=`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("doc chip missing %q in: %s", want, html)
		}
	}
}

func TestRender_ArchChipResolves(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	if _, err := arch.UpsertNode(ctx, fx.pool, fx.projectID, arch.UpsertParams{
		Slug: "sys", Kind: "system", Name: "System X",
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	html, err := markdown.Render(ctx, fx.pool, "lives in [[arch:sys]]", &fx.projectID)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		`class="chip chip-arch"`,
		`System X`,
		`/projects/` + fx.projectID.String() + `/arch/diagram#slug=`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("arch chip missing %q in: %s", want, html)
		}
	}
}

func TestRender_MissingChipsFallToMissingClass(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	src := "[[task:00000000]]\n\n[[doc:does/not/exist.md]]\n\n[[arch:ghost]]\n\n[[unknown:huh]]"
	html, err := markdown.Render(ctx, fx.pool, src, &fx.projectID)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Three chip-missing spans — only the three kinds matched by the
	// chip regex (`(task|doc|arch)`) are substituted; the unknown
	// kind reaches goldmark verbatim and renders as plain text.
	if got := strings.Count(html, "chip-missing"); got != 3 {
		t.Errorf("expected 3 chip-missing, got %d in: %s", got, html)
	}
	for _, want := range []string{
		"no task with that id prefix",
		"no document with that path",
		"no architecture node",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing reason %q in: %s", want, html)
		}
	}
	// The unknown-kind chip reaches the output as literal text since
	// chipRe never matched it.
	if !strings.Contains(html, "[[unknown:huh]]") {
		t.Errorf("expected literal [[unknown:huh]] in: %s", html)
	}
}
