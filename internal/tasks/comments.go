package tasks

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
		ID:             row.ID,
		TaskID:         row.TaskID,
		AuthorUserID:   row.AuthorUserID,
		AuthorTokenID:  row.AuthorTokenID,
		BodyMD:         row.BodyMd,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		EditedAt:       timestampPtr(row.EditedAt),
		EditedByUserID: row.EditedByUserID,
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
			ID:             r.ID,
			TaskID:         r.TaskID,
			AuthorUserID:   r.AuthorUserID,
			AuthorTokenID:  r.AuthorTokenID,
			ViaMCP:         identity.ViaMCPFromMap(r.AuthorTokenID, names),
			BodyMD:         r.BodyMd,
			CreatedAt:      r.CreatedAt.Time,
			UpdatedAt:      r.UpdatedAt.Time,
			EditedAt:       timestampPtr(r.EditedAt),
			EditedByUserID: r.EditedByUserID,
		})
	}
	return out, nil
}

// GetComment returns a single comment by id (used for permission checks
// before edit/delete and to surface the current `updated_at` after a
// 409 stale response).
func GetComment(ctx context.Context, pool *pgxpool.Pool, commentID uuid.UUID) (*Comment, error) {
	row, err := dbq.New(pool).GetTaskComment(ctx, commentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c := &Comment{
		ID:             row.ID,
		TaskID:         row.TaskID,
		AuthorUserID:   row.AuthorUserID,
		AuthorTokenID:  row.AuthorTokenID,
		BodyMD:         row.BodyMd,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		EditedAt:       timestampPtr(row.EditedAt),
		EditedByUserID: row.EditedByUserID,
	}
	if c.AuthorTokenID != nil {
		names, err := identity.LookupTokenNames(ctx, pool, []uuid.UUID{*c.AuthorTokenID})
		if err != nil {
			return nil, err
		}
		c.ViaMCP = identity.ViaMCPFromMap(c.AuthorTokenID, names)
	}
	return c, nil
}

// UpdateCommentParams carries the inputs for UpdateComment.
type UpdateCommentParams struct {
	Body              string
	CallerUserID      uuid.UUID
	ExpectedUpdatedAt time.Time
}

// UpdateComment edits the body of a comment with optimistic concurrency.
// Returns ErrConflict if expected_updated_at doesn't match the row.
func UpdateComment(ctx context.Context, pool *pgxpool.Pool, commentID uuid.UUID, p UpdateCommentParams) (*Comment, error) {
	body := strings.TrimSpace(p.Body)
	if body == "" {
		return nil, errInvalid("body is required")
	}
	expected := pgtype.Timestamptz{}
	if !p.ExpectedUpdatedAt.IsZero() {
		expected = pgtype.Timestamptz{Time: p.ExpectedUpdatedAt, Valid: true}
	}
	row, err := dbq.New(pool).UpdateTaskComment(ctx, dbq.UpdateTaskCommentParams{
		ID:                commentID,
		BodyMd:            body,
		CallerUserID:      p.CallerUserID,
		ExpectedUpdatedAt: expected,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	c := &Comment{
		ID:             row.ID,
		TaskID:         row.TaskID,
		AuthorUserID:   row.AuthorUserID,
		AuthorTokenID:  row.AuthorTokenID,
		BodyMD:         row.BodyMd,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		EditedAt:       timestampPtr(row.EditedAt),
		EditedByUserID: row.EditedByUserID,
	}
	if c.AuthorTokenID != nil {
		names, err := identity.LookupTokenNames(ctx, pool, []uuid.UUID{*c.AuthorTokenID})
		if err != nil {
			return nil, err
		}
		c.ViaMCP = identity.ViaMCPFromMap(c.AuthorTokenID, names)
	}
	return c, nil
}

// DeleteComment hard-deletes a comment row. Caller must have verified
// authorisation (author OR admin) before invoking.
func DeleteComment(ctx context.Context, pool *pgxpool.Pool, commentID uuid.UUID) error {
	n, err := dbq.New(pool).DeleteTaskComment(ctx, commentID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
