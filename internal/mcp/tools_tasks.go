package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// slimTask is the compact payload mutations return by default. The
// fields are exactly what an agent needs to chain the next call
// (id + updated_at for optimistic concurrency + state/role/assignee
// for the routing decision the agent is about to make), minus the
// expensive ones (description, created_by, via_mcp, actual_start/end,
// cycle_id which the project's active cycle covers). A typical
// claim → comment → done loop drops from ~5 KB to ~600 B of MCP
// traffic per round-trip.
type slimTask struct {
	ID             uuid.UUID   `json:"id"`
	Type           tasks.Type  `json:"type"`
	Title          string      `json:"title"`
	State          tasks.State `json:"state"`
	Priority       int         `json:"priority"`
	ParentTaskID   *uuid.UUID  `json:"parent_task_id,omitempty"`
	TargetRoleID   *uuid.UUID  `json:"target_role_id,omitempty"`
	AssigneeUserID *uuid.UUID  `json:"assignee_user_id,omitempty"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func toSlimTask(t *tasks.Task) slimTask {
	return slimTask{
		ID:             t.ID,
		Type:           t.Type,
		Title:          t.Title,
		State:          t.State,
		Priority:       t.Priority,
		ParentTaskID:   t.ParentTaskID,
		TargetRoleID:   t.TargetRoleID,
		AssigneeUserID: t.AssigneeUserID,
		UpdatedAt:      t.UpdatedAt,
	}
}

// taskPayload returns either the slim shape (default) or the full
// *tasks.Task (when the caller asked for verbose). Use this at every
// MCP mutation return site so the "minimal by default" rule is
// enforced in one place.
func taskPayload(t *tasks.Task, verbose bool) any {
	if verbose {
		return t
	}
	return toSlimTask(t)
}

// slimComment mirrors slimTask for comments. Body is the big payload
// agents pass IN; echoing it back is pure waste. Keep the timestamps
// so the agent can chain an immediate edit if it wants.
type slimComment struct {
	ID        uuid.UUID `json:"id"`
	TaskID    uuid.UUID `json:"task_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func commentPayload(c *tasks.Comment, verbose bool) any {
	if verbose {
		return c
	}
	return slimComment{
		ID:        c.ID,
		TaskID:    c.TaskID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// slimTaskList trims the per-row description from a list response. Same
// principle as taskPayload but applied to the multi-row case where the
// description is the largest field by far.
func slimTaskList(ts []tasks.Task, verbose bool) any {
	if verbose {
		return ts
	}
	out := make([]slimTask, len(ts))
	for i := range ts {
		out[i] = toSlimTask(&ts[i])
	}
	return out
}

// Common input fragments. Every tool that targets a task carries an
// explicit project_id (caller-supplied — no per-session "active
// project") and a task_id.

type tasksListInput struct {
	ProjectID       string `json:"project_id" jsonschema:"project uuid"`
	State           string `json:"state,omitempty" jsonschema:"filter: 'todo','doing','done','wont_do'"`
	Type            string `json:"type,omitempty" jsonschema:"filter: 'task','bug','chore','spike','feature'"`
	AssigneeUserID  string `json:"assignee_user_id,omitempty" jsonschema:"filter by assignee uuid"`
	TargetRoleID    string `json:"target_role_id,omitempty" jsonschema:"filter by target_role uuid"`
	ParentTaskID    string `json:"parent_task_id,omitempty" jsonschema:"list children of this feature task"`
	CycleID         string `json:"cycle_id,omitempty" jsonschema:"cycle uuid, or 'all' for every cycle. Omit = active cycle."`
	IncludeChildren bool   `json:"include_children,omitempty" jsonschema:"include parented tasks too (default: top-level only)"`
	IncludeClosed   bool   `json:"include_closed,omitempty" jsonschema:"include done/wont_do rows (default: open only). Explicit state filter overrides."`
	Limit           int    `json:"limit,omitempty" jsonschema:"page size 1..500; omit for project's mcp_page_size"`
	Cursor          string `json:"cursor,omitempty" jsonschema:"opaque cursor from a previous next_cursor; empty = first page"`
	Verbose         bool   `json:"verbose,omitempty" jsonschema:"full Task per row instead of slim shape"`
}

type taskRefInput struct {
	ProjectID       string `json:"project_id" jsonschema:"project uuid"`
	TaskID          string `json:"task_id" jsonschema:"task uuid"`
	IncludeDeps     bool   `json:"include_deps,omitempty" jsonschema:"tasks.get: include depends_on"`
	IncludeCommits  bool   `json:"include_commits,omitempty" jsonschema:"tasks.get: include commits"`
	IncludeComments bool   `json:"include_comments,omitempty" jsonschema:"tasks.get: include comments (can be large)"`
	Verbose         bool   `json:"verbose,omitempty" jsonschema:"tasks.claim: full Task instead of slim shape"`
}

type tasksNextInput struct {
	ProjectID      string `json:"project_id" jsonschema:"project uuid"`
	CycleID        string `json:"cycle_id,omitempty" jsonschema:"cycle uuid, or 'all'. Omit = active cycle."`
	AssigneeUserID string `json:"assignee_user_id,omitempty" jsonschema:"restrict to this user"`
	RoleID         string `json:"role_id,omitempty" jsonschema:"restrict to this target role"`
	Verbose        bool   `json:"verbose,omitempty" jsonschema:"full Task instead of slim shape"`
}

type tasksCreateInput struct {
	ProjectID      string `json:"project_id" jsonschema:"project uuid"`
	Title          string `json:"title" jsonschema:"short title"`
	Description    string `json:"description,omitempty" jsonschema:"markdown body"`
	Type           string `json:"type,omitempty" jsonschema:"'task' (default), 'bug', 'chore', 'spike' or 'feature'"`
	Priority       *int   `json:"priority,omitempty" jsonschema:"raw 0-100. Prefer priority_key."`
	PriorityKey    string `json:"priority_key,omitempty" jsonschema:"bucket key from projects.list_priorities"`
	AssigneeUserID string `json:"assignee_user_id,omitempty" jsonschema:"assignee uuid"`
	TargetRoleID   string `json:"target_role_id,omitempty" jsonschema:"target role uuid"`
	ParentTaskID   string `json:"parent_task_id,omitempty" jsonschema:"parent feature task uuid"`
	Verbose        bool   `json:"verbose,omitempty" jsonschema:"full Task instead of slim shape"`
}

type tasksUpdateInput struct {
	ProjectID      string  `json:"project_id" jsonschema:"project uuid"`
	TaskID         string  `json:"task_id" jsonschema:"task uuid"`
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Type           *string `json:"type,omitempty"`
	Priority       *int    `json:"priority,omitempty" jsonschema:"raw 0-100. Prefer priority_key."`
	PriorityKey    string  `json:"priority_key,omitempty" jsonschema:"bucket key from projects.list_priorities"`
	AssigneeUserID *string `json:"assignee_user_id,omitempty" jsonschema:"uuid, or '' to unset"`
	TargetRoleID   *string `json:"target_role_id,omitempty" jsonschema:"uuid, or '' to unset"`
	Verbose        bool    `json:"verbose,omitempty" jsonschema:"full Task instead of slim shape"`
}

type tasksStateInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	TaskID    string `json:"task_id" jsonschema:"task uuid"`
	State     string `json:"state" jsonschema:"'todo','doing','done','wont_do'"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"full Task instead of slim shape"`
}

type tasksDepInput struct {
	ProjectID   string `json:"project_id" jsonschema:"project uuid"`
	TaskID      string `json:"task_id" jsonschema:"dependent task uuid"`
	DependsOnID string `json:"depends_on_id" jsonschema:"prerequisite task uuid"`
}

type tasksLinkCommitInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	TaskID    string `json:"task_id" jsonschema:"task uuid"`
	Repo      string `json:"repo" jsonschema:"'owner/repo'"`
	SHA       string `json:"sha" jsonschema:"commit SHA"`
	Message   string `json:"message,omitempty" jsonschema:"commit subject for display"`
}

type tasksCommentInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	TaskID    string `json:"task_id" jsonschema:"task uuid"`
	Body      string `json:"body" jsonschema:"markdown body"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"full Comment with body echo"`
}

type tasksCloseCommitInput struct {
	Repo    string `json:"repo" jsonschema:"'owner/repo'"`
	SHA     string `json:"sha" jsonschema:"commit SHA"`
	Message string `json:"message,omitempty" jsonschema:"commit subject"`
}

type tasksCloseInput struct {
	ProjectID string                  `json:"project_id" jsonschema:"project uuid"`
	TaskID    string                  `json:"task_id" jsonschema:"task uuid"`
	State     string                  `json:"state,omitempty" jsonschema:"'done' (default) or 'wont_do'"`
	Comment   string                  `json:"comment,omitempty" jsonschema:"optional closing comment (markdown body)"`
	Commits   []tasksCloseCommitInput `json:"commits,omitempty" jsonschema:"optional commit links to attach before the transition"`
	Verbose   bool                    `json:"verbose,omitempty" jsonschema:"full Task instead of slim ack"`
}

func registerTasks(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.list",
		Description: "Lists tasks. Slim rows + keyset pagination ({tasks, next_cursor, has_more}). Open-only by default; pass include_closed=true or an explicit state to see done/wont_do. See skill domains/tasks.md for the full reference.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksListInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		cycleID, err := resolveCycleFilter(ctx, d, pid, in.CycleID)
		if err != nil {
			return toolError(err.Error())
		}
		f := tasks.ListFilter{ProjectID: pid, IncludeChildren: in.IncludeChildren, CycleID: cycleID}
		if in.State != "" {
			f.State = tasks.State(in.State)
		} else if !in.IncludeClosed {
			f.OpenOnly = true
		}
		if in.Type != "" {
			f.Type = tasks.Type(in.Type)
		}
		if id := optUUID(in.AssigneeUserID); id != nil {
			f.AssigneeUserID = id
		}
		if id := optUUID(in.TargetRoleID); id != nil {
			f.TargetRoleID = id
		}
		if id := optUUID(in.ParentTaskID); id != nil {
			f.ParentTaskID = id
		}
		limit := in.Limit
		if limit <= 0 {
			proj, err := identity.GetProject(ctx, d.Pool, pid.String())
			if err != nil {
				return toolError("load project: " + err.Error())
			}
			limit = proj.MCPPageSize
		}
		cursor, err := tasks.DecodeCursor(in.Cursor)
		if err != nil {
			return toolError(err.Error())
		}
		page, err := tasks.ListPaginated(ctx, d.Pool, f, limit, cursor)
		if err != nil {
			return toolError(err.Error())
		}
		next, _ := tasks.EncodeCursor(page.NextCursor)
		return jsonResult(map[string]any{
			"tasks":       slimTaskList(page.Tasks, in.Verbose),
			"next_cursor": next,
			"has_more":    page.HasMore,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.get",
		Description: "Fetches a task with its description. include_deps / include_commits / include_comments opt in to the related collections (default off — they can be large).",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in taskRefInput) (*sdk.CallToolResult, any, error) {
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		t, err := tasks.Get(ctx, d.Pool, tid)
		if err != nil || t.ProjectID != pid {
			return toolError("task not found")
		}
		out := map[string]any{"task": t}
		if in.IncludeDeps {
			deps, _ := tasks.ListDependenciesOf(ctx, d.Pool, tid)
			out["depends_on"] = deps
		}
		if in.IncludeCommits {
			commits, _ := tasks.ListCommits(ctx, d.Pool, tid)
			out["commits"] = commits
		}
		if in.IncludeComments {
			comments, _ := tasks.ListComments(ctx, d.Pool, tid)
			out["comments"] = comments
		}
		return jsonResult(out)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.claim_next",
		Description: "Atomically picks the next eligible task and assigns it to the caller (state=doing). Multi-agent safe via SELECT ... FOR UPDATE SKIP LOCKED. Returns {task: null} when nothing is eligible. Slim shape by default.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksNextInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		cycleID, err := resolveCycleFilter(ctx, d, pid, in.CycleID)
		if err != nil {
			return toolError(err.Error())
		}
		f := tasks.NextFilter{ProjectID: pid, CycleID: cycleID}
		if id := optUUID(in.AssigneeUserID); id != nil {
			f.AssigneeUserID = id
			roles, _ := identity.UserRoleIDs(ctx, d.Pool, *id, pid)
			f.UserRoleIDs = roles
		}
		if id := optUUID(in.RoleID); id != nil {
			f.RoleID = id
			f.AssigneeUserID = nil
			f.UserRoleIDs = nil
		}
		t, err := tasks.ClaimNext(ctx, d.Pool, f, c.UserID)
		if errors.Is(err, tasks.ErrNotFound) {
			return jsonResult(map[string]any{"task": nil})
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"task": taskPayload(t, in.Verbose)})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.claim",
		Description: "Atomically claims a specific task by id (assignee=caller, state=doing). Returns the task on success or {error, reason, current_state, current_assignee_user_id, preconditions?, pending_children_count?} on conflict. Idempotent. Slim shape by default.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in taskRefInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		t, err := tasks.Claim(ctx, d.Pool, tid, c.UserID)
		if err != nil {
			var cerr *tasks.ClaimConflictError
			if errors.As(err, &cerr) {
				return jsonResult(map[string]any{
					"error":                    cerr.Error(),
					"reason":                   cerr.Reason,
					"task_id":                  cerr.TaskID,
					"current_state":            string(cerr.CurrentState),
					"current_assignee_user_id": cerr.CurrentAssigneeUserID,
					"preconditions":            cerr.Preconditions,
					"pending_children_count":   cerr.PendingChildrenCount,
				})
			}
			return toolError(err.Error())
		}
		return jsonResult(taskPayload(t, in.Verbose))
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.next",
		Description: "Preview only (no side effects): returns the next eligible task without claiming it. To take it, call tasks.claim_next (atomic). Returns {task: null} when nothing is eligible.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksNextInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		cycleID, err := resolveCycleFilter(ctx, d, pid, in.CycleID)
		if err != nil {
			return toolError(err.Error())
		}
		f := tasks.NextFilter{ProjectID: pid, CycleID: cycleID}
		if id := optUUID(in.AssigneeUserID); id != nil {
			f.AssigneeUserID = id
			roles, _ := identity.UserRoleIDs(ctx, d.Pool, *id, pid)
			f.UserRoleIDs = roles
		}
		if id := optUUID(in.RoleID); id != nil {
			f.RoleID = id
			f.AssigneeUserID = nil
			f.UserRoleIDs = nil
		}
		t, err := tasks.Next(ctx, d.Pool, f)
		if errors.Is(err, tasks.ErrNotFound) {
			return jsonResult(map[string]any{"task": nil})
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"task": taskPayload(t, in.Verbose)})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.create",
		Description: "Creates a task. Defaults: state=todo, type=task, priority=50. Slim shape by default — the description you sent is not echoed back.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksCreateInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		priority := in.Priority
		if priority == nil && in.PriorityKey != "" {
			v, err := identity.ResolvePriorityKey(ctx, d.Pool, pid, in.PriorityKey)
			if err != nil {
				return toolError("unknown priority_key '" + in.PriorityKey + "' (call nottario.projects.list_priorities to see available keys)")
			}
			priority = &v
		}
		if priority == nil {
			v, err := identity.DefaultPriorityValue(ctx, d.Pool, pid)
			if err == nil {
				priority = &v
			}
		}
		params := tasks.CreateParams{
			ProjectID:     pid,
			Title:         in.Title,
			DescriptionMD: in.Description,
			Type:          tasks.Type(in.Type),
			Priority:      priority,
		}
		if id := optUUID(in.AssigneeUserID); id != nil {
			params.AssigneeUserID = id
		}
		if id := optUUID(in.TargetRoleID); id != nil {
			params.TargetRoleID = id
		}
		if id := optUUID(in.ParentTaskID); id != nil {
			params.ParentTaskID = id
		}
		t, err := tasks.Create(ctx, d.Pool, params, authorshipFor(c))
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(taskPayload(t, in.Verbose))
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.update",
		Description: "Updates task fields. Pass '' in assignee_user_id / target_role_id to unset. Slim shape by default.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksUpdateInput) (*sdk.CallToolResult, any, error) {
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		var up tasks.UpdateParams
		up.Title = in.Title
		up.DescriptionMD = in.Description
		if in.Type != nil {
			tt := tasks.Type(*in.Type)
			up.Type = &tt
		}
		up.Priority = in.Priority
		if up.Priority == nil && in.PriorityKey != "" {
			v, err := identity.ResolvePriorityKey(ctx, d.Pool, pid, in.PriorityKey)
			if err != nil {
				return toolError("unknown priority_key '" + in.PriorityKey + "'")
			}
			up.Priority = &v
		}
		if in.AssigneeUserID != nil {
			if *in.AssigneeUserID == "" {
				up.UnsetAssignee = true
			} else {
				id, err := uuid.Parse(*in.AssigneeUserID)
				if err != nil {
					return toolError("assignee_user_id must be a uuid or empty string")
				}
				up.AssigneeUserID = &id
			}
		}
		if in.TargetRoleID != nil {
			if *in.TargetRoleID == "" {
				up.UnsetTargetRole = true
			} else {
				id, err := uuid.Parse(*in.TargetRoleID)
				if err != nil {
					return toolError("target_role_id must be a uuid or empty string")
				}
				up.TargetRoleID = &id
			}
		}
		t, err := tasks.Update(ctx, d.Pool, tid, up)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(taskPayload(t, in.Verbose))
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.set_state",
		Description: "Transitions between 'todo','doing','done','wont_do' and manages actual_start/end. done↔wont_do is refused (route via todo). On precondition failure returns {error, preconditions}. Slim shape by default.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksStateInput) (*sdk.CallToolResult, any, error) {
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		t, err := tasks.SetState(ctx, d.Pool, tid, tasks.State(in.State))
		if err != nil {
			var uerr *tasks.UnresolvedPreconditionsError
			if errors.As(err, &uerr) {
				return jsonResult(map[string]any{
					"error":         uerr.Error(),
					"preconditions": uerr.Preconditions,
				})
			}
			var terr *tasks.ErrInvalidStateTransition
			if errors.As(err, &terr) {
				return jsonResult(map[string]any{
					"error":      terr.Error(),
					"from_state": string(terr.From),
					"to_state":   string(terr.To),
				})
			}
			return toolError(err.Error())
		}
		return jsonResult(taskPayload(t, in.Verbose))
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.close",
		Description: "Atomic close: attaches commits, adds a closing comment and transitions state in one transaction. state defaults to 'done' ('wont_do' also accepted). On precondition failure rolls back the comment and links too. Slim ack {id, state, updated_at, comment_id?, linked_commit_count}; verbose=true returns the full Task.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksCloseInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		state := in.State
		if state == "" {
			state = string(tasks.StateDone)
		}
		commits := make([]tasks.CloseCommit, 0, len(in.Commits))
		for _, cm := range in.Commits {
			commits = append(commits, tasks.CloseCommit{Repo: cm.Repo, SHA: cm.SHA, Message: cm.Message})
		}
		res, err := tasks.Close(ctx, d.Pool, tid, tasks.CloseParams{
			State:   tasks.State(state),
			Comment: in.Comment,
			Commits: commits,
		}, authorshipFor(c))
		if err != nil {
			var uerr *tasks.UnresolvedPreconditionsError
			if errors.As(err, &uerr) {
				return jsonResult(map[string]any{
					"error":         uerr.Error(),
					"preconditions": uerr.Preconditions,
				})
			}
			var terr *tasks.ErrInvalidStateTransition
			if errors.As(err, &terr) {
				return jsonResult(map[string]any{
					"error":      terr.Error(),
					"from_state": string(terr.From),
					"to_state":   string(terr.To),
				})
			}
			return toolError(err.Error())
		}
		out := map[string]any{
			"task":                taskPayload(res.Task, in.Verbose),
			"comment_id":          res.CommentID,
			"linked_commit_count": res.LinkedCommit,
		}
		return jsonResult(out)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.add_dependency",
		Description: "Declares that task depends on depends_on. Cycle detection rejects edges that would form a loop.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksDepInput) (*sdk.CallToolResult, any, error) {
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		depID, err := uuid.Parse(in.DependsOnID)
		if err != nil {
			return toolError("depends_on_id must be a uuid")
		}
		if err := tasks.AddDependency(ctx, d.Pool, tid, depID); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.remove_dependency",
		Description: "Removes the dependency edge if present.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksDepInput) (*sdk.CallToolResult, any, error) {
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		depID, err := uuid.Parse(in.DependsOnID)
		if err != nil {
			return toolError("depends_on_id must be a uuid")
		}
		if err := tasks.RemoveDependency(ctx, d.Pool, tid, depID); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.link_commit",
		Description: "Attaches a git commit to a task. Repo must be in 'owner/repo' format.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksLinkCommitInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		if err := tasks.LinkCommit(ctx, d.Pool, tid, in.Repo, in.SHA, in.Message, authorshipFor(c)); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.add_comment",
		Description: "Appends a markdown comment to a task, attributed to the calling user/token. Slim shape ({id, task_id, created_at, updated_at}) by default — body not echoed.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksCommentInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		cm, err := tasks.AddComment(ctx, d.Pool, tid, in.Body, authorshipFor(c))
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(commentPayload(cm, in.Verbose))
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.inconsistencies",
		Description: "Lists tasks in inconsistent states. Each entry: {task_id, reason, details}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in struct {
		ProjectID string `json:"project_id" jsonschema:"project uuid"`
	}) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		items, err := tasks.ListInconsistencies(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"inconsistencies": items})
	})
}

