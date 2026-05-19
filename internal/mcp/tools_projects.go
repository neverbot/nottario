package mcp

import (
	"context"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/identity"
)

// ListProjectsInput is empty: list returns the projects visible to
// the caller.
type ListProjectsInput struct{}

// GetProjectInput selects one project by uuid or slug.
type GetProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"the uuid or slug of the project to fetch"`
}

func registerProjects(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list",
		Description: "Lists projects visible to the caller. Admins see every project; other users see only the projects where they hold at least one role.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ ListProjectsInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		projects, err := identity.ListProjects(ctx, d.Pool, c.UserID, c.IsAdmin)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"projects": projects})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.get",
		Description: "Fetches a single project (by uuid or slug) including its repos, primary language, project type and slug. Call this to discover available metadata before working in the project.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in GetProjectInput) (*sdk.CallToolResult, any, error) {
		if in.ProjectID == "" {
			return toolError("project_id is required")
		}
		p, err := identity.GetProject(ctx, d.Pool, in.ProjectID)
		if err != nil {
			return toolError("project not found: " + err.Error())
		}
		return jsonResult(p)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list_priorities",
		Description: "Lists the project's priority buckets (named priorities). Each entry has a 'key' (e.g. 'low', 'high'), a numeric 'value' that tasks store, and a 'position' for sort order. Always call this before creating a task so you choose a key from the project's vocabulary rather than guessing a numeric value.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in GetProjectInput) (*sdk.CallToolResult, any, error) {
		if in.ProjectID == "" {
			return toolError("project_id is required")
		}
		pid, perr := uuid.Parse(in.ProjectID)
		if perr != nil {
			p, err := identity.GetProject(ctx, d.Pool, in.ProjectID)
			if err != nil {
				return toolError("project not found: " + err.Error())
			}
			pid = p.ID
		}
		pr, err := identity.ListPriorities(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"priorities": pr})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list_roles",
		Description: "Lists the role catalogue of a project (e.g. backend, frontend, qa, design). Tool callers use these role IDs to filter tasks by role or to set the target_role of newly-created tasks.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in GetProjectInput) (*sdk.CallToolResult, any, error) {
		if in.ProjectID == "" {
			return toolError("project_id is required")
		}
		pid, perr := uuid.Parse(in.ProjectID)
		if perr != nil {
			// Maybe a slug was passed; resolve through GetProject.
			p, err := identity.GetProject(ctx, d.Pool, in.ProjectID)
			if err != nil {
				return toolError("project not found: " + err.Error())
			}
			pid = p.ID
		}
		roles, err := identity.ListRoles(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"roles": roles})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.reorder_roles",
		Description: "Admin-only. Rewrites the top-to-bottom order of a project's roles. Pass the full ordered list of role ids; the position field is updated atomically. The Gantt view and the Roles settings page render in this order.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in ReorderRolesInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if !c.IsAdmin {
			return toolError("admin only")
		}
		pid, perr := uuid.Parse(in.ProjectID)
		if perr != nil {
			p, err := identity.GetProject(ctx, d.Pool, in.ProjectID)
			if err != nil {
				return toolError("project not found: " + err.Error())
			}
			pid = p.ID
		}
		ids := make([]uuid.UUID, 0, len(in.RoleIDs))
		for _, s := range in.RoleIDs {
			id, err := uuid.Parse(s)
			if err != nil {
				return toolError("invalid role id: " + s)
			}
			ids = append(ids, id)
		}
		if err := identity.MoveRole(ctx, d.Pool, pid, ids); err != nil {
			return toolError(err.Error())
		}
		roles, _ := identity.ListRoles(ctx, d.Pool, pid)
		return jsonResult(map[string]any{"roles": roles})
	})
}

// ReorderRolesInput is the input for nottario.projects.reorder_roles.
type ReorderRolesInput struct {
	ProjectID string   `json:"project_id" jsonschema:"uuid or slug of the project"`
	RoleIDs   []string `json:"role_ids" jsonschema:"role uuids in the desired top-to-bottom order"`
}
