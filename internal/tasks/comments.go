package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
)

// AddComment appends a markdown comment to a task.
func AddComment(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, body string, by Authorship) (*Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errInvalid("body is required")
	}
	row, err := dbq.New(pool).InsertTaskComment(ctx, dbq.InsertTaskCommentParams{
		TaskID:        taskID,
		AuthorUserID:  by.UserID,
		AuthorTokenID: by.TokenID,
		BodyMd:        body,
	})
	if err != nil {
		return nil, err
	}
	c := Comment{
		ID:            row.ID,
		TaskID:        row.TaskID,
		AuthorUserID:  row.AuthorUserID,
		AuthorTokenID: row.AuthorTokenID,
		BodyMD:        row.BodyMd,
		CreatedAt:     row.CreatedAt.Time,
	}
	if c.AuthorTokenID != nil {
		names, err := identity.LookupTokenNames(ctx, pool, []uuid.UUID{*c.AuthorTokenID})
		if err != nil {
			return nil, err
		}
		c.ViaMCP = identity.ViaMCPFromMap(c.AuthorTokenID, names)
	}
	return &c, nil
}

// ListComments returns comments for a task, oldest first.
func ListComments(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]Comment, error) {
	rows, err := dbq.New(pool).ListTaskComments(ctx, taskID)
	if err != nil {
		return nil, err
	}
	tokenIDs := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.AuthorTokenID != nil {
			tokenIDs = append(tokenIDs, *r.AuthorTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, pool, tokenIDs)
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Comment{
			ID:            r.ID,
			TaskID:        r.TaskID,
			AuthorUserID:  r.AuthorUserID,
			AuthorTokenID: r.AuthorTokenID,
			ViaMCP:        identity.ViaMCPFromMap(r.AuthorTokenID, names),
			BodyMD:        r.BodyMd,
			CreatedAt:     r.CreatedAt.Time,
		})
	}
	return out, nil
}
