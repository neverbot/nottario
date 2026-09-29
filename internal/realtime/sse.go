package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// SSEHandler returns an http.Handler that streams events for one
// project via Server-Sent Events. The caller must be authenticated
// (session cookie or Bearer token) and have access to the project.
// maxStreamProjects caps how many projects one stream may follow.
// Each one costs a membership lookup when the stream opens; a browser
// with this many project tabs open is far past any real use.
const maxStreamProjects = 32

func SSEHandler(hub *Hub, pool *pgxpool.Pool, resolver *identity.Resolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := resolver.ResolveSession(r)
		if !ok {
			c, ok = resolver.ResolveToken(r)
		}
		if !ok {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		// project_id may repeat: one browser shares a single stream
		// across its tabs, and each tab can be on a different project.
		// Every id is checked; one the caller cannot see refuses the
		// whole stream rather than silently dropping it.
		raws := r.URL.Query()["project_id"]
		if len(raws) > maxStreamProjects {
			http.Error(w, "too many project_id values", http.StatusBadRequest)
			return
		}
		var pids []uuid.UUID
		for _, raw := range raws {
			if raw == "" {
				continue
			}
			parsed, err := uuid.Parse(raw)
			if err != nil {
				http.Error(w, "project_id malformed", http.StatusBadRequest)
				return
			}
			// An API token is bound to one project; it must not listen
			// to another one just because its owner belongs to both.
			if err := identity.RequireProjectScope(c, parsed); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if !c.IsAdmin {
				roles, err := identity.UserRoleIDs(r.Context(), pool, c.UserID, parsed)
				if err != nil {
					http.Error(w, "lookup failed", http.StatusInternalServerError)
					return
				}
				if len(roles) == 0 {
					http.Error(w, "not a project member", http.StatusForbidden)
					return
				}
			}
			pids = append(pids, parsed)
		}
		// Empty project_id = global-only subscription (banner, instance-wide
		// advisories). Auth is still required; membership check skipped.

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx-style buffering
		w.WriteHeader(http.StatusOK)
		// Initial comment line forces browsers to consider the
		// connection open before the first event arrives.
		fmt.Fprint(w, ": ok\n\n")
		flusher.Flush()

		events, cancel := hub.Subscribe(pids...)
		defer cancel()

		ctx := r.Context()
		// Keep-alive ticks so proxies and the EventSource client know
		// the connection is still live during quiet periods.
		ka := time.NewTicker(20 * time.Second)
		defer ka.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ka.C:
				if _, err := fmt.Fprint(w, ": ka\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case ev, ok := <-events:
				if !ok {
					return
				}
				if err := writeEvent(w, flusher, ev); err != nil {
					return
				}
			}
		}
	})
}

func writeEvent(w http.ResponseWriter, f http.Flusher, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	f.Flush()
	return nil
}
