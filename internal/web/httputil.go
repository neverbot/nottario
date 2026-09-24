package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/identity"
)

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeProjectAccessError translates an access-check failure into the
// appropriate HTTP response. A ProjectScopeError surfaces as 403 with
// the explicit token/target message so agents debugging a cross-project
// call get a precise hint; everything else is collapsed to 404 to
// avoid leaking project existence to outsiders.
func writeProjectAccessError(w http.ResponseWriter, err error) {
	var pse *identity.ProjectScopeError
	if errors.As(err, &pse) {
		writeError(w, http.StatusForbidden, pse.Error())
		return
	}
	writeError(w, http.StatusNotFound, "project not found")
}

// withProjectSlug rewrites the project segment of the path to the
// canonical uuid when it carries a slug, so every downstream handler
// keeps parsing a uuid and nothing has to learn about slugs.
//
// It has to run BEFORE withProjectScopeGuard: that guard skips its
// check when the segment is not a uuid, so resolving afterwards would
// let an API token reach another project simply by naming its slug.
//
// An unknown slug is passed through untouched rather than answered
// here. Replying 404 before authentication would tell an anonymous
// caller which project slugs exist; left alone, the request ends in
// the same 400 or 401 it gets today.
func withProjectSlug(pool *pgxpool.Pool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "id"
		raw := r.PathValue(key)
		if raw == "" {
			key = "project_id"
			raw = r.PathValue(key)
		}
		if raw == "" {
			h.ServeHTTP(w, r)
			return
		}
		if _, err := uuid.Parse(raw); err == nil {
			h.ServeHTTP(w, r)
			return
		}
		p, err := identity.GetProject(r.Context(), pool, raw)
		if err != nil {
			h.ServeHTTP(w, r)
			return
		}
		r.SetPathValue(key, p.ID.String())
		h.ServeHTTP(w, r)
	})
}

// withProjectScopeGuard wraps a per-project HTTP handler. It resolves
// the caller, extracts the project uuid from the path (trying "id"
// then "project_id" — both forms appear across the API surface), and
// enforces the per-token project boundary BEFORE the wrapped handler
// runs. Membership checks remain inside the wrapped handlers.
// Sessions, missing tokens and admin tokens pass through to the
// handler as usual; the wrapper does NOT short-circuit unauthenticated
// requests — each handler keeps its own 401 semantics.
func withProjectScopeGuard(resolver *identity.Resolver, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := resolver.ResolveToken(r)
		if !ok || c.Source != identity.SourceToken {
			h.ServeHTTP(w, r)
			return
		}
		pidStr := r.PathValue("id")
		if pidStr == "" {
			pidStr = r.PathValue("project_id")
		}
		if pidStr == "" {
			h.ServeHTTP(w, r)
			return
		}
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			h.ServeHTTP(w, r)
			return
		}
		if err := identity.RequireProjectScope(c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
