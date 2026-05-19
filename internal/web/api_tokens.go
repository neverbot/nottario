package web

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// TokenDeps wires the token endpoints.
type TokenDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d TokenDeps) caller(r *http.Request) (identity.Caller, bool) {
	return d.Resolver.ResolveSession(r)
}

// ListTokensHandler returns the caller's own tokens.
func ListTokensHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		tokens, err := identity.ListTokens(r.Context(), d.Pool, c.UserID)
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

// IssueTokenHandler creates a new token and returns the plaintext
// exactly once.
func IssueTokenHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
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
		plaintext, token, err := identity.IssueToken(r.Context(), d.Pool, c.UserID, req.Name, req.DefaultRoleID)
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

// RevokeTokenHandler revokes a token. Owner or admin only.
func RevokeTokenHandler(d TokenDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid token id")
			return
		}
		if err := identity.RevokeToken(r.Context(), d.Pool, id, c.UserID, c.IsAdmin); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
