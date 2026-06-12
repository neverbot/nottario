package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestWontDo_Transitions covers the lifecycle rules introduced with
// the wont_do state: forward transitions from todo/doing, the re-open
// path, and the two refused cross-terminal transitions.
func TestWontDo_Transitions(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9101, "wontdo", "WontDo", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "WontDoProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			Type:         tasks.TypeTask,
			Title:        title,
			TargetRoleID: &roleID,
		}, by)
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}

	t.Run("todo -> wont_do sets actual_end", func(t *testing.T) {
		tk := mk("cancelled from todo")
		got, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo)
		if err != nil {
			t.Fatalf("SetState wont_do: %v", err)
		}
		if got.State != tasks.StateWontDo {
			t.Fatalf("state = %s, want wont_do", got.State)
		}
		if got.ActualEnd == nil {
			t.Fatalf("expected actual_end set, got nil")
		}
		if got.ActualStart != nil {
			t.Fatalf("expected actual_start nil (never started), got %v", got.ActualStart)
		}
	})

	t.Run("doing -> wont_do preserves actual_start", func(t *testing.T) {
		tk := mk("cancelled mid-flight")
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDoing); err != nil {
			t.Fatalf("SetState doing: %v", err)
		}
		started, _ := tasks.Get(ctx, pool, tk.ID)
		if started.ActualStart == nil {
			t.Fatalf("expected actual_start set after doing")
		}
		got, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo)
		if err != nil {
			t.Fatalf("SetState wont_do: %v", err)
		}
		if got.State != tasks.StateWontDo {
			t.Fatalf("state = %s, want wont_do", got.State)
		}
		if got.ActualStart == nil || !got.ActualStart.Equal(*started.ActualStart) {
			t.Fatalf("actual_start changed: was %v, now %v", started.ActualStart, got.ActualStart)
		}
		if got.ActualEnd == nil {
			t.Fatalf("expected actual_end set")
		}
	})

	t.Run("wont_do -> todo clears timestamps (re-open)", func(t *testing.T) {
		tk := mk("reopened")
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo); err != nil {
			t.Fatalf("SetState wont_do: %v", err)
		}
		got, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateTodo)
		if err != nil {
			t.Fatalf("SetState todo (re-open): %v", err)
		}
		if got.State != tasks.StateTodo {
			t.Fatalf("state = %s, want todo", got.State)
		}
		if got.ActualEnd != nil {
			t.Fatalf("expected actual_end cleared on re-open, got %v", got.ActualEnd)
		}
	})

	t.Run("done -> wont_do is refused", func(t *testing.T) {
		tk := mk("shipped")
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDone); err != nil {
			t.Fatalf("SetState done: %v", err)
		}
		_, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo)
		var terr *tasks.ErrInvalidStateTransition
		if !errors.As(err, &terr) {
			t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
		}
		if terr.From != tasks.StateDone || terr.To != tasks.StateWontDo {
			t.Fatalf("expected done -> wont_do error, got %s -> %s", terr.From, terr.To)
		}
		got, _ := tasks.Get(ctx, pool, tk.ID)
		if got.State != tasks.StateDone {
			t.Fatalf("expected state to stay done after refused transition, got %s", got.State)
		}
	})

	t.Run("wont_do -> done is refused", func(t *testing.T) {
		tk := mk("cancelled then promoted")
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo); err != nil {
			t.Fatalf("SetState wont_do: %v", err)
		}
		_, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDone)
		var terr *tasks.ErrInvalidStateTransition
		if !errors.As(err, &terr) {
			t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
		}
		if terr.From != tasks.StateWontDo || terr.To != tasks.StateDone {
			t.Fatalf("expected wont_do -> done error, got %s -> %s", terr.From, terr.To)
		}
	})
}

// TestWontDo_DependencyPrecondition covers that a wont_do upstream
// satisfies a downstream's precondition — closing B (which depends on
// A) as done is allowed once A is wont_do, just like when A is done.
func TestWontDo_DependencyPrecondition(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9102, "deps", "Deps", "")
	p, _ := identity.CreateProject(ctx, pool, "DepsProj", "", "", "", u.ID, nil)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		tk, _ := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			Type:         tasks.TypeTask,
			Title:        title,
			TargetRoleID: &roleID,
		}, by)
		return tk
	}

	a := mk("A (upstream)")
	b := mk("B (depends on A)")
	if err := tasks.AddDependency(ctx, pool, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	// Cannot close B yet — A is still todo.
	if _, err := tasks.SetState(ctx, pool, b.ID, tasks.StateDone); err == nil {
		t.Fatalf("expected unresolved-precondition error, got nil")
	}

	// Cancel A. B should now be closable as done because wont_do
	// upstreams count as closed for the precondition check.
	if _, err := tasks.SetState(ctx, pool, a.ID, tasks.StateWontDo); err != nil {
		t.Fatalf("SetState A wont_do: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, b.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState B done after upstream wont_do: %v", err)
	}
}

// TestWontDo_FeatureRollup covers that a feature parent rolls up to
// done when its children are a mix of done + wont_do.
func TestWontDo_FeatureRollup(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9103, "roll", "Roll", "")
	p, _ := identity.CreateProject(ctx, pool, "RollProj", "", "", "", u.ID, nil)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	parent, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		Type:         tasks.TypeFeature,
		Title:        "feature with mixed closures",
		TargetRoleID: &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	c1, _ := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		ParentTaskID: &parent.ID,
		Type:         tasks.TypeTask,
		Title:        "shipped child",
		TargetRoleID: &roleID,
	}, by)
	c2, _ := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		ParentTaskID: &parent.ID,
		Type:         tasks.TypeTask,
		Title:        "cancelled child",
		TargetRoleID: &roleID,
	}, by)

	if _, err := tasks.SetState(ctx, pool, c1.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState c1 done: %v", err)
	}
	// Parent should still be open: c2 is still todo.
	got, _ := tasks.Get(ctx, pool, parent.ID)
	if got.State == tasks.StateDone {
		t.Fatalf("parent rolled up too early — c2 still open")
	}
	if _, err := tasks.SetState(ctx, pool, c2.ID, tasks.StateWontDo); err != nil {
		t.Fatalf("SetState c2 wont_do: %v", err)
	}
	got, _ = tasks.Get(ctx, pool, parent.ID)
	if got.State != tasks.StateDone {
		t.Fatalf("expected parent to roll up to done with done+wont_do children, got %s", got.State)
	}
}

// TestWontDo_ClaimNextSkips covers that ClaimNext / next preview
// never surface a wont_do task — they only consider todo.
func TestWontDo_ClaimNextSkips(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9104, "skipper", "Skipper", "")
	p, _ := identity.CreateProject(ctx, pool, "SkipProj", "", "", "", u.ID, nil)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	// Only candidate is wont_do — next must return nil.
	tk, _ := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		Type:         tasks.TypeTask,
		Title:        "cancelled candidate",
		TargetRoleID: &roleID,
	}, by)
	if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateWontDo); err != nil {
		t.Fatalf("SetState wont_do: %v", err)
	}
	_, err := tasks.Next(ctx, pool, tasks.NextFilter{ProjectID: p.ID})
	if !errors.Is(err, tasks.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wont_do-only backlog, got %v", err)
	}
}
