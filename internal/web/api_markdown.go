package web

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/markdown"
)

// MarkdownDeps wires the generic markdown rendering endpoint.
type MarkdownDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

type renderMarkdownRequest struct {
	ProjectID string `json:"project_id"`
	Content   string `json:"content"`
}

// RenderMarkdownHandler returns rendered HTML for arbitrary markdown.
// Used by the web UI for surfaces where the markdown is not already
// bundled with a rendered copy (task descriptions, comments, arch
// node descriptions, …). The docs reader does NOT call this — its
// HTML ships inside the doc payload itself.
//
// Authenticated via session cookie or Bearer token, like every other
// /api endpoint. project_id is optional but recommended: without it
// cross-domain link chips ([[task:N]] etc.) cannot resolve and render
// as inert "no project context" spans.
func RenderMarkdownHandler(d MarkdownDeps) http.Handler {
	caller := func(r *http.Request) (identity.Caller, bool) {
		if c, ok := d.Resolver.ResolveSession(r); ok {
			return c, true
		}
		return d.Resolver.ResolveToken(r)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := caller(r); !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var req renderMarkdownRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var pid *uuid.UUID
		if req.ProjectID != "" {
			parsed, err := uuid.Parse(req.ProjectID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "project_id is not a valid uuid")
				return
			}
			pid = &parsed
		}
		html, err := markdown.Render(r.Context(), d.Pool, req.Content, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"html": html})
	})
}
