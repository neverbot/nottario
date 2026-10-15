package docs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestDocs_Move(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 7101, "mover", "Mover", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "MoveProj", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	by := docs.Authorship{UserID: &u.ID}
	write := func(path, body string) {
		t.Helper()
		zero := 0
		if _, err := docs.Write(ctx, pool, docs.WriteParams{
			Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
			ContentMD: body, ExpectedVersion: &zero,
		}, by); err != nil {
			t.Fatalf("Write %s: %v", path, err)
		}
	}
	move := func(path, newPath string, expected int) (*docs.Document, error) {
		return docs.Move(ctx, pool, docs.MoveParams{
			Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path, NewPath: newPath,
			ExpectedVersion: &expected,
		}, by)
	}

	write("notes/old.md", "# Plan\n\nBody.\n")
	archBy := arch.Authorship{UserID: u.ID}
	if _, err := arch.UpsertNode(ctx, pool, p.ID, archBy, arch.UpsertParams{Slug: "svc", Kind: "service", Name: "Svc"}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if err := arch.LinkDoc(ctx, pool, p.ID, archBy, "svc", "notes/old.md"); err != nil {
		t.Fatalf("LinkDoc: %v", err)
	}

	if _, err := move("notes/old.md", "notes/new.md", 7); !errors.Is(err, docs.ErrVersionConflict) {
		t.Fatalf("stale expected_version: want version conflict, got %v", err)
	}

	d, err := move("notes/old.md", "notes/new.md", 1)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if d.Path != "notes/new.md" || d.CurrentVersion != 2 || d.Title != "Plan" {
		t.Fatalf("moved doc: %+v", d)
	}
	if _, err := docs.Read(ctx, pool, docs.ScopeProject, &p.ID, "notes/old.md"); !errors.Is(err, docs.ErrNotFound) {
		t.Fatalf("old path should be gone, got %v", err)
	}
	hist, err := docs.History(ctx, pool, d.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 || hist[0].Message != "moved from notes/old.md" {
		t.Fatalf("history should keep v1 and add the move: %+v", hist)
	}
	links, err := arch.ListLinks(ctx, pool, p.ID, "svc")
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 1 || links[0].TargetID != "notes/new.md" {
		t.Fatalf("arch link should follow the move: %+v", links)
	}

	// The destination is taken by a live document.
	write("notes/other.md", "other\n")
	var taken *docs.PathTakenError
	if _, err := move("notes/new.md", "notes/other.md", 2); !errors.As(err, &taken) || taken.Deleted {
		t.Fatalf("live destination: want PathTakenError, got %v", err)
	}

	// The destination belongs to a deleted document.
	if err := docs.Delete(ctx, pool, docs.ScopeProject, &p.ID, "notes/other.md", "", by); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := move("notes/new.md", "notes/other.md", 2); !errors.As(err, &taken) || !taken.Deleted {
		t.Fatalf("deleted destination: want PathTakenError{Deleted}, got %v", err)
	}

	// A title that was only the path follows the path.
	write("bare.md", "no heading here\n")
	d, err = move("bare.md", "renamed.md", 1)
	if err != nil {
		t.Fatalf("Move bare: %v", err)
	}
	if d.Title != "renamed.md" {
		t.Fatalf("path-derived title should follow the move, got %q", d.Title)
	}

	if _, err := move("missing.md", "x.md", 1); !errors.Is(err, docs.ErrNotFound) {
		t.Fatalf("missing source: want ErrNotFound, got %v", err)
	}
}
