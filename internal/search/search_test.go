// Package search_test exercises the unified FTS query end-to-end:
// each indexed domain produces hits with the right payload shape,
// kind filters narrow the result set, and the stemming-regression
// case is encoded so the day we fix the `simple` config the assertion
// flips visibly.
package search_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/search"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

type fixture struct {
	pool      *pgxpool.Pool
	userID    uuid.UUID
	projectID uuid.UUID
}

func seedFixture(t *testing.T) (context.Context, *fixture, func()) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	u, _, err := identity.UpsertFromGithub(ctx, pool, 8000, "s", "S", "")
	if err != nil {
		cancel()
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "SP", "", "", "", u.ID)
	if err != nil {
		cancel()
		t.Fatalf("project: %v", err)
	}
	return ctx, &fixture{pool: pool, userID: u.ID, projectID: p.ID}, cancel
}

func TestSearch_Validation(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	if _, err := search.Search(ctx, pool, "x", search.Filter{}); err == nil {
		t.Error("missing project_id should error")
	}
	if _, err := search.Search(ctx, pool, "  ", search.Filter{ProjectID: uuid.New()}); err == nil {
		t.Error("empty query should error")
	}
}

func TestSearch_HitsAcrossAllThreeDomains(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	// Seed a task with the word "alpaca" in title.
	roles, _ := identity.ListRoles(ctx, fx.pool, fx.projectID)
	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "Buy an alpaca",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("Create task: %v", err)
	}

	// Seed a doc that mentions "alpaca".
	zero := 0
	docPath := "projects/" + fx.projectID.String() + "/notes/zoo.md"
	if _, err := docs.Write(ctx, fx.pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &fx.projectID, Path: docPath,
		ContentMD: "# Zoo\n\nalpaca is fluffy.\n",
		Message:   "init", ExpectedVersion: &zero,
	}, docs.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("docs.Write: %v", err)
	}

	// Seed an arch node named "alpaca".
	if _, err := arch.UpsertNode(ctx, fx.pool, fx.projectID, arch.Authorship{UserID: fx.userID}, arch.UpsertParams{
		Slug: "alpaca-svc", Kind: "service", Name: "alpaca service",
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	hits, err := search.Search(ctx, fx.pool, "alpaca", search.Filter{ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	seen := map[search.Kind]bool{}
	for _, h := range hits {
		seen[h.Kind] = true
		switch h.Kind {
		case search.KindTask:
			if h.Title == "" || h.TaskID == "" || h.TaskState == "" {
				t.Errorf("task hit missing fields: %+v", h)
			}
		case search.KindDocument:
			if h.DocPath == "" {
				t.Errorf("doc hit missing path: %+v", h)
			}
		case search.KindArchNode:
			if h.NodeSlug == "" || h.NodeKind == "" {
				t.Errorf("arch hit missing slug/kind: %+v", h)
			}
		}
	}
	for _, want := range []search.Kind{search.KindTask, search.KindDocument, search.KindArchNode} {
		if !seen[want] {
			t.Errorf("no hit for kind %q: %+v", want, hits)
		}
	}
}

func TestSearch_KindFilterNarrowsResults(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	roles, _ := identity.ListRoles(ctx, fx.pool, fx.projectID)
	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "alpaca duty",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := arch.UpsertNode(ctx, fx.pool, fx.projectID, arch.Authorship{UserID: fx.userID}, arch.UpsertParams{
		Slug: "alpaca-arch", Kind: "service", Name: "alpaca arch",
	}); err != nil {
		t.Fatalf("arch: %v", err)
	}

	taskHits, err := search.Search(ctx, fx.pool, "alpaca", search.Filter{
		ProjectID: fx.projectID, Kinds: []search.Kind{search.KindTask},
	})
	if err != nil {
		t.Fatalf("Search task: %v", err)
	}
	for _, h := range taskHits {
		if h.Kind != search.KindTask {
			t.Errorf("expected only task hits, got %+v", h)
		}
	}
	if len(taskHits) == 0 {
		t.Error("expected at least one task hit")
	}

	archHits, err := search.Search(ctx, fx.pool, "alpaca", search.Filter{
		ProjectID: fx.projectID, Kinds: []search.Kind{search.KindArchNode},
	})
	if err != nil {
		t.Fatalf("Search arch: %v", err)
	}
	for _, h := range archHits {
		if h.Kind != search.KindArchNode {
			t.Errorf("expected only arch hits, got %+v", h)
		}
	}
	if len(archHits) == 0 {
		t.Error("expected at least one arch hit")
	}
}

func TestSearch_StemmingRegression(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	roles, _ := identity.ListRoles(ctx, fx.pool, fx.projectID)
	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "tasks pipeline",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("task: %v", err)
	}

	// After be13415d the search vector unions simple+english+spanish
	// configs, so "task" stems via english and matches stored
	// "tasks". Also assert the spanish stemmer works.
	hits, err := search.Search(ctx, fx.pool, "task", search.Filter{
		ProjectID: fx.projectID, Kinds: []search.Kind{search.KindTask},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Errorf("expected 'task' to stem-match stored 'tasks', got 0 hits")
	}

	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "tareas pendientes",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("spanish task: %v", err)
	}
	spanish, err := search.Search(ctx, fx.pool, "tarea", search.Filter{
		ProjectID: fx.projectID, Kinds: []search.Kind{search.KindTask},
	})
	if err != nil {
		t.Fatalf("Search spanish: %v", err)
	}
	if len(spanish) == 0 {
		t.Errorf("expected 'tarea' to stem-match stored 'tareas' (spanish), got 0 hits")
	}
}

func TestSearch_ScopesToProject(t *testing.T) {
	ctx, fx, cancel := seedFixture(t)
	defer cancel()

	// Second project with same content.
	other, err := identity.CreateProject(ctx, fx.pool, "Other", "", "", "", fx.userID)
	if err != nil {
		t.Fatalf("other project: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, fx.pool, fx.projectID)
	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: fx.projectID, Type: tasks.TypeTask, Title: "alpaca here",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("task own: %v", err)
	}
	otherRoles, _ := identity.ListRoles(ctx, fx.pool, other.ID)
	if _, err := tasks.Create(ctx, fx.pool, tasks.CreateParams{
		ProjectID: other.ID, Type: tasks.TypeTask, Title: "alpaca there",
		TargetRoleID: &otherRoles[0].ID,
	}, tasks.Authorship{UserID: &fx.userID}); err != nil {
		t.Fatalf("task other: %v", err)
	}

	hits, err := search.Search(ctx, fx.pool, "alpaca", search.Filter{ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range hits {
		if h.ProjectID != fx.projectID.String() {
			t.Errorf("leak: hit from another project: %+v", h)
		}
	}
}

// Sanity check that the function rejects misuse rather than panicking.
func TestSearch_NoPanicOnBadFilter(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	if _, err := search.Search(ctx, pool, "x", search.Filter{}); err == nil ||
		!errors.Is(err, errors.New("project_id is required")) {
		// errors.Is on a freshly-allocated errors.New only matches on
		// pointer identity, so we settle for "err is non-nil" plus
		// the message check the function uses verbatim.
		if err == nil || err.Error() != "project_id is required" {
			t.Errorf("expected project_id error, got %v", err)
		}
	}
}
