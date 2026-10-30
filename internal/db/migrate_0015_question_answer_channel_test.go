package db_test

import (
	"context"
	"testing"

	"github.com/neverbot/nottario/internal/db"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// answer_channel is web, chat or other, web by default; an answer
// recorded from the chat may be answered and delivered in one write;
// the migration must be reversible and re-appliable.
func TestMigration0015_QuestionAnswerChannel(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9501, "chan15", "Chan15", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Chan15Proj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "t"},
		tasks.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, tok, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md, answer_channel)
		VALUES ($1, $2, 'text', 'where?', 'email')`, p.ID, tk.ID); err == nil {
		t.Error("answer_channel 'email' was accepted")
	}
	var channel string
	if err := pool.QueryRow(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md, state,
		answer_text, answered_at, delivered_at, answer_channel, answered_by_token_id)
		VALUES ($1, $2, 'text', 'said in chat?', 'answered', 'yes', now(), now(), 'chat', $3)
		RETURNING answer_channel`, p.ID, tk.ID, tok.ID).Scan(&channel); err != nil || channel != "chat" {
		t.Errorf("chat-recorded answer: %q err=%v", channel, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO task_questions (project_id, task_id, kind, body_md)
		VALUES ($1, $2, 'text', 'default?') RETURNING answer_channel`, p.ID, tk.ID).Scan(&channel); err != nil || channel != "web" {
		t.Errorf("default answer_channel = %q err=%v", channel, err)
	}

	// Revoking the token keeps the answer, without the token.
	if _, err := pool.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1`, tok.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_questions WHERE answer_channel = 'chat' AND answered_by_token_id IS NULL`).Scan(&n); err != nil || n != 1 {
		t.Errorf("after token delete: %d err=%v", n, err)
	}

	// Round trip: down to 14 and back up.
	if err := db.MigrateTo(ctx, pool, 14); err != nil {
		t.Fatalf("down to 14: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
