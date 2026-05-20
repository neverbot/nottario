package web

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
)

// ProjectDeps wires project endpoints.
type ProjectDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d ProjectDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

// ListProjectsHandler returns the projects visible to the caller.
func ListProjectsHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		projects, err := identity.ListProjects(r.Context(), d.Pool, c.UserID, c.IsAdmin)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
	})
}

type createProjectRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PrimaryLanguage string   `json:"primary_language"`
	ProjectType     string   `json:"project_type"`
	Repos           []string `json:"repos"`
}

// CreateProjectHandler creates a project. Admin-only.
func CreateProjectHandler(d ProjectDeps) http.Handler {
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
		var req createProjectRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		p, err := identity.CreateProject(r.Context(), d.Pool, req.Name, req.Description, req.PrimaryLanguage, req.ProjectType, c.UserID, req.Repos)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	})
}

// GetProjectHandler returns a single project.
func GetProjectHandler(d ProjectDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		id := r.PathValue("id")
		p, err := identity.GetProject(r.Context(), d.Pool, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

type updateProjectRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PrimaryLanguage string   `json:"primary_language"`
	ProjectType     string   `json:"project_type"`
	Repos           []string `json:"repos"`
}

// UpdateProjectHandler edits a project. Admin-only.
func UpdateProjectHandler(d ProjectDeps) http.Handler {
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
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		var req updateProjectRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := identity.UpdateProject(r.Context(), d.Pool, id, req.Name, req.Description, req.PrimaryLanguage, req.ProjectType, req.Repos)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

// UpdateProjectMCPHandler edits the MCP-related settings (today just
// the per-project pagination page size). Admin-only.
func UpdateProjectMCPHandler(d ProjectDeps) http.Handler {
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
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		var body struct {
			MCPPageSize int `json:"mcp_page_size"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := identity.UpdateProjectMCPPageSize(r.Context(), d.Pool, id, body.MCPPageSize)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

// DeleteProjectHandler removes a project. Admin-only.
func DeleteProjectHandler(d ProjectDeps) http.Handler {
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
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := identity.DeleteProject(r.Context(), d.Pool, id); err != nil {
			if errors.Is(err, errors.New("not found")) {
				writeError(w, http.StatusNotFound, "project not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
