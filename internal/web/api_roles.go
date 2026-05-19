package web

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/neverbot/nottario/internal/identity"
)

// ListRolesHandler returns the role catalogue of a project.
func ListRolesHandler(d ProjectDeps) http.Handler {
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
		roles, err := identity.ListRoles(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
	})
}

type roleRequest struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// CreateRoleHandler adds a role to a project. Admin-only.
func CreateRoleHandler(d ProjectDeps) http.Handler {
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
		var req roleRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Key == "" || req.Label == "" {
			writeError(w, http.StatusBadRequest, "key and label are required")
			return
		}
		role, err := identity.CreateRole(r.Context(), d.Pool, pid, req.Key, req.Label, req.Color)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, role)
	})
}

// UpdateRoleHandler renames or recolours a role. Admin-only.
func UpdateRoleHandler(d ProjectDeps) http.Handler {
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
		rid, err := uuid.Parse(r.PathValue("role_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		var req roleRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		role, err := identity.UpdateRole(r.Context(), d.Pool, rid, req.Label, req.Color)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, role)
	})
}

// DeleteRoleHandler removes a role. Admin-only.
func DeleteRoleHandler(d ProjectDeps) http.Handler {
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
		rid, err := uuid.Parse(r.PathValue("role_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		if err := identity.DeleteRole(r.Context(), d.Pool, rid); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
