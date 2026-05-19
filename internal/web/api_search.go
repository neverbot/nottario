package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/search"
)

// SearchDeps wires the cross-cutting search endpoint.
type SearchDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d SearchDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

func (d SearchDeps) requireAccess(ctx context.Context, c identity.Caller, projectID uuid.UUID) error {
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

// SearchHandler runs the unified FTS query.
func SearchHandler(d SearchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		query := q.Get("q")
		pidStr := q.Get("project_id")
		if pidStr == "" {
			writeError(w, http.StatusBadRequest, "project_id is required")
			return
		}
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "project_id must be a uuid")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}

		var kinds []search.Kind
		for _, k := range strings.Split(q.Get("kinds"), ",") {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			kinds = append(kinds, search.Kind(k))
		}

		hits, err := search.Search(r.Context(), d.Pool, query, search.Filter{
			ProjectID: pid,
			Kinds:     kinds,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
	})
}
