package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestListInconsistencies_DependentAlreadyDone(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9101, "incons", "Inc", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "IncProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID: p.ID, Type: tasks.TypeTask, Title: title, TargetRoleID: &roleID,
		}, by)
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}

	parent := mk("parent")
	child := mk("child")

	// child depends on parent. Force-close child to bypass the
	// precondition check is not possible — instead, declare the
	// dependency only AFTER child is done, which is itself the
	// inconsistency this check catches in real life (deps added
	// retroactively after the dependent shipped).
	if _, err := tasks.SetState(ctx, pool, child.ID, tasks.StateDoing); err != nil {
		t.Fatalf("SetState doing: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, child.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState done: %v", err)
	}
	if err := tasks.AddDependency(ctx, pool, child.ID, parent.ID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	items, err := tasks.ListInconsistencies(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListInconsistencies: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 inconsistency, got %d: %+v", len(items), items)
	}
	if items[0].TaskID != parent.ID {
		t.Fatalf("expected flag on parent %s, got %s", parent.ID, items[0].TaskID)
	}
	if items[0].Reason != tasks.ReasonDependentAlreadyDone {
		t.Fatalf("unexpected reason: %s", items[0].Reason)
	}
	ids, ok := items[0].Details["dependent_task_ids"].([]string)
	if !ok || len(ids) != 1 || ids[0] != child.ID.String() {
		t.Fatalf("expected dependent %s in details, got %+v", child.ID, items[0].Details)
	}

	// Closing the parent removes it from the report.
	if _, err := tasks.SetState(ctx, pool, parent.ID, tasks.StateDoing); err != nil {
		t.Fatalf("SetState parent doing: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, parent.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState parent done: %v", err)
	}
	items, err = tasks.ListInconsistencies(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListInconsistencies after close: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 inconsistencies after parent done, got %+v", items)
	}
}
