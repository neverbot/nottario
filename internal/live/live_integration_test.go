package live_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/live"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestBuild(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 17001, "live-a", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := identity.UpsertFromGithub(ctx, pool, 17002, "live-b", "B", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Live proj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: title}, by)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	doing := func(tk *tasks.Task) {
		t.Helper()
		if _, _, err := tasks.SetState(ctx, pool, tk.ID, tasks.StateDoing, &u.ID); err != nil {
			t.Fatal(err)
		}
	}
	older, newer, todo, closed := mk("older"), mk("newer"), mk("todo"), mk("closed")
	doing(older)
	doing(newer)
	doing(closed)
	if _, _, err := tasks.SetState(ctx, pool, closed.ID, tasks.StateDone, &u.ID); err != nil {
		t.Fatal(err)
	}
	_ = todo
	now := time.Now()
	setActivity := func(id uuid.UUID, at time.Time) {
		t.Helper()
		if err := dbq.New(pool).SetTaskLastActivityForTest(ctx, dbq.SetTaskLastActivityForTestParams{
			TaskID: id, At: pgTime(at),
		}); err != nil {
			t.Fatal(err)
		}
	}
	setActivity(older.ID, now.Add(-2*time.Hour))
	setActivity(newer.ID, now.Add(-time.Hour))

	board, err := live.Build(ctx, pool, []uuid.UUID{p.ID}, u.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Working) != 2 || board.Working[0].TaskID != newer.ID || board.Working[1].TaskID != older.ID {
		t.Fatalf("working order = %+v", board.Working)
	}
	if len(board.Projects) != 1 || board.Projects[0].IdleAfterMinutes != 30 {
		t.Fatalf("projects = %+v", board.Projects)
	}
	if !board.Working[0].Idle {
		t.Fatalf("an hour old with a 30 minute threshold must be idle")
	}

	// A comment on the older task moves it first and wakes it.
	if _, err := tasks.AddComment(ctx, pool, older.ID, "ping", by); err != nil {
		t.Fatal(err)
	}
	board, err = live.Build(ctx, pool, []uuid.UUID{p.ID}, u.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if board.Working[0].TaskID != older.ID || board.Working[0].Idle {
		t.Fatalf("after a comment: %+v", board.Working[0])
	}

	// The idle threshold is per project.
	if _, err := identity.UpdateProjectLiveIdleMinutes(ctx, pool, p.ID, 5); err != nil {
		t.Fatal(err)
	}
	setActivity(newer.ID, time.Now().Add(-10*time.Minute))
	board, err = live.Build(ctx, pool, []uuid.UUID{p.ID}, u.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var newerItem *live.Item
	for i := range board.Working {
		if board.Working[i].TaskID == newer.ID {
			newerItem = &board.Working[i]
		}
	}
	if newerItem == nil || !newerItem.Idle || board.Projects[0].IdleAfterMinutes != 5 {
		t.Fatalf("idle with 5 minutes: %+v", board.Projects)
	}

	// Finished: inside the window only.
	if len(board.Finished) != 1 || board.Finished[0].TaskID != closed.ID || board.Finished[0].State != "done" {
		t.Fatalf("finished = %+v", board.Finished)
	}
	board, err = live.Build(ctx, pool, []uuid.UUID{p.ID}, u.ID, time.Now().Add(25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Finished) != 0 {
		t.Fatalf("finished after the window = %+v", board.Finished)
	}

	// Steps, sessions, progress and open questions attach.
	steps, err := tasks.AddSteps(ctx, pool, newer.ID, []string{"one", "two"}, by)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: "k1", Label: "Agent one", Client: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.StartStep(ctx, pool, newer.ID, steps[1].ID, &sess.ID, by); err != nil {
		t.Fatal(err)
	}
	mine, err := questions.Ask(ctx, pool, questions.AskParams{
		ProjectID: p.ID, TaskID: newer.ID, SessionID: &sess.ID, AskedBy: by, Body: "for me?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := questions.Ask(ctx, pool, questions.AskParams{
		ProjectID: p.ID, TaskID: older.ID, AskedBy: tasks.Authorship{UserID: &other.ID}, Body: "for other",
	}); err != nil {
		t.Fatal(err)
	}
	board, err = live.Build(ctx, pool, []uuid.UUID{p.ID}, u.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var it live.Item
	for _, w := range board.Working {
		if w.TaskID == newer.ID {
			it = w
		}
	}
	if len(it.Steps) != 1 || it.Steps[0].Text != "two" || it.Steps[0].Session == nil || it.Steps[0].Session.Key != "k1" {
		t.Fatalf("steps = %+v", it.Steps)
	}
	if it.Progress == nil || it.Progress.Total != 2 || it.Progress.Done != 0 || it.QuestionsOpen != 1 {
		t.Fatalf("progress/questions = %+v %d", it.Progress, it.QuestionsOpen)
	}
	// Waiting is scoped to the recipient.
	if len(board.Waiting) != 1 || board.Waiting[0].ID != mine.ID {
		t.Fatalf("waiting = %+v", board.Waiting)
	}
	empty, err := live.Build(ctx, pool, nil, u.ID, time.Now())
	if err != nil || len(empty.Working) != 0 {
		t.Fatalf("no projects: %+v %v", empty, err)
	}
}

func pgTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
