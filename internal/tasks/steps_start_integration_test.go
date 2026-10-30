package tasks_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestStartStep(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 9311, "startstep", "StartStep", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "StartStepProj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	session := func(key string) *uuid.UUID {
		t.Helper()
		s, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: key})
		if err != nil {
			t.Fatal(err)
		}
		return &s.ID
	}
	mk := func(title string, texts ...string) (*tasks.Task, []tasks.Step) {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: title}, by)
		if err != nil {
			t.Fatal(err)
		}
		if len(texts) == 0 {
			return tk, nil
		}
		st, err := tasks.AddSteps(ctx, pool, tk.ID, texts, by)
		if err != nil {
			t.Fatal(err)
		}
		return tk, st
	}

	t.Run("todo task moves to doing and is assigned", func(t *testing.T) {
		tk, st := mk("todo start", "a", "b")
		s1 := session("s-todo")
		got, err := tasks.StartStep(ctx, pool, tk.ID, st[0].ID, s1, by)
		if err != nil {
			t.Fatal(err)
		}
		if !got[0].InProgress() || got[1].InProgress() || got[0].StartedSessionID == nil || *got[0].StartedSessionID != *s1 {
			t.Errorf("steps after start: %+v", got)
		}
		after, err := tasks.Get(ctx, pool, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.State != tasks.StateDoing || after.AssigneeUserID == nil || *after.AssigneeUserID != u.ID {
			t.Errorf("task = %s assignee %v", after.State, after.AssigneeUserID)
		}
		var active *uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT active_session_id FROM tasks WHERE id = $1`, tk.ID).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active == nil || *active != *s1 {
			t.Errorf("active session = %v", active)
		}
	})

	t.Run("same session moves its step, another session keeps its own", func(t *testing.T) {
		tk, st := mk("sessions", "a", "b", "c")
		s1, s2 := session("s-one"), session("s-two")
		if _, err := tasks.StartStep(ctx, pool, tk.ID, st[0].ID, s1, by); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.StartStep(ctx, pool, tk.ID, st[2].ID, s2, by); err != nil {
			t.Fatal(err)
		}
		got, err := tasks.StartStep(ctx, pool, tk.ID, st[1].ID, s1, by)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].InProgress() || !got[1].InProgress() || !got[2].InProgress() {
			t.Errorf("in progress = %v %v %v", got[0].InProgress(), got[1].InProgress(), got[2].InProgress())
		}
	})

	t.Run("done step and closed task are rejected", func(t *testing.T) {
		tk, st := mk("rejects", "a")
		done := true
		if _, err := tasks.UpdateStep(ctx, pool, tk.ID, st[0].ID, tasks.UpdateStepParams{Done: &done}, by); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.StartStep(ctx, pool, tk.ID, st[0].ID, nil, by); err == nil {
			t.Error("starting a done step succeeded")
		}
		if _, err := tasks.StartStep(ctx, pool, tk.ID, uuid.New(), nil, by); err == nil {
			t.Error("starting an unknown step succeeded")
		}
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDone, &u.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.StartStep(ctx, pool, tk.ID, st[0].ID, nil, by); err == nil {
			t.Error("starting a step of a closed task succeeded")
		}
	})

	t.Run("ticking a started step ends its progress", func(t *testing.T) {
		tk, st := mk("tick", "a")
		if _, err := tasks.StartStep(ctx, pool, tk.ID, st[0].ID, session("s-tick"), by); err != nil {
			t.Fatal(err)
		}
		done := true
		s, err := tasks.UpdateStep(ctx, pool, tk.ID, st[0].ID, tasks.UpdateStepParams{Done: &done}, by)
		if err != nil {
			t.Fatal(err)
		}
		if s.InProgress() {
			t.Error("a done step is still in progress")
		}
	})

	t.Run("active session needs the assignee", func(t *testing.T) {
		tk, _ := mk("owner")
		if _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDoing, &u.ID); err != nil {
			t.Fatal(err)
		}
		ok, err := tasks.SetActiveSession(ctx, pool, tk.ID, *session("s-own"), uuid.New())
		if err != nil || ok {
			t.Errorf("stranger: ok=%v err=%v", ok, err)
		}
		ok, err = tasks.SetActiveSession(ctx, pool, tk.ID, *session("s-own"), u.ID)
		if err != nil || !ok {
			t.Errorf("assignee: ok=%v err=%v", ok, err)
		}
	})
}
