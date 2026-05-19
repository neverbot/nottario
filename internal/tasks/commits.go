package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LinkCommit attaches a git commit to a task. Repo must be in
// "owner/repo" format; sha is any non-empty string (full or short).
func LinkCommit(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, repo, sha, message string, by Authorship) error {
	repo = strings.TrimSpace(repo)
	sha = strings.TrimSpace(sha)
	if repo == "" || sha == "" {
		return errInvalid("repo and sha are required")
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO task_commits (task_id, repo, sha, message, added_by_user_id, added_by_token_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (task_id, repo, sha) DO UPDATE SET message = EXCLUDED.message
	`, taskID, repo, sha, message, by.UserID, by.TokenID)
	return err
}

// UnlinkCommit removes the (repo, sha) link from the task.
func UnlinkCommit(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, repo, sha string) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM task_commits WHERE task_id = $1 AND repo = $2 AND sha = $3
	`, taskID, repo, sha)
	return err
}

// ListCommits returns the commits attached to a task.
func ListCommits(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]CommitLink, error) {
	rows, err := pool.Query(ctx, `
		SELECT task_id, repo, sha, message, added_at
		FROM task_commits
		WHERE task_id = $1
		ORDER BY added_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommitLink{}
	for rows.Next() {
		var c CommitLink
		if err := rows.Scan(&c.TaskID, &c.Repo, &c.SHA, &c.Message, &c.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type invalidErr string

func (e invalidErr) Error() string { return string(e) }

func errInvalid(msg string) error { return invalidErr(msg) }
