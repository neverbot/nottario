package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimConflictError is returned by Claim (the by-id variant) when the
// task is not eligible to be picked up by the caller. Callers use the
// details to decide whether to wait, try another candidate, or
// surface the conflict to a human.
type ClaimConflictError struct {
	TaskID                uuid.UUID         `json:"task_id"`
	CurrentState          State             `json:"current_state"`
	CurrentAssigneeUserID *uuid.UUID        `json:"current_assignee_user_id"`
	Preconditions         []PreconditionRef `json:"preconditions,omitempty"`
	PendingChildrenCount  int               `json:"pending_children_count,omitempty"`
	Reason                string            `json:"reason"`
}

func (e *ClaimConflictError) Error() string {
	return "cannot claim task: " + e.Reason
}

// ClaimNext atomically picks the highest-priority eligible task and
// marks it assigned to callerUserID + state=doing. Uses
// SELECT … FOR UPDATE SKIP LOCKED inside a CTE so a concurrent
// ClaimNext from another agent picks a different task without
// blocking.
//
// Returns (nil, ErrNotFound) when nothing is eligible.
func ClaimNext(ctx context.Context, pool *pgxpool.Pool, f NextFilter, callerUserID uuid.UUID) (*Task, error) {
	if f.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}
	if callerUserID == uuid.Nil {
		return nil, errors.New("caller user_id is required")
	}

	candidate := `
		SELECT t.id
		FROM tasks t
		WHERE t.project_id = $1
		  AND t.state = 'todo'
		  AND (
		    t.type <> 'feature'
		    OR NOT EXISTS (
		        SELECT 1 FROM tasks c
		        WHERE c.parent_task_id = t.id AND c.state <> 'done'
		    )
		  )
		  AND NOT EXISTS (
		      SELECT 1
		      FROM task_dependencies d
		      JOIN tasks d2 ON d2.id = d.depends_on_id
		      WHERE d.task_id = t.id AND d2.state <> 'done'
		  )
	`
	args := []any{f.ProjectID, callerUserID}
	switch {
	case f.AssigneeUserID != nil && len(f.UserRoleIDs) > 0:
		candidate += " AND (t.assignee_user_id = $3 OR (t.assignee_user_id IS NULL AND (t.target_role_id IS NULL OR t.target_role_id = ANY($4))))"
		args = append(args, *f.AssigneeUserID, uuidSliceToArray(f.UserRoleIDs))
	case f.AssigneeUserID != nil:
		candidate += " AND (t.assignee_user_id = $3 OR t.assignee_user_id IS NULL)"
		args = append(args, *f.AssigneeUserID)
	case f.RoleID != nil:
		candidate += " AND (t.target_role_id = $3 OR t.target_role_id IS NULL)"
		args = append(args, *f.RoleID)
	}
	candidate += " ORDER BY t.priority DESC, t.created_at ASC FOR UPDATE SKIP LOCKED LIMIT 1"

	query := fmt.Sprintf(`
		WITH candidate AS (%s)
		UPDATE tasks t
		SET assignee_user_id = $2,
		    state = 'doing',
		    actual_start = COALESCE(t.actual_start, now()),
		    updated_at = now()
		FROM candidate
		WHERE t.id = candidate.id
		RETURNING t.id, t.project_id, t.parent_task_id, t.type, t.title, t.description_md,
		          t.state, t.priority, t.assignee_user_id, t.target_role_id,
		          t.actual_start, t.actual_end,
		          t.created_by_user_id, t.created_by_token_id,
		          t.created_at, t.updated_at
	`, candidate)

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

// Claim atomically claims a SPECIFIC task by id for callerUserID.
// Acquires a row-level lock so a concurrent Claim from another agent
// either waits or fails cleanly. Eligibility rules match ClaimNext;
// when the task is not eligible Claim returns a *ClaimConflictError
// with the current state + assignee + (if applicable) unresolved
// preconditions so the caller can decide what to do next.
//
// Idempotent when the task is already assigned to callerUserID and
// in doing state — returns the task without changes.
func Claim(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, callerUserID uuid.UUID) (*Task, error) {
	if callerUserID == uuid.Nil {
		return nil, errors.New("caller user_id is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var t Task
	err = tx.QueryRow(ctx, `
		SELECT id, project_id, parent_task_id, type, title, description_md,
		       state, priority, assignee_user_id, target_role_id,
		       actual_start, actual_end,
		       created_by_user_id, created_by_token_id,
		       created_at, updated_at
		FROM tasks WHERE id = $1
		FOR UPDATE
	`, id).Scan(
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

	conflict := &ClaimConflictError{
		TaskID:                id,
		CurrentState:          t.State,
		CurrentAssigneeUserID: t.AssigneeUserID,
	}
	switch t.State {
	case StateDone:
		conflict.Reason = "task is already done"
		return nil, conflict
	case StateDoing:
		// Idempotent if the caller already owns it.
		if t.AssigneeUserID != nil && *t.AssigneeUserID == callerUserID {
			return &t, tx.Commit(ctx)
		}
		conflict.Reason = "task is already claimed by another user"
		return nil, conflict
	}
	// state == todo from here.
	if t.AssigneeUserID != nil && *t.AssigneeUserID != callerUserID {
		conflict.Reason = "task is already assigned to another user"
		return nil, conflict
	}
	// Feature with non-done children.
	if t.Type == TypeFeature {
		var pending int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM tasks
			WHERE parent_task_id = $1 AND state <> 'done'
		`, id).Scan(&pending); err != nil {
			return nil, err
		}
		if pending > 0 {
			conflict.PendingChildrenCount = pending
			conflict.Reason = fmt.Sprintf("feature has %d non-done child task(s); the engine rolls it up automatically when they all finish", pending)
			return nil, conflict
		}
	}
	// Unresolved preconditions.
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.title, t.state
		FROM task_dependencies d
		JOIN tasks t ON t.id = d.depends_on_id
		WHERE d.task_id = $1 AND t.state <> 'done'
		ORDER BY t.created_at
	`, id)
	if err != nil {
		return nil, err
	}
	var unresolved []PreconditionRef
	for rows.Next() {
		var p PreconditionRef
		if err := rows.Scan(&p.ID, &p.Title, &p.State); err != nil {
			rows.Close()
			return nil, err
		}
		unresolved = append(unresolved, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(unresolved) > 0 {
		conflict.Preconditions = unresolved
		conflict.Reason = fmt.Sprintf("%d unresolved precondition(s)", len(unresolved))
		return nil, conflict
	}
	// All good — claim it.
	if _, err := tx.Exec(ctx, `
		UPDATE tasks SET
		  assignee_user_id = $1,
		  state = 'doing',
		  actual_start = COALESCE(actual_start, now()),
		  updated_at = now()
		WHERE id = $2
	`, callerUserID, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return Get(ctx, pool, id)
}
