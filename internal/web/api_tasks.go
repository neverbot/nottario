package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/tasks"
)

// TaskDeps wires the task endpoints.
type TaskDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
	// Notifier is called AFTER task writes commit so the notification
	// path is off the write critical section. Nil-safe: a zero Notifier
	// or a nil pointer produces no rows.
	Notifier *notifications.Notifier
}

func (d TaskDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

// actorFrom returns the caller's user id as *uuid.UUID for the
// Notifier's actor argument. Nil when the caller is anonymous or
// when the user id is zero (shouldn't happen for authenticated
// requests but keeps the helper defensive).
func actorFrom(c identity.Caller) *uuid.UUID {
	if c.UserID == uuid.Nil {
		return nil
	}
	uid := c.UserID
	return &uid
}

func (d TaskDeps) authorship(c identity.Caller) tasks.Authorship {
	a := tasks.Authorship{}
	switch c.Source {
	case identity.SourceSession:
		uid := c.UserID
		a.UserID = &uid
	case identity.SourceToken:
		uid := c.UserID
		tid := c.TokenID
		a.UserID = &uid
		a.TokenID = &tid
	}
	return a
}

// projectIDFromPath parses the {id} param.
func projectIDFromPath(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}

// taskIDFromPath parses the {task_id} param.
func taskIDFromPath(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("task_id"))
}

// ensureProjectAccess loads the project and confirms the caller can
// see it (admin or any membership). It returns 404 to avoid leaking
// existence to outsiders.
func (d TaskDeps) ensureProjectAccess(ctx context.Context, c identity.Caller, projectID uuid.UUID) error {
	if err := identity.RequireProjectScope(c, projectID); err != nil {
		return err
	}
	if c.IsAdmin {
		return nil
	}
	roles, err := identity.UserRoleIDs(ctx, d.Pool, c.UserID, projectID)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		return errors.New("not a project member")
	}
	return nil
}

// ListDependenciesHandler returns every dependency edge for the project.
func ListDependenciesHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		deps, err := tasks.ListAllDependencies(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
	})
}

// ListInconsistenciesHandler returns the project's inconsistent tasks
// with a `reason` key per entry.
func ListInconsistenciesHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		items, err := tasks.ListInconsistencies(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"inconsistencies": items})
	})
}

// ListTasksHandler returns the project's tasks.
func ListTasksHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}

		f := tasks.ListFilter{ProjectID: pid, IncludeChildren: r.URL.Query().Get("include_children") == "true"}
		if v := r.URL.Query().Get("state"); v != "" {
			f.State = tasks.State(v)
		}
		if v := r.URL.Query().Get("type"); v != "" {
			f.Type = tasks.Type(v)
		}
		if v := r.URL.Query().Get("assignee_user_id"); v != "" {
			id, err := uuid.Parse(v)
			if err == nil {
				f.AssigneeUserID = &id
			}
		}
		if v := r.URL.Query().Get("target_role_id"); v != "" {
			id, err := uuid.Parse(v)
			if err == nil {
				f.TargetRoleID = &id
			}
		}
		if v := r.URL.Query().Get("parent_task_id"); v != "" {
			id, err := uuid.Parse(v)
			if err == nil {
				f.ParentTaskID = &id
			}
		}
		// cycle_id: same rule as the MCP path. Empty → default to the
		// active cycle so the board/gantt narrow to the current sprint
		// after End Sprint. "all" → no filter. Anything else → uuid.
		// If the project has no active cycle for any reason, fall back
		// to no filter rather than 500-ing the whole list.
		switch v := r.URL.Query().Get("cycle_id"); v {
		case "all":
			// no filter
		case "":
			if active, err := cycles.ActiveCycle(r.Context(), d.Pool, pid); err == nil {
				id := active.ID
				f.CycleID = &id
			}
		default:
			id, err := uuid.Parse(v)
			if err == nil {
				f.CycleID = &id
			}
		}

		list, err := tasks.List(r.Context(), d.Pool, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": list})
	})
}

