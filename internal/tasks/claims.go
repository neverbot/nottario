package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
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
	row, err := dbq.New(pool).ClaimNextEligibleTask(ctx, dbq.ClaimNextEligibleTaskParams{
		ProjectID:      f.ProjectID,
		CallerUserID:   callerUserID,
		AssigneeUserID: f.AssigneeUserID,
		UserRoleIds:    nonNilUUIDs(f.UserRoleIDs),
		RoleID:         f.RoleID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Task{
		ID:               row.ID,
		ProjectID:        row.ProjectID,
		ParentTaskID:     row.ParentTaskID,
		Type:             Type(row.Type),
		Title:            row.Title,
		DescriptionMD:    row.DescriptionMd,
		State:            State(row.State),
		Priority:         int(row.Priority),
		AssigneeUserID:   row.AssigneeUserID,
		TargetRoleID:     row.TargetRoleID,
		ActualStart:      timestampPtr(row.ActualStart),
		ActualEnd:        timestampPtr(row.ActualEnd),
		CreatedByUserID:  row.CreatedByUserID,
		CreatedByTokenID: row.CreatedByTokenID,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}, nil
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
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	row, err := q.GetTaskForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	state := State(row.State)
	conflict := &ClaimConflictError{
		TaskID:                id,
		CurrentState:          state,
		CurrentAssigneeUserID: row.AssigneeUserID,
	}
	switch state {
	case StateDone:
		conflict.Reason = "task is already done"
		return nil, conflict
	case StateDoing:
		if row.AssigneeUserID != nil && *row.AssigneeUserID == callerUserID {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return rowToTaskFromForUpdate(row), nil
		}
		conflict.Reason = "task is already claimed by another user"
		return nil, conflict
	}
	if row.AssigneeUserID != nil && *row.AssigneeUserID != callerUserID {
		conflict.Reason = "task is already assigned to another user"
		return nil, conflict
	}
	if Type(row.Type) == TypeFeature {
		pending, err := q.CountNonDoneChildren(ctx, &id)
		if err != nil {
			return nil, err
		}
		if pending > 0 {
			conflict.PendingChildrenCount = int(pending)
			conflict.Reason = fmt.Sprintf("feature has %d non-done child task(s); the engine rolls it up automatically when they all finish", pending)
			return nil, conflict
		}
	}
	preconds, err := q.ListUnresolvedPreconditions(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(preconds) > 0 {
		unresolved := make([]PreconditionRef, 0, len(preconds))
		for _, p := range preconds {
			unresolved = append(unresolved, PreconditionRef{ID: p.ID, Title: p.Title, State: State(p.State)})
		}
		conflict.Preconditions = unresolved
		conflict.Reason = fmt.Sprintf("%d unresolved precondition(s)", len(unresolved))
		return nil, conflict
	}
	if err := q.ClaimTask(ctx, dbq.ClaimTaskParams{
		ID:             id,
		AssigneeUserID: &callerUserID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return Get(ctx, pool, id)
}

func rowToTaskFromForUpdate(r dbq.GetTaskForUpdateRow) *Task {
	return &Task{
		ID:               r.ID,
		ProjectID:        r.ProjectID,
		ParentTaskID:     r.ParentTaskID,
		Type:             Type(r.Type),
		Title:            r.Title,
		DescriptionMD:    r.DescriptionMd,
		State:            State(r.State),
		Priority:         int(r.Priority),
		AssigneeUserID:   r.AssigneeUserID,
		TargetRoleID:     r.TargetRoleID,
		ActualStart:      timestampPtr(r.ActualStart),
		ActualEnd:        timestampPtr(r.ActualEnd),
		CreatedByUserID:  r.CreatedByUserID,
		CreatedByTokenID: r.CreatedByTokenID,
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}
