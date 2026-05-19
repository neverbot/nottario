package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/identity"
)

// WhoamiInput is intentionally empty: whoami takes no arguments.
type WhoamiInput struct{}

func registerWhoami(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.whoami",
		Description: "Returns information about the authenticated agent: the underlying user, admin status, how authentication was resolved, and every (project, role) tuple the user belongs to. Always call this first to confirm credentials and discover which roles you can filter tasks by, in which projects.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ WhoamiInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		user, err := identity.GetUser(ctx, d.Pool, c.UserID)
		if err != nil {
			return toolError("user not found: " + err.Error())
		}
		memberships, err := identity.ListUserMemberships(ctx, d.Pool, user.ID)
		if err != nil {
			return toolError("list memberships: " + err.Error())
		}
		return jsonResult(map[string]any{
			"user_id":      user.ID,
			"github_login": user.GithubLogin,
			"display_name": user.DisplayName,
			"is_admin":     user.IsAdmin,
			"source":       string(c.Source),
			"token_id":     c.TokenID,
			"memberships":  memberships,
		})
	})
}
