package cycles_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestEndCycle_BasicMove(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 7101, "cy-basic", "Cy Basic", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "CycleBasic", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v len=%d", err, len(roles))
	}
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

	tTodo := mk("stays-todo")
	tDoing := mk("goes-doing")
	tDone := mk("goes-done")

	if _, err := tasks.SetState(ctx, pool, tDoing.ID, tasks.StateDoing); err != nil {
		t.Fatalf("SetState doing: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, tDone.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState done: %v", err)
	}

	res, err := cycles.EndCycle(ctx, pool, cycles.EndCycleParams{ProjectID: p.ID}, cycles.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatalf("EndCycle: %v", err)
	}
	if res.Closed.ClosedAt == nil {
		t.Errorf("expected res.Closed.ClosedAt != nil, got nil")
	}
	if res.Closed.Name != "sprint-1" {
		t.Errorf("expected closed.Name=sprint-1, got %q", res.Closed.Name)
	}
	if res.Next.Position != 2 {
		t.Errorf("expected next.Position=2, got %d", res.Next.Position)
	}
	if res.Next.Name != "sprint-2" {
		t.Errorf("expected next.Name=sprint-2, got %q", res.Next.Name)
	}

	got, err := tasks.Get(ctx, pool, tTodo.ID)
	if err != nil {
		t.Fatalf("Get tTodo: %v", err)
	}
	if got.CycleID != res.Next.ID {
		t.Errorf("todo task: expected CycleID=%s (next), got %s", res.Next.ID, got.CycleID)
	}
	got, err = tasks.Get(ctx, pool, tDoing.ID)
	if err != nil {
		t.Fatalf("Get tDoing: %v", err)
	}
	if got.CycleID != res.Next.ID {
		t.Errorf("doing task: expected CycleID=%s (next), got %s", res.Next.ID, got.CycleID)
	}
	got, err = tasks.Get(ctx, pool, tDone.ID)
	if err != nil {
		t.Fatalf("Get tDone: %v", err)
	}
	if got.CycleID != res.Closed.ID {
		t.Errorf("done task: expected CycleID=%s (closed), got %s", res.Closed.ID, got.CycleID)
	}
}

func TestEndCycle_CascadesPartialFeature(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 7102, "cy-partial", "Cy Partial", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "CyclePartial", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v len=%d", err, len(roles))
	}
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	feature, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		Type:         tasks.TypeFeature,
		Title:        "F",
		TargetRoleID: &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create feature: %v", err)
	}

	mkChild := func(title string) *tasks.Task {
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			ParentTaskID: &feature.ID,
			Type:         tasks.TypeTask,
			Title:        title,
			TargetRoleID: &roleID,
		}, by)
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}
	c1 := mkChild("c1")
	c2 := mkChild("c2")
	c3 := mkChild("c3")

	if _, err := tasks.SetState(ctx, pool, c1.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState c1 done: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, c2.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState c2 done: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, c3.ID, tasks.StateDoing); err != nil {
		t.Fatalf("SetState c3 doing: %v", err)
	}

	// Sanity: feature must NOT have auto-rolled to done.
	fGot, err := tasks.Get(ctx, pool, feature.ID)
	if err != nil {
		t.Fatalf("Get feature pre-end: %v", err)
	}
	if fGot.State == tasks.StateDone {
		t.Fatalf("feature unexpectedly rolled up to done while c3 is still doing")
	}

	res, err := cycles.EndCycle(ctx, pool, cycles.EndCycleParams{ProjectID: p.ID}, cycles.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatalf("EndCycle: %v", err)
	}

	for _, tk := range []struct {
		name string
		id   uuid.UUID
	}{
		{"F", feature.ID},
		{"c1", c1.ID},
		{"c2", c2.ID},
		{"c3", c3.ID},
	} {
		got, err := tasks.Get(ctx, pool, tk.id)
		if err != nil {
			t.Fatalf("Get %s: %v", tk.name, err)
		}
		if got.CycleID != res.Next.ID {
			t.Errorf("%s: expected CycleID=%s (next), got %s", tk.name, res.Next.ID, got.CycleID)
		}
	}
}

func TestEndCycle_LeavesFullyDoneFeatureAlone(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 7103, "cy-fulldone", "Cy FullDone", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "CycleFullDone", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v len=%d", err, len(roles))
	}
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	feature, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:    p.ID,
		Type:         tasks.TypeFeature,
		Title:        "F",
		TargetRoleID: &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create feature: %v", err)
	}
	mkChild := func(title string) *tasks.Task {
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			ParentTaskID: &feature.ID,
			Type:         tasks.TypeTask,
			Title:        title,
			TargetRoleID: &roleID,
		}, by)
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}
	c1 := mkChild("c1")
	c2 := mkChild("c2")

	if _, err := tasks.SetState(ctx, pool, c1.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState c1 done: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, c2.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState c2 done: %v", err)
	}

	// The rollUpParentDone logic inside SetState should have promoted
	// the feature to done now that every child is closed. If for any
	// reason it didn't (different rollup path), force it manually so
	// the test is deterministic.
	fGot, err := tasks.Get(ctx, pool, feature.ID)
	if err != nil {
		t.Fatalf("Get feature post-children: %v", err)
	}
	if fGot.State != tasks.StateDone {
		if _, err := tasks.SetState(ctx, pool, feature.ID, tasks.StateDoing); err != nil {
			t.Fatalf("force feature doing: %v", err)
		}
		if _, err := tasks.SetState(ctx, pool, feature.ID, tasks.StateDone); err != nil {
			t.Fatalf("force feature done: %v", err)
		}
	}

	res, err := cycles.EndCycle(ctx, pool, cycles.EndCycleParams{ProjectID: p.ID}, cycles.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatalf("EndCycle: %v", err)
	}

	for _, tk := range []struct {
		name string
		id   uuid.UUID
	}{
		{"F", feature.ID},
		{"c1", c1.ID},
		{"c2", c2.ID},
	} {
		got, err := tasks.Get(ctx, pool, tk.id)
		if err != nil {
			t.Fatalf("Get %s: %v", tk.name, err)
		}
		if got.CycleID != res.Closed.ID {
			t.Errorf("%s: expected CycleID=%s (closed, unmoved), got %s", tk.name, res.Closed.ID, got.CycleID)
		}
	}

	// The new cycle should have no tasks at all.
	nextID := res.Next.ID
	list, err := tasks.List(ctx, pool, tasks.ListFilter{
		ProjectID:       p.ID,
		CycleID:         &nextID,
		IncludeChildren: true,
	})
	if err != nil {
		t.Fatalf("tasks.List(next cycle): %v", err)
	}
	if len(list) != 0 {
		titles := make([]string, 0, len(list))
		for _, tk := range list {
			titles = append(titles, tk.Title)
		}
		t.Errorf("expected next cycle to be empty, got %d tasks: %v", len(list), titles)
	}
}
