package docs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestDocs_WriteReadHistoryConflict(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 7001, "writer", "Writer", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "DocProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	by := docs.Authorship{UserID: &u.ID}
	path := "projects/" + p.ID.String() + "/context/glossary.md"

	zero := 0
	d, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD:       "# Glossary\n\nFirst version.\n",
		Message:         "create",
		ExpectedVersion: &zero,
	}, by)
	if err != nil {
		t.Fatalf("Write create: %v", err)
	}
	if d.CurrentVersion != 1 {
		t.Fatalf("expected version 1 on create, got %d", d.CurrentVersion)
	}

	got, err := docs.Read(ctx, pool, docs.ScopeProject, &p.ID, path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Title != "Glossary" || got.CurrentVersion != 1 {
		t.Fatalf("unexpected read: %+v", got)
	}

	v1 := 1
	d2, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD:       "# Glossary\n\nSecond version.\n",
		Message:         "update",
		ExpectedVersion: &v1,
	}, by)
	if err != nil {
		t.Fatalf("Write update: %v", err)
	}
	if d2.CurrentVersion != 2 {
		t.Fatalf("expected version 2 after update, got %d", d2.CurrentVersion)
	}

	stale := 1
	_, err = docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD:       "# Glossary\n\nThird, but stale version.\n",
		Message:         "should conflict",
		ExpectedVersion: &stale,
	}, by)
	var vc *docs.VersionConflictError
	if !errors.As(err, &vc) {
		t.Fatalf("expected VersionConflictError, got %v", err)
	}
	if vc.CurrentVersion != 2 {
		t.Fatalf("conflict reports wrong current_version: %d", vc.CurrentVersion)
	}
	if !errors.Is(err, docs.ErrVersionConflict) {
		t.Fatalf("conflict no longer satisfies errors.Is(ErrVersionConflict)")
	}

	hist, err := docs.History(ctx, pool, d2.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 || hist[0].Version != 2 || hist[1].Version != 1 {
		t.Fatalf("unexpected history: %+v", hist)
	}

	v2 := 2
	if err := docs.DeleteWithParams(ctx, pool, docs.DeleteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		Message:         "drop",
		ExpectedVersion: &v2,
	}, by); err != nil {
		t.Fatalf("DeleteWithParams: %v", err)
	}
	if _, err := docs.Read(ctx, pool, docs.ScopeProject, &p.ID, path); !errors.Is(err, docs.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
