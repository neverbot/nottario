package tasks

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NextFilter narrows what counts as the "next" task.
type NextFilter struct {
	ProjectID      uuid.UUID
	AssigneeUserID *uuid.UUID // when set, only tasks assigned to this user or to roles they hold
	RoleID         *uuid.UUID // when set, only tasks targeting this role or with no role
	UserRoleIDs    []uuid.UUID // when set together with AssigneeUserID, expands eligibility
}

// Next returns the highest-priority task eligible to be picked up:
// - state = 'todo'
// - no unresolved dependencies (every depends_on is in state 'done')
// - matching the assignee/role filter if provided
//
// Tasks of type 'feature' are skipped: features are containers, the
// actual work lives in their children.
func Next(ctx context.Context, pool *pgxpool.Pool, f NextFilter) (*Task, error) {
	if f.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}

	query := `
		SELECT t.id, t.project_id, t.parent_task_id, t.type, t.title, t.description_md,
		       t.state, t.priority, t.assignee_user_id, t.target_role_id,
		       t.actual_start, t.actual_end,
		       t.created_by_user_id, t.created_by_token_id,
		       t.created_at, t.updated_at
		FROM tasks t
		WHERE t.project_id = $1
		  AND t.state = 'todo'
		  AND t.type <> 'feature'
		  AND NOT EXISTS (
		      SELECT 1
		      FROM task_dependencies d
		      JOIN tasks d2 ON d2.id = d.depends_on_id
		      WHERE d.task_id = t.id AND d2.state <> 'done'
		  )
	`
	args := []any{f.ProjectID}
	idx := 2

	switch {
	case f.AssigneeUserID != nil && len(f.UserRoleIDs) > 0:
		query += " AND (t.assignee_user_id = $2 OR (t.assignee_user_id IS NULL AND (t.target_role_id IS NULL OR t.target_role_id = ANY($3))))"
		args = append(args, *f.AssigneeUserID, uuidSliceToArray(f.UserRoleIDs))
		idx = 4
	case f.AssigneeUserID != nil:
		query += " AND t.assignee_user_id = $2"
		args = append(args, *f.AssigneeUserID)
		idx = 3
	case f.RoleID != nil:
		query += " AND (t.target_role_id = $2 OR t.target_role_id IS NULL)"
		args = append(args, *f.RoleID)
		idx = 3
	}
	_ = idx

	query += " ORDER BY t.priority DESC, t.created_at ASC LIMIT 1"

	var t Task
	err := pool.QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.ProjectID, &t.ParentTaskID, &t.Type, &t.Title, &t.DescriptionMD,
		&t.State, &t.Priority, &t.AssigneeUserID, &t.TargetRoleID,
		&t.ActualStart, &t.ActualEnd,
		&t.CreatedByUserID, &t.CreatedByTokenID,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// uuidSliceToArray converts a Go slice into a value pgx will encode
// as a PostgreSQL uuid[] array.
func uuidSliceToArray(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}
