package mcp

import (
	"context"
	"errors"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// Common input fragments. Every tool that targets a task carries an
// explicit project_id (caller-supplied — no per-session "active
// project") and a task_id.

type tasksListInput struct {
	ProjectID       string `json:"project_id" jsonschema:"uuid of the project (required)"`
	State           string `json:"state,omitempty" jsonschema:"optional filter: 'todo', 'doing' or 'done'"`
	Type            string `json:"type,omitempty" jsonschema:"optional filter: 'task','bug','chore','spike','feature'"`
	AssigneeUserID  string `json:"assignee_user_id,omitempty" jsonschema:"optional uuid of a user to filter assigned tasks"`
	TargetRoleID    string `json:"target_role_id,omitempty" jsonschema:"optional uuid of a role to filter target_role"`
	ParentTaskID    string `json:"parent_task_id,omitempty" jsonschema:"if set, list children of this feature task"`
	IncludeChildren bool   `json:"include_children,omitempty" jsonschema:"if true, include tasks that have a parent_task_id (default false: top-level only)"`
	Limit           int    `json:"limit,omitempty" jsonschema:"page size (1..500). When omitted, uses the project's configured mcp_page_size (default 50)."`
	Cursor          string `json:"cursor,omitempty" jsonschema:"opaque cursor returned by a previous call's next_cursor. Empty/omitted returns the first page."`
}

type taskRefInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project the task belongs to"`
	TaskID    string `json:"task_id" jsonschema:"uuid of the task"`
}

type tasksNextInput struct {
	ProjectID      string `json:"project_id" jsonschema:"uuid of the project"`
	AssigneeUserID string `json:"assignee_user_id,omitempty" jsonschema:"optional: restrict to tasks assigned to this user"`
	RoleID         string `json:"role_id,omitempty" jsonschema:"optional: restrict to tasks targeting this role"`
}

type tasksCreateInput struct {
	ProjectID      string `json:"project_id" jsonschema:"uuid of the project"`
	Title          string `json:"title" jsonschema:"short human-readable title"`
	Description    string `json:"description,omitempty" jsonschema:"longer markdown description"`
	Type           string `json:"type,omitempty" jsonschema:"'task' (default), 'bug', 'chore', 'spike' or 'feature'"`
	Priority       *int   `json:"priority,omitempty" jsonschema:"raw 0-100 value. Prefer priority_key over this — call projects.list_priorities to discover the project's buckets."`
	PriorityKey    string `json:"priority_key,omitempty" jsonschema:"named bucket from projects.list_priorities (e.g. 'low','medium','high','critical'). Resolved server-side to the bucket's numeric value. Takes precedence over priority only when priority is omitted."`
	AssigneeUserID string `json:"assignee_user_id,omitempty" jsonschema:"optional uuid of the user this task is assigned to"`
	TargetRoleID   string `json:"target_role_id,omitempty" jsonschema:"optional uuid of the role this task is targeted at"`
	ParentTaskID   string `json:"parent_task_id,omitempty" jsonschema:"optional uuid of a parent feature task this task is a child of"`
}

type tasksUpdateInput struct {
	ProjectID      string  `json:"project_id" jsonschema:"uuid of the project"`
	TaskID         string  `json:"task_id" jsonschema:"uuid of the task"`
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Type           *string `json:"type,omitempty"`
	Priority       *int    `json:"priority,omitempty" jsonschema:"raw 0-100 value. Prefer priority_key."`
	PriorityKey    string  `json:"priority_key,omitempty" jsonschema:"named bucket from projects.list_priorities; resolved server-side."`
	AssigneeUserID *string `json:"assignee_user_id,omitempty" jsonschema:"uuid to set, or empty string to unset"`
	TargetRoleID   *string `json:"target_role_id,omitempty" jsonschema:"uuid to set, or empty string to unset"`
}

type tasksStateInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
	TaskID    string `json:"task_id" jsonschema:"uuid of the task"`
	State     string `json:"state" jsonschema:"'todo', 'doing' or 'done'"`
}

type tasksDepInput struct {
	ProjectID   string `json:"project_id" jsonschema:"uuid of the project"`
	TaskID      string `json:"task_id" jsonschema:"uuid of the dependent task"`
	DependsOnID string `json:"depends_on_id" jsonschema:"uuid of the task that must be done first"`
}

type tasksLinkCommitInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
	TaskID    string `json:"task_id" jsonschema:"uuid of the task"`
	Repo      string `json:"repo" jsonschema:"'owner/repo' GitHub identifier"`
	SHA       string `json:"sha" jsonschema:"commit SHA, full or short"`
	Message   string `json:"message,omitempty" jsonschema:"optional commit subject for display"`
}

type tasksCommentInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
	TaskID    string `json:"task_id" jsonschema:"uuid of the task"`
	Body      string `json:"body" jsonschema:"markdown body of the comment"`
}

func registerTasks(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.list",
		Description: "Lists tasks in a project, optionally filtered by state, type, assignee, target role or parent feature task. Returns top-level tasks by default; pass include_children=true to flatten feature subtrees.\n\nPagination is keyset-based: each call returns at most `limit` tasks (defaults to the project's `mcp_page_size`, 50 unless an admin changed it), plus `next_cursor` and `has_more`. To walk the full backlog: call repeatedly passing the previous `next_cursor` until `has_more` is false. Filters can change between pages without corrupting the walk.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksListInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		f := tasks.ListFilter{ProjectID: pid, IncludeChildren: in.IncludeChildren}
		if in.State != "" {
			f.State = tasks.State(in.State)
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
			"tasks":       page.Tasks,
			"next_cursor": next,
			"has_more":    page.HasMore,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.get",
		Description: "Fetches a task with its dependencies, linked commits and comments.",
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
		deps, _ := tasks.ListDependenciesOf(ctx, d.Pool, tid)
		commits, _ := tasks.ListCommits(ctx, d.Pool, tid)
		comments, _ := tasks.ListComments(ctx, d.Pool, tid)
		return jsonResult(map[string]any{
			"task":       t,
			"depends_on": deps,
			"commits":    commits,
			"comments":   comments,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.claim_next",
		Description: "Atomically picks the highest-priority eligible task and CLAIMS it for the calling user (sets assignee = caller, state = doing, actual_start = now) — all in a single Postgres UPDATE backed by SELECT ... FOR UPDATE SKIP LOCKED so two agents picking at the same time get DIFFERENT tasks. This is the SAFE pickup primitive; the older next+update+set_state pattern has a race window. Returns {task: null} when nothing is eligible. Filters: same as tasks.next (assignee_user_id, role_id).",
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
		f := tasks.NextFilter{ProjectID: pid}
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
		return jsonResult(map[string]any{"task": t})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.claim",
		Description: "Atomically claims a SPECIFIC task by id for the calling user (assignee = caller, state = doing). Use this when you found a candidate via tasks.list and want to take it without racing other agents. Returns the task on success, or a {error, current_state, current_assignee_user_id, preconditions[]?, pending_children_count?, reason} payload when the task is not claimable. Idempotent if the caller already owns the task in 'doing' state.",
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
		return jsonResult(t)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.next",
		Description: "PREVIEW ONLY (no side effects). Returns the next eligible task as ranked by priority + dependency satisfaction, without claiming it. Useful for inspecting what tasks.claim_next would pick, or to surface to a human before committing. To actually take the task, call nottario.tasks.claim_next (atomic) — calling next + then update + then set_state in three separate steps is racy under multi-agent load. Returns {task: null} when nothing is eligible.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in tasksNextInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		f := tasks.NextFilter{ProjectID: pid}
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
		return jsonResult(map[string]any{"task": t})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.create",
		Description: "Creates a new task in the project. Defaults: state='todo', type='task', priority=50.",
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
		return jsonResult(t)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.update",
		Description: "Updates one or more fields of a task. Pass empty string in assignee_user_id or target_role_id to unset those.",
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
		return jsonResult(t)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.set_state",
		Description: "Transitions a task between 'todo', 'doing' and 'done'. Manages actual_start and actual_end timestamps automatically.",
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
			return toolError(err.Error())
		}
		return jsonResult(t)
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
		Description: "Appends a markdown comment to a task. The comment is attributed to the calling token and user.",
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
		return jsonResult(cm)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.inconsistencies",
		Description: "Lists tasks whose state is inconsistent with the rest of the project graph. Each entry has `task_id`, a stable `reason` key and a `details` payload. Initial reason: `dependent_already_done` — a non-done task that has at least one dependent already marked done.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in struct {
		ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
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
func requireProjectAccess(ctx context.Context, d Deps, projectID uuid.UUID) error {
	c, err := callerFromContext(ctx)
	if err != nil {
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
