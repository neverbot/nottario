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
		// Token-scoped callers only see memberships within their
		// bound project — the rest are out of scope by definition.
		if c.Source == identity.SourceToken {
			filtered := memberships[:0]
			for _, m := range memberships {
				if m.ProjectID == c.ProjectID {
					filtered = append(filtered, m)
				}
			}
			memberships = filtered
		}
		// Token never echoes back its own UUID. Tokens are credentials
		// and should never travel whole over any endpoint; the only
		// time the full token leaves the server is at issuance. The
		// caller already authenticated with the bearer it presented;
		// it does not need its own id repeated to do its job.
		return jsonResult(map[string]any{
			"user_id":      user.ID,
			"github_login": user.GithubLogin,
			"display_name": user.DisplayName,
			"is_admin":     user.IsAdmin,
			"source":       string(c.Source),
			"memberships":  memberships,
		})
	})
}
