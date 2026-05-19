package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AddComment appends a markdown comment to a task.
func AddComment(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, body string, by Authorship) (*Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errInvalid("body is required")
	}
	var c Comment
	err := pool.QueryRow(ctx, `
		INSERT INTO task_comments (task_id, author_user_id, author_token_id, body_md)
		VALUES ($1, $2, $3, $4)
		RETURNING id, task_id, author_user_id, author_token_id, body_md, created_at
	`, taskID, by.UserID, by.TokenID, body).Scan(
		&c.ID, &c.TaskID, &c.AuthorUserID, &c.AuthorTokenID, &c.BodyMD, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListComments returns comments for a task, oldest first.
func ListComments(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]Comment, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, task_id, author_user_id, author_token_id, body_md, created_at
		FROM task_comments
		WHERE task_id = $1
		ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorUserID, &c.AuthorTokenID, &c.BodyMD, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
