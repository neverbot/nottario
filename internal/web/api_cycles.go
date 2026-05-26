package web

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
)

// ListCyclesHandler returns every cycle of a project (newest first).
func ListCyclesHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		out, err := cycles.List(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cycles": out})
	})
}

// GetCurrentCycleHandler returns the project's active (open) cycle.
func GetCurrentCycleHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		c, err := cycles.ActiveCycle(r.Context(), d.Pool, pid)
		if err != nil {
			if errors.Is(err, cycles.ErrNoActiveCycle) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, c)
	})
}

type endCycleRequest struct {
	NextName string `json:"next_name"`
}

// EndCycleHandler closes the project's active cycle and opens the next
// one. Gated to project owner / instance admin.
func EndCycleHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := identity.RequireProjectOwner(r.Context(), d.Pool, pid, c.UserID, c.IsAdmin); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		var req endCycleRequest
		// Body is optional — ignore decode errors on empty body.
		if r.ContentLength > 0 {
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		by := cycles.Authorship{UserID: &c.UserID}
		if c.Source == identity.SourceToken {
			tid := c.TokenID
			by.TokenID = &tid
		}
		res, err := cycles.EndCycle(r.Context(), d.Pool, cycles.EndCycleParams{
			ProjectID: pid,
			NextName:  req.NextName,
		}, by)
		if err != nil {
			if errors.Is(err, cycles.ErrNoActiveCycle) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}
