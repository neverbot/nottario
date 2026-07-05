package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// AuthDeps wires the auth handlers.
type AuthDeps struct {
	Pool        *pgxpool.Pool
	Resolver    *identity.Resolver
	OAuthConfig identity.OAuthConfig
}

// GithubStartHandler kicks off the OAuth flow.
func GithubStartHandler(deps AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity.BeginGithubAuth(w, r, deps.OAuthConfig)
	})
}

// GithubCallbackHandler completes the OAuth flow and redirects home.
func GithubCallbackHandler(deps AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := identity.HandleGithubCallback(w, r, deps.Pool, deps.OAuthConfig); err != nil {
			if errors.Is(err, identity.ErrOrgRequired) {
				q := url.Values{"error": {"org_required"}, "org": {deps.OAuthConfig.RequiredOrg}}
				http.Redirect(w, r, "/login?"+q.Encode(), http.StatusFound)
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

// LogoutHandler clears the session and redirects home.
func LogoutHandler(deps AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := deps.Resolver.ResolveSession(r)
		if ok {
			_ = identity.DeleteSession(r.Context(), deps.Pool, c.SessionID)
		}
		identity.ClearSessionCookie(w, deps.OAuthConfig.CookieSecure)
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

// MeTokensHandler returns every API token the caller has issued,
// across every project. Populates the cross-project tokens table on
// `/me`. 401 anonymous; no admin bypass — this is a personal view.
func MeTokensHandler(deps AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := deps.Resolver.ResolveSession(r)
		if !ok {
			if c2, ok2 := deps.Resolver.ResolveToken(r); ok2 {
				c = c2
				ok = true
			}
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		rows, err := identity.ListUserTokens(r.Context(), deps.Pool, c.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": rows})
	})
}

// MeHandler returns the current caller, or 401 if anonymous.
func MeHandler(deps AuthDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := deps.Resolver.ResolveSession(r)
		if !ok {
			if c2, ok2 := deps.Resolver.ResolveToken(r); ok2 {
				c = c2
				ok = true
			}
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		user, err := identity.GetUser(r.Context(), deps.Pool, c.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user lookup failed")
			return
		}
		// Memberships are best-effort: the profile page needs them but a
		// transient db error shouldn't blank out the rest of the response.
		memberships, _ := identity.ListUserMemberships(r.Context(), deps.Pool, user.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":           user.ID,
			"github_login": user.GithubLogin,
			"display_name": user.DisplayName,
			"avatar_url":   user.AvatarURL,
			"is_admin":     user.IsAdmin,
			"created_at":   user.CreatedAt,
			"source":       string(c.Source),
			"memberships":  memberships,
		})
	})
}
