package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// ownershipFixture returns a project plus two distinct users: the one
// acting and somebody else, so "does not steal an existing assignee"
// can actually be observed.
func ownershipFixture(t *testing.T) (context.Context, *pgxpool.Pool, identity.Project, identity.User, identity.User) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	actor, _, err := identity.UpsertFromGithub(ctx, pool, 9201, "actor", "Actor", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	other, _, err := identity.UpsertFromGithub(ctx, pool, 9202, "other", "Other", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "OwnProj", "", "", "", actor.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v (%d)", err, len(roles))
	}
	if err := identity.AddMembership(ctx, pool, other.ID, p.ID, roles[0].ID); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	return ctx, pool, *p, *actor, *other
}

// Nothing reaches doing or done without somebody answering for it,
// whatever route the mover took.
func TestSetState_LeavingTodoGivesAnUnownedTaskAnOwner(t *testing.T) {
	ctx, pool, p, actor, other := ownershipFixture(t)
	by := tasks.Authorship{UserID: &actor.ID}

	newTask := func(t *testing.T, assignee *uuid.UUID) *tasks.Task {
		t.Helper()
		task, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID: p.ID, Type: tasks.TypeTask, Title: "work", AssigneeUserID: assignee,
		}, by)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return task
	}

	t.Run("doing", func(t *testing.T) {
		task := newTask(t, nil)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateDoing, &actor.ID)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID == nil || *got.AssigneeUserID != actor.ID {
			t.Errorf("assignee = %v, want the acting user", got.AssigneeUserID)
		}
	})

	t.Run("straight to done", func(t *testing.T) {
		task := newTask(t, nil)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateDone, &actor.ID)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID == nil || *got.AssigneeUserID != actor.ID {
			t.Errorf("assignee = %v, want the acting user", got.AssigneeUserID)
		}
	})

	t.Run("wont_do", func(t *testing.T) {
		task := newTask(t, nil)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateWontDo, &actor.ID)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID == nil || *got.AssigneeUserID != actor.ID {
			t.Errorf("assignee = %v, want the acting user", got.AssigneeUserID)
		}
	})

	t.Run("never steals an existing assignee", func(t *testing.T) {
		task := newTask(t, &other.ID)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateDone, &actor.ID)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID == nil || *got.AssigneeUserID != other.ID {
			t.Errorf("assignee = %v, want it left on the original owner", got.AssigneeUserID)
		}
	})

	t.Run("back to todo assigns nobody", func(t *testing.T) {
		task := newTask(t, nil)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateTodo, &actor.ID)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID != nil {
			t.Errorf("assignee = %v, want nil: a task returned to the backlog has no owner", got.AssigneeUserID)
		}
	})

	t.Run("no actor, no assignment", func(t *testing.T) {
		task := newTask(t, nil)
		got, err := tasks.SetState(ctx, pool, task.ID, tasks.StateDoing, nil)
		if err != nil {
			t.Fatalf("SetState: %v", err)
		}
		if got.AssigneeUserID != nil {
			t.Errorf("assignee = %v, want nil for an automated transition", got.AssigneeUserID)
		}
	})
}

// Close is the recommended way to finish a task, so it has to give the
// task an owner as well.
func TestClose_GivesAnUnownedTaskAnOwner(t *testing.T) {
	ctx, pool, p, actor, _ := ownershipFixture(t)
	by := tasks.Authorship{UserID: &actor.ID}
	task, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "close me",
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	res, err := tasks.Close(ctx, pool, task.ID, tasks.CloseParams{
		State: tasks.StateDone, Comment: "done",
	}, by)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if res.Task.AssigneeUserID == nil || *res.Task.AssigneeUserID != actor.ID {
		t.Errorf("assignee = %v, want the closing user", res.Task.AssigneeUserID)
	}
}

// claim=true is the one-call form of "I am filing the work I am about
// to start".
func TestCreate_Claim(t *testing.T) {
	ctx, pool, p, actor, _ := ownershipFixture(t)
	task, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "mine", Claim: true,
	}, tasks.Authorship{UserID: &actor.ID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.State != tasks.StateDoing {
		t.Errorf("state = %q, want doing", task.State)
	}
	if task.AssigneeUserID == nil || *task.AssigneeUserID != actor.ID {
		t.Errorf("assignee = %v, want the creator", task.AssigneeUserID)
	}
	if task.ActualStart == nil {
		t.Error("actual_start not recorded for a claimed task")
	}

	// Without a user behind the call there is nobody to claim for.
	if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "nobody", Claim: true,
	}, tasks.Authorship{}); err == nil {
		t.Error("claim without a user should be refused")
	}

	// The default is still an unclaimed backlog row.
	plain, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "backlog",
	}, tasks.Authorship{UserID: &actor.ID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plain.State != tasks.StateTodo || plain.AssigneeUserID != nil {
		t.Errorf("plain create = %q / %v, want todo with no assignee", plain.State, plain.AssigneeUserID)
	}
}
