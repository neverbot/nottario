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
		if err := pool.QueryRow(ctx, `UPDATE tasks SET last_activity_at = now() - interval '1 hour'
			WHERE id = $1 RETURNING last_activity_at`, tk.ID).Scan(&ts); err != nil {
			t.Fatalf("backdate: %v", err)
		}
		return ts
	}
	activity := func() time.Time {
		t.Helper()
		var ts time.Time
		if err := pool.QueryRow(ctx, `SELECT last_activity_at FROM tasks WHERE id = $1`, tk.ID).Scan(&ts); err != nil {
			t.Fatal(err)
		}
		return ts
	}

	// A plain task update moves activity (BEFORE UPDATE trigger).
	before := backdate()
	if _, err := pool.Exec(ctx, `UPDATE tasks SET title = 'renamed' WHERE id = $1`, tk.ID); err != nil {
		t.Fatal(err)
	}
	if !activity().After(before) {
		t.Error("task update did not advance last_activity_at")
	}

	// A comment moves activity (child trigger).
	before = backdate()
	if _, err := pool.Exec(ctx, `INSERT INTO task_comments (task_id, author_user_id, body_md) VALUES ($1, $2, 'hi')`,
		tk.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if !activity().After(before) {
		t.Error("comment insert did not advance last_activity_at")
	}

	// A choice question without options violates the CHECK.
	if _, err := pool.Exec(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md)
		VALUES ($1, $2, 'choice', 'which?')`, p.ID, tk.ID); err == nil {
		t.Error("choice question without options was accepted")
	}
	// A text question works and moves activity.
	before = backdate()
	if _, err := pool.Exec(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md)
		VALUES ($1, $2, 'text', 'why?')`, p.ID, tk.ID); err != nil {
		t.Fatalf("insert text question: %v", err)
	}
	if !activity().After(before) {
		t.Error("question insert did not advance last_activity_at")
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
