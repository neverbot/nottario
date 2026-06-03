package web

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// TokenDeps wires the per-project token endpoints. Tokens are issued
// inside a project context and only ever authenticate against that
// project, so every endpoint sits under /api/projects/{project_id}/…
type TokenDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d TokenDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

// isProjectMember returns true if the caller is admin or has any role
// in the project. Used to gate per-project token endpoints; an
// outsider gets the same 404 the rest of the per-project surface
// returns so we don't leak project existence.
func (d TokenDeps) isProjectMember(r *http.Request, c identity.Caller, projectID uuid.UUID) (bool, error) {
	if c.IsAdmin {
		return true, nil
	}
	roles, err := identity.UserRoleIDs(r.Context(), d.Pool, c.UserID, projectID)
	if err != nil {
		return false, err
	}
	return len(roles) > 0, nil
}

// ListProjectTokensHandler returns every token issued for the
// project. Any member of the project can list; non-members get 404.
func ListProjectTokensHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if _, err := identity.GetProject(r.Context(), d.Pool, pid.String()); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		ok, err = d.isProjectMember(r, c, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "not a project member")
			return
		}
		tokens, err := identity.ListProjectTokens(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
	})
}

type issueTokenRequest struct {
	Name          string     `json:"name"`
	DefaultRoleID *uuid.UUID `json:"default_role_id"`
}

// IssueProjectTokenHandler mints a token for the caller inside the
// project URL-scope. Returns the plaintext exactly once. Caller must
// be a project member (or instance admin).
func IssueProjectTokenHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if _, err := identity.GetProject(r.Context(), d.Pool, pid.String()); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		ok, err = d.isProjectMember(r, c, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "not a project member")
			return
		}
		var req issueTokenRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		plaintext, token, err := identity.IssueToken(r.Context(), d.Pool, c.UserID, pid, req.Name, req.DefaultRoleID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"plaintext": plaintext,
			"token":     token,
		})
	})
}

// RevokeProjectTokenHandler revokes a token. Allowed when the caller
// is the token's owner, the project's owner, or an instance admin.
// Returns 404 when the token doesn't belong to the project in the
// URL — important so revocation can't reach across project scopes.
func RevokeProjectTokenHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		tid, err := uuid.Parse(r.PathValue("token_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid token id")
			return
		}
		proj, err := identity.GetProject(r.Context(), d.Pool, pid.String())
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		tok, err := identity.GetToken(r.Context(), d.Pool, tid)
		if err != nil {
			if errors.Is(err, identity.ErrTokenInvalid) {
				writeError(w, http.StatusNotFound, "token not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tok.ProjectID != pid {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		// Authz: owner of the token, owner of the project, or admin.
		if !c.IsAdmin && tok.UserID != c.UserID && proj.OwnerUserID != c.UserID {
			writeError(w, http.StatusForbidden, "not allowed")
			return
		}
		if err := identity.RevokeToken(r.Context(), d.Pool, tid, c.UserID, true /*allow*/); err != nil {
			if errors.Is(err, identity.ErrTokenInvalid) {
				writeError(w, http.StatusNotFound, "token not found")
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