// GetTaskHandler returns a task with its dependencies, commits and comments.
func GetTaskHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		t, err := tasks.Get(r.Context(), d.Pool, tid)
		if errors.Is(err, tasks.ErrNotFound) || (err == nil && t.ProjectID != pid) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		deps, _ := tasks.ListDependenciesOf(r.Context(), d.Pool, tid)
		commits, _ := tasks.ListCommits(r.Context(), d.Pool, tid)
		comments, _ := tasks.ListComments(r.Context(), d.Pool, tid)
		writeJSON(w, http.StatusOK, map[string]any{
			"task":       t,
			"depends_on": deps,
			"commits":    commits,
			"comments":   comments,
		})
	})
}

type createTaskRequest struct {
	ParentTaskID *uuid.UUID `json:"parent_task_id"`
	Type         tasks.Type `json:"type"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Priority     *int       `json:"priority"`
	// PriorityKey is an alternative to Priority: when set, the server
	// resolves it against the project's priority catalogue and uses
	// the bucket's numeric value. Priority (numeric) takes precedence
	// when both are present.
	PriorityKey    string     `json:"priority_key"`
	AssigneeUserID *uuid.UUID `json:"assignee_user_id"`
	TargetRoleID   *uuid.UUID `json:"target_role_id"`
}

// CreateTaskHandler creates a task within the project.
func CreateTaskHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		var req createTaskRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Resolve priority_key when no explicit numeric priority is given;
		// then fall back to the project's medium bucket so tasks created
		// from the UI/REST don't end up off-bucket (raw 50 looks like
		// "p50" in the Gantt when 50 isn't a bucket value).
		priority := req.Priority
		if priority == nil && req.PriorityKey != "" {
			v, err := identity.ResolvePriorityKey(r.Context(), d.Pool, pid, req.PriorityKey)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			priority = &v
		}
		if priority == nil {
			v, err := identity.DefaultPriorityValue(r.Context(), d.Pool, pid)
			if err == nil {
				priority = &v
			}
		}
		t, err := tasks.Create(r.Context(), d.Pool, tasks.CreateParams{
			ProjectID:      pid,
			ParentTaskID:   req.ParentTaskID,
			Type:           req.Type,
			Title:          req.Title,
			DescriptionMD:  req.Description,
			Priority:       priority,
			AssigneeUserID: req.AssigneeUserID,
			TargetRoleID:   req.TargetRoleID,
		}, d.authorship(c))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, t)
	})
}

type updateTaskRequest struct {
	Title           *string     `json:"title"`
	Description     *string     `json:"description"`
	Type            *tasks.Type `json:"type"`
	Priority        *int        `json:"priority"`
	AssigneeUserID  *uuid.UUID  `json:"assignee_user_id"`
	UnsetAssignee   bool        `json:"unset_assignee"`
	TargetRoleID    *uuid.UUID  `json:"target_role_id"`
	UnsetTargetRole bool        `json:"unset_target_role"`
}

// UpdateTaskHandler edits the mutable fields of a task.
func UpdateTaskHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req updateTaskRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Snapshot the task BEFORE the write so the Notifier can tell
		// if the assignee actually changed (silent same-value updates
		// must not notify).
		prev, _ := tasks.Get(r.Context(), d.Pool, tid)
		t, err := tasks.Update(r.Context(), d.Pool, tid, tasks.UpdateParams{
			Title:           req.Title,
			DescriptionMD:   req.Description,
			Type:            req.Type,
			Priority:        req.Priority,
			AssigneeUserID:  req.AssigneeUserID,
			UnsetAssignee:   req.UnsetAssignee,
			TargetRoleID:    req.TargetRoleID,
			UnsetTargetRole: req.UnsetTargetRole,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if d.Notifier != nil {
			actor := actorFrom(c)
			d.Notifier.OnAssigneeChanged(r.Context(), prev, t, actor)
		}
		writeJSON(w, http.StatusOK, t)
	})
}

type setStateRequest struct {
	State tasks.State `json:"state"`
}

// SetTaskStateHandler transitions the task state and updates actual_*.
func SetTaskStateHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req setStateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Snapshot state pre-write so the Notifier can tell whether
		// the transition actually crossed into a closed state.
		prev, _ := tasks.Get(r.Context(), d.Pool, tid)
		t, err := tasks.SetState(r.Context(), d.Pool, tid, req.State)
		if err != nil {
			// Surface the unresolved-precondition detail to clients so
			// they can render a useful message without an extra round-trip.
			var uerr *tasks.UnresolvedPreconditionsError
			if errors.As(err, &uerr) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":         uerr.Error(),
					"preconditions": uerr.Preconditions,
				})
				return
			}
			// Refused done ↔ wont_do transitions also get a structured
			// body so the UI can explain the lifecycle rule.
			var terr *tasks.ErrInvalidStateTransition
			if errors.As(err, &terr) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":      terr.Error(),
					"from_state": string(terr.From),
					"to_state":   string(terr.To),
				})
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if d.Notifier != nil {
			prevState := tasks.State("")
			if prev != nil {
				prevState = prev.State
			}
			actor := actorFrom(c)
			d.Notifier.OnStateChanged(r.Context(), t, prevState, actor)
		}
		writeJSON(w, http.StatusOK, t)
	})
}

// DeleteTaskHandler removes a task.
func DeleteTaskHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		if err := tasks.Delete(r.Context(), d.Pool, tid); err != nil {
			if errors.Is(err, tasks.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NextTaskHandler returns the next eligible task for the caller.
func NextTaskHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}

		// Default: any eligible task in the project. The query params
		// 'assignee_user_id', 'role_id' and 'mine=true' narrow it.
		f := tasks.NextFilter{ProjectID: pid}
		if r.URL.Query().Get("mine") == "true" {
			userID := c.UserID
			roleIDs, _ := identity.UserRoleIDs(r.Context(), d.Pool, userID, pid)
			f.AssigneeUserID = &userID
			f.UserRoleIDs = roleIDs
		}
		if v := r.URL.Query().Get("assignee_user_id"); v != "" {
			id, err := uuid.Parse(v)
			if err == nil {
				f.AssigneeUserID = &id
				f.UserRoleIDs = nil
			}
		}
		if v := r.URL.Query().Get("role_id"); v != "" {
			id, err := uuid.Parse(v)
			if err == nil {
				f.RoleID = &id
				f.AssigneeUserID = nil
				f.UserRoleIDs = nil
			}
		}

		t, err := tasks.Next(r.Context(), d.Pool, f)
		if errors.Is(err, tasks.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"task": nil})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": t})
	})
}

type dependencyRequest struct {
	DependsOnID uuid.UUID `json:"depends_on_id"`
}

// AddDependencyHandler declares that the task depends on another.
func AddDependencyHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req dependencyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := tasks.AddDependency(r.Context(), d.Pool, tid, req.DependsOnID); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, tasks.ErrCycle) {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// RemoveDependencyHandler drops a dependency edge.
func RemoveDependencyHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		depID, err := uuid.Parse(r.PathValue("dep_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid dep id")
			return
		}
		if err := tasks.RemoveDependency(r.Context(), d.Pool, tid, depID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type linkCommitRequest struct {
	Repo    string `json:"repo"`
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// LinkCommitHandler attaches a commit to a task.
func LinkCommitHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req linkCommitRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := tasks.LinkCommit(r.Context(), d.Pool, tid, req.Repo, req.SHA, req.Message, d.authorship(c)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type commentRequest struct {
	Body string `json:"body"`
}

// AddCommentHandler appends a comment to a task.
func AddCommentHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req commentRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cm, err := tasks.AddComment(r.Context(), d.Pool, tid, req.Body, d.authorship(c))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if d.Notifier != nil {
			if task, gerr := tasks.Get(r.Context(), d.Pool, tid); gerr == nil {
				actor := actorFrom(c)
				d.Notifier.OnComment(r.Context(), task, actor)
			}
		}
		writeJSON(w, http.StatusCreated, cm)
	})
}