// requireProjectAccess returns an error when the caller cannot see the project.
// It enforces, in this order: (1) per-token project scope — an API
// token bound to project A is rejected when the request targets
// project B; (2) instance-admin override; (3) project membership.
func requireProjectAccess(ctx context.Context, d Deps, projectID uuid.UUID) error {
	c, err := callerFromContext(ctx)
	if err != nil {
		return err
	}
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

func parseProjectAndTask(projectIDStr, taskIDStr string) (uuid.UUID, uuid.UUID, error) {
	pid, err := uuid.Parse(projectIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("project_id must be a uuid")
	}
	tid, err := uuid.Parse(taskIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("task_id must be a uuid")
	}
	return pid, tid, nil
}

func optUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// resolveCycleFilter turns the MCP-level cycle_id input into the
// *uuid.UUID the repo layer expects. Rules:
//   - empty/omitted: default to the project's active cycle (returns
//     ErrNoActiveCycle on invariant violations).
//   - "all": no filter (nil), every cycle considered.
//   - any other string: parsed as a uuid.
func resolveCycleFilter(ctx context.Context, d Deps, projectID uuid.UUID, in string) (*uuid.UUID, error) {
	if in == "all" {
		return nil, nil
	}
	if in == "" {
		active, err := cycles.ActiveCycle(ctx, d.Pool, projectID)
		if err != nil {
			return nil, err
		}
		id := active.ID
		return &id, nil
	}
	id, err := uuid.Parse(in)
	if err != nil {
		return nil, errors.New("cycle_id must be a uuid or 'all'")
	}
	return &id, nil
}

func authorshipFor(c identity.Caller) tasks.Authorship {
	a := tasks.Authorship{}
	uid := c.UserID
	a.UserID = &uid
	if c.Source == identity.SourceToken {
		tid := c.TokenID
		a.TokenID = &tid
	}
	return a
}
