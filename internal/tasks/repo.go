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
	"github.com/jackc/pgx/v5/pgxpool"
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

// Create inserts a new task. State defaults to 'todo'.
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

	var out Task
	err := pool.QueryRow(ctx, `
		INSERT INTO tasks (
			project_id, parent_task_id, type, title, description_md,
			state, priority, assignee_user_id, target_role_id,
			created_by_user_id, created_by_token_id
		)
		VALUES ($1, $2, $3, $4, $5, 'todo', $6, $7, $8, $9, $10)
		RETURNING id, project_id, parent_task_id, type, title, description_md,
		          state, priority, assignee_user_id, target_role_id,
		          actual_start, actual_end,
		          created_by_user_id, created_by_token_id,
		          created_at, updated_at
	`, p.ProjectID, p.ParentTaskID, t, p.Title, p.DescriptionMD,
		priority, p.AssigneeUserID, p.TargetRoleID,
		by.UserID, by.TokenID,
	).Scan(
		&out.ID, &out.ProjectID, &out.ParentTaskID, &out.Type, &out.Title, &out.DescriptionMD,
		&out.State, &out.Priority, &out.AssigneeUserID, &out.TargetRoleID,
		&out.ActualStart, &out.ActualEnd,
		&out.CreatedByUserID, &out.CreatedByTokenID,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Get loads a task by id.
func Get(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Task, error) {
	var t Task
	err := pool.QueryRow(ctx, `
		SELECT id, project_id, parent_task_id, type, title, description_md,
		       state, priority, assignee_user_id, target_role_id,
		       actual_start, actual_end,
		       created_by_user_id, created_by_token_id,
		       created_at, updated_at
		FROM tasks WHERE id = $1
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
	return &t, nil
}

// ListFilter restricts a List call.
type ListFilter struct {
	ProjectID       uuid.UUID
	State           State
	Type            Type
	AssigneeUserID  *uuid.UUID
	TargetRoleID    *uuid.UUID
	ParentTaskID    *uuid.UUID
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

	query := `
		SELECT id, project_id, parent_task_id, type, title, description_md,
		       state, priority, assignee_user_id, target_role_id,
		       actual_start, actual_end,
		       created_by_user_id, created_by_token_id,
		       created_at, updated_at
		FROM tasks WHERE project_id = $1
	`
	args := []any{f.ProjectID}
	idx := 2
	if f.State != "" {
		query += fmt.Sprintf(" AND state = $%d", idx)
		args = append(args, f.State)
		idx++
	}
	if f.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, f.Type)
		idx++
	}
	if f.AssigneeUserID != nil {
		query += fmt.Sprintf(" AND assignee_user_id = $%d", idx)
		args = append(args, *f.AssigneeUserID)
		idx++
	}
	if f.TargetRoleID != nil {
		query += fmt.Sprintf(" AND target_role_id = $%d", idx)
		args = append(args, *f.TargetRoleID)
		idx++
	}
	if f.ParentTaskID != nil {
		query += fmt.Sprintf(" AND parent_task_id = $%d", idx)
		args = append(args, *f.ParentTaskID)
		idx++
	} else if !f.IncludeChildren {
		query += " AND parent_task_id IS NULL"
	}
	query += " ORDER BY priority DESC, created_at ASC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.ParentTaskID, &t.Type, &t.Title, &t.DescriptionMD,
			&t.State, &t.Priority, &t.AssigneeUserID, &t.TargetRoleID,
			&t.ActualStart, &t.ActualEnd,
			&t.CreatedByUserID, &t.CreatedByTokenID,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
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

	query := `
		SELECT id, project_id, parent_task_id, type, title, description_md,
		       state, priority, assignee_user_id, target_role_id,
		       actual_start, actual_end,
		       created_by_user_id, created_by_token_id,
		       created_at, updated_at
		FROM tasks WHERE project_id = $1
	`
	args := []any{f.ProjectID}
	idx := 2
	if f.State != "" {
		query += fmt.Sprintf(" AND state = $%d", idx)
		args = append(args, f.State)
		idx++
	}
	if f.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, f.Type)
		idx++
	}
	if f.AssigneeUserID != nil {
		query += fmt.Sprintf(" AND assignee_user_id = $%d", idx)
		args = append(args, *f.AssigneeUserID)
		idx++
	}
	if f.TargetRoleID != nil {
		query += fmt.Sprintf(" AND target_role_id = $%d", idx)
		args = append(args, *f.TargetRoleID)
		idx++
	}
	if f.ParentTaskID != nil {
		query += fmt.Sprintf(" AND parent_task_id = $%d", idx)
		args = append(args, *f.ParentTaskID)
		idx++
	} else if !f.IncludeChildren {
		query += " AND parent_task_id IS NULL"
	}
	if after != nil {
		query += fmt.Sprintf(`
			AND (
			  priority <  $%d
			  OR (priority = $%d AND created_at > $%d)
			  OR (priority = $%d AND created_at = $%d AND id > $%d)
			)`,
			idx, idx, idx+1, idx, idx+1, idx+2)
		args = append(args, after.Priority, after.CreatedAt, after.ID)
		idx += 3
	}
	query += fmt.Sprintf(" ORDER BY priority DESC, created_at ASC, id ASC LIMIT $%d", idx)
	args = append(args, limit+1)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.ParentTaskID, &t.Type, &t.Title, &t.DescriptionMD,
			&t.State, &t.Priority, &t.AssigneeUserID, &t.TargetRoleID,
			&t.ActualStart, &t.ActualEnd,
			&t.CreatedByUserID, &t.CreatedByTokenID,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return Page{}, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
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
	if p.TargetRoleID != nil || p.AssigneeUserID != nil {
		// Resolve task's project to validate against.
		var projectID uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT project_id FROM tasks WHERE id = $1`, id).Scan(&projectID); err != nil {
			return nil, err
		}
		if err := validateTaskAssignments(ctx, pool, projectID, p.TargetRoleID, p.AssigneeUserID); err != nil {
			return nil, err
		}
	}
	sets := []string{}
	args := []any{id}
	idx := 2
	add := func(expr string, value any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", expr, idx))
		args = append(args, value)
		idx++
	}
	if p.Title != nil {
		add("title", *p.Title)
	}
	if p.DescriptionMD != nil {
		add("description_md", *p.DescriptionMD)
	}
	if p.Type != nil {
		if !ValidType(*p.Type) {
			return nil, fmt.Errorf("invalid type: %q", *p.Type)
		}
		add("type", *p.Type)
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.UnsetAssignee {
		sets = append(sets, "assignee_user_id = NULL")
	} else if p.AssigneeUserID != nil {
		add("assignee_user_id", *p.AssigneeUserID)
	}
	if p.UnsetTargetRole {
		sets = append(sets, "target_role_id = NULL")
	} else if p.TargetRoleID != nil {
		add("target_role_id", *p.TargetRoleID)
	}
	if len(sets) == 0 {
		return Get(ctx, pool, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = $1", joinComma(sets))
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		return nil, err
	}
	return Get(ctx, pool, id)
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
	// When closing a non-feature task, require every direct precondition
	// to be `done`. Features are rolled up automatically by the engine
	// (see rollUpParentDone) so this check would race with itself; we
	// skip it for them.
	if s == StateDone {
		var taskType string
		if err := pool.QueryRow(ctx,
			`SELECT type FROM tasks WHERE id = $1`, id).Scan(&taskType); err != nil {
			return nil, err
		}
		if taskType != "feature" {
			rows, err := pool.Query(ctx, `
				SELECT t.id, t.title, t.state
				FROM task_dependencies td
				JOIN tasks t ON t.id = td.depends_on_id
				WHERE td.task_id = $1 AND t.state <> 'done'
				ORDER BY t.actual_end NULLS LAST, t.created_at
			`, id)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var unresolved []PreconditionRef
			for rows.Next() {
				var p PreconditionRef
				if err := rows.Scan(&p.ID, &p.Title, &p.State); err != nil {
					return nil, err
				}
				unresolved = append(unresolved, p)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			if len(unresolved) > 0 {
				return nil, &UnresolvedPreconditionsError{TaskID: id, Preconditions: unresolved}
			}
		}
	}
	var actualStartSQL, actualEndSQL string
	switch s {
	case StateTodo:
		actualStartSQL = "actual_start = NULL"
		actualEndSQL = "actual_end = NULL"
	case StateDoing:
		actualStartSQL = "actual_start = COALESCE(actual_start, now())"
		actualEndSQL = "actual_end = NULL"
	case StateDone:
		actualStartSQL = "actual_start = COALESCE(actual_start, now())"
		actualEndSQL = "actual_end = now()"
	}
	query := fmt.Sprintf(
		"UPDATE tasks SET state = $2, %s, %s WHERE id = $1",
		actualStartSQL, actualEndSQL,
	)
	if _, err := pool.Exec(ctx, query, id, s); err != nil {
		return nil, err
	}
	t, err := Get(ctx, pool, id)
	if err != nil {
		return nil, err
	}
	// Bubble "done" upward: every ancestor feature whose children are
	// all done becomes done too. The skill bundle promises this; we now
	// honour it. We only roll forward (never un-done a parent when a
	// child reopens) because that would surprise a human who manually
	// closed a feature.
	if s == StateDone {
		if err := rollUpParentDone(ctx, pool, t.ParentTaskID); err != nil {
			return nil, err
		}
		// Re-fetch the task in case its own parent chain mutated other
		// rows that observers care about. (Cheap.)
		t, err = Get(ctx, pool, id)
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// rollUpParentDone walks the parent chain and marks any parent feature
// whose children are now all `done` as done too. No-op when parent is
// nil, already done, or has at least one non-done child.
func rollUpParentDone(ctx context.Context, pool *pgxpool.Pool, parentID *uuid.UUID) error {
	for parentID != nil {
		var pState string
		var pNext *uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT state, parent_task_id FROM tasks WHERE id = $1
		`, *parentID).Scan(&pState, &pNext); err != nil {
			return err
		}
		if pState == "done" {
			return nil
		}
		var pending int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM tasks
			WHERE parent_task_id = $1 AND state <> 'done'
		`, *parentID).Scan(&pending); err != nil {
			return err
		}
		if pending > 0 {
			return nil
		}
		if _, err := pool.Exec(ctx, `
			UPDATE tasks SET state = 'done',
			       actual_start = COALESCE(actual_start, now()),
			       actual_end = now()
			 WHERE id = $1
		`, *parentID); err != nil {
			return err
		}
		parentID = pNext
	}
	return nil
}

// Delete removes the task. Children and links cascade.
func Delete(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	cmd, err := pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// validateTaskAssignments ensures the role and assignee, when given,
// belong to the project. A clear error makes the agent recover by
// calling projects.list_roles / projects.list_members instead of
// silently storing an unusable foreign key.
func validateTaskAssignments(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, roleID *uuid.UUID, userID *uuid.UUID) error {
	if roleID != nil {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1 AND project_id = $2)`,
			*roleID, projectID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("target_role_id %s does not belong to this project (call projects.list_roles to discover valid role ids)", roleID)
		}
	}
	if userID != nil {
		// Admins can be assigned even without an explicit membership;
		// otherwise the user must be a member of the project.
		var ok bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM users WHERE id = $1 AND is_admin = true
			) OR EXISTS (
				SELECT 1 FROM memberships
				WHERE user_id = $1 AND project_id = $2
			)
		`, *userID, projectID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("assignee_user_id %s is not a member of this project (use projects/{id}/members to grant a role first)", userID)
		}
	}
	return nil
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
