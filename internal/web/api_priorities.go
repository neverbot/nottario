package web

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/neverbot/nottario/internal/identity"
)

// ListPrioritiesHandler returns the priority buckets of a project.
func ListPrioritiesHandler(d ProjectDeps) http.Handler {
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
		pr, err := identity.ListPriorities(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"priorities": pr})
	})
}

type priorityRequest struct {
	Key      string `json:"key"`
	Value    int    `json:"value"`
	Position int    `json:"position"`
}

// UpsertPriorityHandler creates or updates a bucket. Admin-only.
func UpsertPriorityHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !c.IsAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		pid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		var req priorityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := identity.UpsertPriority(r.Context(), d.Pool, pid, req.Key, req.Value, req.Position)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

// RemovePriorityHandler deletes a bucket. Admin-only.
func RemovePriorityHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !c.IsAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		pid, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		key := r.PathValue("key")
		if err := identity.RemovePriority(r.Context(), d.Pool, pid, key); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
