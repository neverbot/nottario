package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/db/dbq"
)

// ErrNotFound is returned when a lookup yields no row.
var ErrNotFound = errors.New("task not found")

// Authorship identifies who created or commented on a task. Either
// a user (humans through the web) or an API token (agents through
// the MCP/REST). Both can be nil for system-created rows.
type Authorship struct {
	UserID  *uuid.UUID
	TokenID *uuid.UUID
}

// CreateParams carries the fields settable at task creation time.
type CreateParams struct {
	ProjectID      uuid.UUID
	ParentTaskID   *uuid.UUID
	Type           Type
	Title          string
	DescriptionMD  string
	Priority       *int
	AssigneeUserID *uuid.UUID
	TargetRoleID   *uuid.UUID
}

// Create inserts a new task. State defaults to 'todo'. Implemented
// via sqlc-generated dbq.InsertTask.
func Create(ctx context.Context, pool *pgxpool.Pool, p CreateParams, by Authorship) (*Task, error) {
	if p.Title == "" {
		return nil, errors.New("title is required")
	}
	t := p.Type
	if t == "" {
		t = TypeTask
	}
	if !ValidType(t) {
		return nil, fmt.Errorf("invalid type: %q", t)
	}
	if err := validateTaskAssignments(ctx, pool, p.ProjectID, p.TargetRoleID, p.AssigneeUserID); err != nil {
		return nil, err
	}
	priority := 50
	if p.Priority != nil {
		priority = *p.Priority
	}
	// Resolve the project's active cycle so the top-level insert has a
	// concrete cycle_id (NOT NULL). For parented tasks the cascade
	// trigger overrides whatever we pass with the parent's cycle_id,
	// so the value we use here is irrelevant in that case but must
	// still satisfy NOT NULL — the active cycle works for both.
	active, err := cycles.ActiveCycle(ctx, pool, p.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("resolve active cycle: %w", err)
	}
	row, err := dbq.New(pool).InsertTask(ctx, dbq.InsertTaskParams{
		ProjectID:        p.ProjectID,
		ParentTaskID:     p.ParentTaskID,
		Type:             string(t),
		Title:            p.Title,
		DescriptionMd:    p.DescriptionMD,
		Priority:         int32(priority),
		AssigneeUserID:   p.AssigneeUserID,
		TargetRoleID:     p.TargetRoleID,
		CreatedByUserID:  by.UserID,
		CreatedByTokenID: by.TokenID,
		CycleID:          active.ID,
	})
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
		CycleID:          row.CycleID,
	}, nil
}

// Get loads a task by id. Implemented via sqlc-generated dbq.GetTask
// to bypass any chance of injection in this hot path. The hand-
// written SQL it replaced lived inline above; the canonical version
// now lives in internal/db/queries/tasks.sql.
func Get(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Task, error) {
	row, err := dbq.New(pool).GetTask(ctx, id)
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
		CycleID:          row.CycleID,
	}, nil
}

// timestampPtr returns a *time.Time when the dbq timestamp column was
// non-null and nil otherwise, matching the shape callers expect on
// Task.ActualStart / Task.ActualEnd.
func timestampPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// ListFilter restricts a List call.
type ListFilter struct {
	ProjectID       uuid.UUID
	State           State
	Type            Type
	AssigneeUserID  *uuid.UUID
	TargetRoleID    *uuid.UUID
	ParentTaskID    *uuid.UUID
	CycleID         *uuid.UUID
	IncludeChildren bool // when false (default), only top-level tasks (parent IS NULL) are returned
}

// Cursor identifies the last item of a page in the (priority DESC,
// created_at ASC, id ASC) ordering. Encoded opaquely so callers
// shouldn't depend on its shape.
type Cursor struct {
	Priority  int       `json:"p"`
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

// Page wraps a paginated slice of tasks plus the cursor for the next
// page (nil when this was the last page).
type Page struct {
	Tasks      []Task
	NextCursor *Cursor
	HasMore    bool
}

// EncodeCursor base64-encodes an opaque cursor so clients can pass it
// back verbatim.
func EncodeCursor(c *Cursor) (string, error) {
	if c == nil {
		return "", nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// DecodeCursor reverses EncodeCursor.
func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("invalid cursor: " + err.Error())
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, errors.New("invalid cursor: " + err.Error())
	}
	return &c, nil
}

// List returns tasks for a project filtered by f, ordered by
// priority DESC, created_at ASC.
func List(ctx context.Context, pool *pgxpool.Pool, f ListFilter) ([]Task, error) {
	if f.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}
	rows, err := dbq.New(pool).ListTasks(ctx, dbq.ListTasksParams{
		ProjectID:       f.ProjectID,
		State:           pgtypeText(string(f.State)),
		Type:            pgtypeText(string(f.Type)),
		AssigneeUserID:  f.AssigneeUserID,
		TargetRoleID:    f.TargetRoleID,
		ParentTaskID:    f.ParentTaskID,
		CycleID:         f.CycleID,
		IncludeChildren: f.IncludeChildren,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromListRow(r))
	}
	return out, nil
}

