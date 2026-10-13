package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/usage"
)

// mcpUsageView is a project's MCP usage as the settings page shows it.
// Per-tool totals cover every token; per-token rows only the caller's
// own tokens (all of them for an admin), the same rule as the token
// list, with the rest summed under others.
type mcpUsageView struct {
	*usage.Summary
	Others *usage.Counters `json:"others,omitempty"`
}

// MCPUsageHandler serves GET /api/projects/{project_id}/mcp-usage?days=N
// (1..180, default 30).
func MCPUsageHandler(d TokenDeps) http.Handler {
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
			writeProjectAccessError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "not a project member")
			return
		}
		days := 30
		if s := r.URL.Query().Get("days"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 180 {
				writeError(w, http.StatusBadRequest, "days must be between 1 and 180")
				return
			}
			days = n
		}
		sum, err := usage.Summarise(r.Context(), d.Pool, pid, days, time.Now())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view := mcpUsageView{Summary: sum}
		if !c.IsAdmin {
			mine := sum.Tokens[:0:0]
			var others usage.Counters
			for _, t := range sum.Tokens {
				if t.UserID == c.UserID {
					mine = append(mine, t)
					continue
				}
				others.Calls += t.Calls
				others.Errors += t.Errors
				others.BytesIn += t.BytesIn
				others.BytesOut += t.BytesOut
			}
			sum.Tokens = mine
			if others.Calls > 0 {
				view.Others = &others
			}
		}
		writeJSON(w, http.StatusOK, view)
	})
}
