package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/db"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// Writes on a task or anything hanging off it must advance
// last_activity_at; a choice question needs options; the migration
// must be reversible and re-appliable.
func TestMigration0014_LiveAndQuestions(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9401, "live14", "Live14", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Live14Proj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "t"},
		tasks.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatal(err)
	}

	backdate := func() time.Time {
		t.Helper()
		var ts time.Time
		if err := pool.QueryRow(ctx, `UPDATE task_activity SET last_activity_at = now() - interval '1 hour'
			WHERE task_id = $1 RETURNING last_activity_at`, tk.ID).Scan(&ts); err != nil {
			t.Fatalf("backdate: %v", err)
		}
		return ts
	}
	activity := func() time.Time {
		t.Helper()
		var ts time.Time
		if err := pool.QueryRow(ctx, `SELECT last_activity_at FROM task_activity WHERE task_id = $1`, tk.ID).Scan(&ts); err != nil {
			t.Fatal(err)
		}
		return ts
	}
	// bumps runs fn and reports whether it advanced the task's activity.
	bumps := func(name string, want bool, sql string, args ...any) {
		t.Helper()
		before := backdate()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := activity().After(before); got != want {
			t.Errorf("%s: activity bumped = %v, want %v", name, got, want)
		}
	}

	// The task insert itself recorded activity.
	activity()

	var stepID string
	if err := pool.QueryRow(ctx, `INSERT INTO task_steps (task_id, position, text) VALUES ($1, 0, 's') RETURNING id::text`,
		tk.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	bumps("step insert", true, `INSERT INTO task_steps (task_id, position, text) VALUES ($1, 1, 's2')`, tk.ID)
	bumps("step update", true, `UPDATE task_steps SET done = true, done_at = now() WHERE id = $1`, stepID)
	bumps("step delete", true, `DELETE FROM task_steps WHERE task_id = $1 AND position = 1`, tk.ID)
	bumps("comment insert", true, `INSERT INTO task_comments (task_id, author_user_id, body_md) VALUES ($1, $2, 'hi')`, tk.ID, u.ID)
	bumps("commit insert", true, `INSERT INTO task_commits (task_id, repo, sha) VALUES ($1, 'r', 'abc')`, tk.ID)
	bumps("task state change", true, `UPDATE tasks SET state = 'doing' WHERE id = $1`, tk.ID)
	bumps("task title change", true, `UPDATE tasks SET title = 'renamed' WHERE id = $1`, tk.ID)
	bumps("cycle_id-only update", false, `UPDATE tasks SET cycle_id = cycle_id WHERE id = $1`, tk.ID)

	// Child writes must leave the tasks row (and its conflict token) alone.
	var updBefore, updAfter time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM tasks WHERE id = $1`, tk.ID).Scan(&updBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_comments (task_id, author_user_id, body_md) VALUES ($1, $2, 'again')`,
		tk.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM tasks WHERE id = $1`, tk.ID).Scan(&updAfter); err != nil {
		t.Fatal(err)
	}
	if !updAfter.Equal(updBefore) {
		t.Errorf("comment insert changed tasks.updated_at: %v -> %v", updBefore, updAfter)
	}

	// A choice question without options violates the CHECK.
	if _, err := pool.Exec(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md)
		VALUES ($1, $2, 'choice', 'which?')`, p.ID, tk.ID); err == nil {
		t.Error("choice question without options was accepted")
	}
	// A text question works and moves activity.
	bumps("question insert", true, `INSERT INTO task_questions (project_id, task_id, kind, body_md)
		VALUES ($1, $2, 'text', 'why?')`, p.ID, tk.ID)
	bumps("question update", true, `UPDATE task_questions SET state = 'withdrawn', withdrawn_at = now() WHERE task_id = $1`, tk.ID)

	// Deleting a task with steps, a comment and a question cascades
	// through triggers that must not trip the task_activity FK.
	if _, err := pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, tk.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_activity WHERE task_id = $1`, tk.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("task_activity rows after task delete = %d, want 0", n)
	}

	// default_view accepts the new values.
	for _, v := range []string{"live", "questions"} {
		if _, err := pool.Exec(ctx, `UPDATE projects SET default_view = $1 WHERE id = $2`, v, p.ID); err != nil {
			t.Errorf("default_view %q rejected: %v", v, err)
		}
	}

	// Round trip: down to 13 and back up.
	if err := db.MigrateTo(ctx, pool, 13); err != nil {
		t.Fatalf("down to 13: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