// pgtypeText returns a non-null pgtype.Text when s is non-empty and
// a NULL one otherwise — matches the "" sentinel ListFilter uses.
func pgtypeText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func taskFromListRow(r dbq.ListTasksRow) Task {
	return Task{
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
		CycleID:          r.CycleID,
	}
}

// ListPaginated runs the same filters as List but with a keyset
// cursor and a hard limit. `limit` is clamped to the [1, 500] range
// (callers should already have resolved nil/zero to the project's
// configured page size). Returns the page plus the next cursor or
// `(_, false, nil)` when there are no more rows.
func ListPaginated(ctx context.Context, pool *pgxpool.Pool, f ListFilter, limit int, after *Cursor) (Page, error) {
	if f.ProjectID == uuid.Nil {
		return Page{}, errors.New("project_id is required")
	}
	if limit < 1 {
		limit = 1
	} else if limit > 500 {
		limit = 500
	}
	params := dbq.ListTasksPaginatedParams{
		ProjectID:       f.ProjectID,
		State:           pgtypeText(string(f.State)),
		Type:            pgtypeText(string(f.Type)),
		AssigneeUserID:  f.AssigneeUserID,
		TargetRoleID:    f.TargetRoleID,
		ParentTaskID:    f.ParentTaskID,
		CycleID:         f.CycleID,
		IncludeChildren: f.IncludeChildren,
		PageLimit:       int32(limit + 1),
	}
	if after != nil {
		params.CursorPriority = pgtype.Int4{Int32: int32(after.Priority), Valid: true}
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}
		params.CursorID = &after.ID
	}
	rows, err := dbq.New(pool).ListTasksPaginated(ctx, params)
	if err != nil {
		return Page{}, err
	}
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		// ListTasksPaginatedRow has the same columns as ListTasksRow;
		// reuse the converter by name-conversion.
		out = append(out, Task{
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
			CycleID:          r.CycleID,
		})
	}
	page := Page{}
	if len(out) > limit {
		page.HasMore = true
		out = out[:limit]
	}
	page.Tasks = out
	if page.HasMore && len(out) > 0 {
		last := out[len(out)-1]
		page.NextCursor = &Cursor{Priority: last.Priority, CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// UpdateParams enumerates the mutable fields of a task. A nil
// pointer means "leave unchanged"; an empty/zero value means
// "set to that value".
type UpdateParams struct {
	Title           *string
	DescriptionMD   *string
	Type            *Type
	Priority        *int
	AssigneeUserID  *uuid.UUID
	UnsetAssignee   bool
	TargetRoleID    *uuid.UUID
	UnsetTargetRole bool
}

// Update mutates the fields enumerated in p.
func Update(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, p UpdateParams) (*Task, error) {
	if p.Type != nil && !ValidType(*p.Type) {
		return nil, fmt.Errorf("invalid type: %q", *p.Type)
	}
	if p.TargetRoleID != nil || p.AssigneeUserID != nil {
		q := dbq.New(pool)
		projectID, err := q.ProjectIDForTask(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := validateTaskAssignments(ctx, pool, projectID, p.TargetRoleID, p.AssigneeUserID); err != nil {
			return nil, err
		}
	}
	params := dbq.UpdateTaskFieldsParams{
		ID:              id,
		Title:           pgtype.Text{Valid: p.Title != nil, String: derefString(p.Title)},
		DescriptionMd:   pgtype.Text{Valid: p.DescriptionMD != nil, String: derefString(p.DescriptionMD)},
		Type:            pgtype.Text{Valid: p.Type != nil, String: stringFromType(p.Type)},
		Priority:        pgtype.Int4{Valid: p.Priority != nil, Int32: int32(derefInt(p.Priority))},
		UnsetAssignee:   p.UnsetAssignee,
		AssigneeUserID:  p.AssigneeUserID,
		UnsetTargetRole: p.UnsetTargetRole,
		TargetRoleID:    p.TargetRoleID,
	}
	row, err := dbq.New(pool).UpdateTaskFields(ctx, params)
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
		CycleID:          row.CycleID,
	}, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func stringFromType(p *Type) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// SetState changes the state and manages actual_start / actual_end
// transitions atomically.
// UnresolvedPreconditionsError is returned by SetState when an agent
// (or REST caller) tries to close a task whose `depends_on` graph
// still has at least one non-done precondition. The Preconditions
// slice carries enough detail for the caller to surface a useful
// message ("you still owe: A, B, C").
type UnresolvedPreconditionsError struct {
	TaskID        uuid.UUID         `json:"task_id"`
	Preconditions []PreconditionRef `json:"preconditions"`
}

// PreconditionRef is the minimal shape a caller needs to find an
// unresolved precondition without an extra round-trip.
type PreconditionRef struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	State State     `json:"state"`
}

func (e *UnresolvedPreconditionsError) Error() string {
	n := len(e.Preconditions)
	if n == 1 {
		return fmt.Sprintf("cannot close task: 1 unresolved precondition (%s, %s)",
			e.Preconditions[0].Title, e.Preconditions[0].State)
	}
	return fmt.Sprintf("cannot close task: %d unresolved preconditions", n)
}

func SetState(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, s State) (*Task, error) {
	if !ValidState(s) {
		return nil, fmt.Errorf("invalid state: %q", s)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	// Lock the task row for the rest of this transaction. AddDependency
	// (and any other writer that may want to mutate this task's deps)
	// takes the same lock, so they serialize: the precondition check
	// below sees a stable view, and a concurrent add_dependency either
	// waits or finds the task already in 'done'.
	header, err := q.LockTaskTypeAndParent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// When closing a non-feature task, require every direct precondition
	// to be `done`. Features are rolled up automatically by the engine
	// (see rollUpParentDoneTx) so this check would race with itself; we
	// skip it for them.
	if s == StateDone && header.Type != "feature" {
		rows, err := q.ListUnresolvedPreconditions(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			unresolved := make([]PreconditionRef, 0, len(rows))
			for _, r := range rows {
				unresolved = append(unresolved, PreconditionRef{
					ID:    r.ID,
					Title: r.Title,
					State: State(r.State),
				})
			}
			return nil, &UnresolvedPreconditionsError{TaskID: id, Preconditions: unresolved}
		}
	}

	switch s {
	case StateTodo:
		err = q.SetTaskTodo(ctx, id)
	case StateDoing:
		err = q.SetTaskDoing(ctx, id)
	case StateDone:
		err = q.SetTaskDone(ctx, id)
	}
	if err != nil {
		return nil, err
	}

	// Bubble "done" upward inside the same transaction so two siblings
	// closing concurrently can't both miss the parent.
	if s == StateDone {
		if err := rollUpParentDoneTx(ctx, tx, header.ParentTaskID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return Get(ctx, pool, id)
}

// rollUpParentDoneTx walks the parent chain inside the given
// transaction, locking each parent row with FOR UPDATE so concurrent
// child closures serialize on the parent. No-op when parent is nil,
// already done, or has at least one non-done child.
func rollUpParentDoneTx(ctx context.Context, tx pgx.Tx, parentID *uuid.UUID) error {
	q := dbq.New(tx)
	for parentID != nil {
		header, err := q.GetParentStateAndGrandparent(ctx, *parentID)
		if err != nil {
			return err
		}
		if header.State == "done" {
			return nil
		}
		pending, err := q.CountNonDoneChildren(ctx, parentID)
		if err != nil {
			return err
		}
		if pending > 0 {
			return nil
		}
		if err := q.SetTaskDone(ctx, *parentID); err != nil {
			return err
		}
		parentID = header.ParentTaskID
	}
	return nil
}

// Delete removes the task. Children and links cascade. Implemented
// via sqlc-generated dbq.DeleteTask.
func Delete(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	rows, err := dbq.New(pool).DeleteTask(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// validateTaskAssignments ensures the role and assignee, when given,
// belong to the project. A clear error makes the agent recover by
// calling projects.list_roles / projects.list_members instead of
// silently storing an unusable foreign key.
func validateTaskAssignments(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, roleID *uuid.UUID, userID *uuid.UUID) error {
	q := dbq.New(pool)
	if roleID != nil {
		ok, err := q.RoleExistsInProject(ctx, dbq.RoleExistsInProjectParams{
			ID: *roleID, ProjectID: projectID,
		})
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("target_role_id %s does not belong to this project (call projects.list_roles to discover valid role ids)", roleID)
		}
	}
	if userID != nil {
		ok, err := q.UserBelongsToProjectOrIsAdmin(ctx, dbq.UserBelongsToProjectOrIsAdminParams{
			ID: *userID, ProjectID: projectID,
		})
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("assignee_user_id %s is not a member of this project (use projects/{id}/members to grant a role first)", userID)
		}
	}
	return nil
}
