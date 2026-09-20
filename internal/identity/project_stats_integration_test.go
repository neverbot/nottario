package identity_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// The card on the projects list has to say what the project's own
// Kanban shows for open work, feature parents included, and keep the
// running total of everything the project has closed.
func TestListProjects_StatsMatchTheBoard(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9301, "stats", "Stats", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "StatsProj", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	by := tasks.Authorship{UserID: &u.ID}

	mk := func(title string, kind tasks.Type, state tasks.State) uuid.UUID {
		t.Helper()
		task, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID: p.ID, Type: kind, Title: title,
		}, by)
		if err != nil {
			t.Fatalf("Create %s: %v", title, err)
		}
		if state != tasks.StateTodo {
			if _, err := tasks.SetState(ctx, pool, task.ID, state, &u.ID); err != nil {
				t.Fatalf("SetState %s: %v", title, err)
			}
		}
		return task.ID
	}

	mk("plain todo", tasks.TypeTask, tasks.StateTodo)
	mk("a bug", tasks.TypeBug, tasks.StateTodo)
	mk("a feature parent", tasks.TypeFeature, tasks.StateTodo)
	mk("in flight", tasks.TypeTask, tasks.StateDoing)
	mk("shipped", tasks.TypeTask, tasks.StateDone)

	stats := func() identity.ProjectStats {
		t.Helper()
		list, err := identity.ListProjects(ctx, pool, u.ID, false)
		if err != nil {
			t.Fatalf("ListProjects: %v", err)
		}
		for _, row := range list {
			if row.ID == p.ID {
				if row.Stats == nil {
					t.Fatal("project has no stats")
				}
				return *row.Stats
			}
		}
		t.Fatal("project missing from the listing")
		return identity.ProjectStats{}
	}

	got := stats()
	// Three todo cards on the board: the task, the bug and the feature.
	if got.TodoCount != 3 {
		t.Errorf("todo_count = %d, want 3 (feature parents are cards too)", got.TodoCount)
	}
	if got.DoingCount != 1 || got.DoneCount != 1 {
		t.Errorf("doing/done = %d/%d, want 1/1", got.DoingCount, got.DoneCount)
	}
	if got.LastActivityAt == nil {
		t.Error("last_activity_at not set")
	}

	// Ending the cycle carries every open task into the new one and
	// leaves the closed ones behind. The open counts must survive that
	// move untouched, and the done total must keep counting the task
	// that stayed in the closed cycle: it is history, not current work.
	if _, err := cycles.EndCycle(ctx, pool, cycles.EndCycleParams{ProjectID: p.ID}, cycles.Authorship{UserID: &u.ID}); err != nil {
		t.Fatalf("EndCycle: %v", err)
	}
	after := stats()
	if after.TodoCount != got.TodoCount || after.DoingCount != got.DoingCount {
		t.Errorf("open counts changed across the cycle boundary: %d/%d, want %d/%d",
			after.TodoCount, after.DoingCount, got.TodoCount, got.DoingCount)
	}
	if after.DoneCount != 1 {
		t.Errorf("done_count = %d after ending the cycle, want 1: closed work counts for the project's life", after.DoneCount)
	}
	if after.LastActivityAt == nil {
		t.Error("last_activity_at must survive a closed cycle")
	}

	// Closing one more task in the new cycle adds to that running total.
	mk("shipped later", tasks.TypeTask, tasks.StateDone)
	if final := stats(); final.DoneCount != 2 {
		t.Errorf("done_count = %d, want 2 across both cycles", final.DoneCount)
	}
}
