package tasks

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// NextFilter narrows what counts as the "next" task.
type NextFilter struct {
	ProjectID      uuid.UUID
	CycleID        *uuid.UUID  // when set, restrict to this cycle; nil means all cycles
	AssigneeUserID *uuid.UUID  // when set, only tasks assigned to this user or to roles they hold
	RoleID         *uuid.UUID  // when set, only tasks targeting this role or with no role
	UserRoleIDs    []uuid.UUID // when set together with AssigneeUserID, expands eligibility
}

// Next returns the highest-priority task eligible to be picked up:
// - state = 'todo'
// - no unresolved dependencies (every depends_on is in state 'done')
// - matching the assignee/role filter if provided
//
// PREVIEW ONLY (no side effects). For atomic claim use ClaimNext.
func Next(ctx context.Context, pool *pgxpool.Pool, f NextFilter) (*Task, error) {
	if f.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}
	row, err := dbq.New(pool).NextEligibleTask(ctx, dbq.NextEligibleTaskParams{
		ProjectID:      f.ProjectID,
		CycleID:        f.CycleID,
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
	t := &Task{
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
	}
	if err := enrichTaskViaMCP(ctx, pool, []*Task{t}); err != nil {
		return nil, err
	}
	return t, nil
}

// nonNilUUIDs ensures pgx receives an empty slice rather than a nil
// one — sqlc's array_length() helper distinguishes them.
func nonNilUUIDs(in []uuid.UUID) []uuid.UUID {
	if in == nil {
		return []uuid.UUID{}
	}
	return in
}
