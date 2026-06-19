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
	ProjectID string `json:"project_id" jsonschema:"project uuid or slug"`
}

func registerProjects(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list",
		Description: "Lists projects visible to the caller. Token callers see only the bound project.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ ListProjectsInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		projects, err := identity.ListProjects(ctx, d.Pool, c.UserID, c.IsAdmin)
		if err != nil {
			return toolError(err.Error())
		}
		// Token-scoped callers see only their bound project, even if
		// the underlying user is a member of others.
		if c.Source == identity.SourceToken {
			filtered := projects[:0]
			for _, p := range projects {
				if p.ID == c.ProjectID {
					filtered = append(filtered, p)
				}
			}
			projects = filtered
		}
		return jsonResult(map[string]any{"projects": projects})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.get",
		Description: "Fetches one project (uuid or slug) with its repos, language, type and slug.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in GetProjectInput) (*sdk.CallToolResult, any, error) {
		if in.ProjectID == "" {
			return toolError("project_id is required")
		}
		p, err := identity.GetProject(ctx, d.Pool, in.ProjectID)
		if err != nil {
			return toolError("project not found: " + err.Error())
		}
		if err := enforceProjectScopeMCP(ctx, p.ID); err != nil {
			return toolError(err.Error())
		}
		return jsonResult(p)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list_priorities",
		Description: "Lists priority buckets: {key, value, position}. Call before creating tasks.",
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
		if err := enforceProjectScopeMCP(ctx, pid); err != nil {
			return toolError(err.Error())
		}
		pr, err := identity.ListPriorities(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"priorities": pr})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.list_roles",
		Description: "Lists the role catalogue of a project (backend, frontend, …).",
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
		if err := enforceProjectScopeMCP(ctx, pid); err != nil {
			return toolError(err.Error())
		}
		roles, err := identity.ListRoles(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"roles": roles})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.reorder_roles",
		Description: "Admin only. Atomically rewrites role order; pass the full ordered list of role ids.",
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
		if err := identity.RequireProjectScope(c, pid); err != nil {
			return toolError(err.Error())
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

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.projects.set_owner",
		Description: "Admin only. Reassigns the project owner.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in SetOwnerInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if !c.IsAdmin {
			return toolError("admin only")
		}
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
		if err := identity.RequireProjectScope(c, pid); err != nil {
			return toolError(err.Error())
		}
		newOwnerID, err := uuid.Parse(in.NewOwnerID)
		if err != nil {
			return toolError("new_owner_id must be a uuid")
		}
		if err := identity.SetProjectOwner(ctx, d.Pool, pid, newOwnerID); err != nil {
			return toolError(err.Error())
		}
		p, err := identity.GetProject(ctx, d.Pool, pid.String())
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(p)
	})
}

// enforceProjectScopeMCP returns a ProjectScopeError when the caller
// authenticated with an API token bound to a different project.
func enforceProjectScopeMCP(ctx context.Context, projectID uuid.UUID) error {
	c, err := callerFromContext(ctx)
	if err != nil {
		return err
	}
	return identity.RequireProjectScope(c, projectID)
}

// ReorderRolesInput is the input for nottario.projects.reorder_roles.
type ReorderRolesInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project uuid or slug"`
	RoleIDs   []string `json:"role_ids" jsonschema:"role uuids in desired order"`
}

// SetOwnerInput is the input for nottario.projects.set_owner.
type SetOwnerInput struct {
	ProjectID  string `json:"project_id" jsonschema:"project uuid or slug"`
	NewOwnerID string `json:"new_owner_id" jsonschema:"new owner user uuid"`
}
