package web

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// UsersDeps wires the users directory endpoint.
type UsersDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

// ListUsersHandler returns every user on the instance with a
// project_count derived from memberships. Visible to any
// authenticated caller (sessions or tokens).
func ListUsersHandler(d UsersDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := d.Resolver.ResolveSession(r); !ok {
			if _, ok2 := d.Resolver.ResolveToken(r); !ok2 {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
		}
		users, err := identity.ListAllUsersPublic(r.Context(), d.Pool)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
	})
}
