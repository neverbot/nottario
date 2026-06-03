package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/identity"
)

// ArchDeps wires the architecture HTTP endpoints.
type ArchDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d ArchDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

func (d ArchDeps) requireAccess(ctx context.Context, c identity.Caller, projectID uuid.UUID) error {
	if err := identity.RequireProjectScope(c, projectID); err != nil {
		return err
	}
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

func (d ArchDeps) parseProject(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}

// ListKindsHandler returns the kind catalogue (seeding defaults on first use).
func ListKindsHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		kinds, err := arch.ListKinds(r.Context(), d.Pool, pid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"kinds": kinds})
	})
}

type kindUpsertRequest struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// UpsertKindHandler creates or updates a kind.
func UpsertKindHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		var req kindUpsertRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		k, err := arch.UpsertKind(r.Context(), d.Pool, pid, arch.Kind{
			Key: req.Key, Label: req.Label, Icon: req.Icon, Color: req.Color, Description: req.Description,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, k)
	})
}

// DeleteKindHandler removes a kind unless nodes still reference it.
func DeleteKindHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		key := r.PathValue("key")
		if err := arch.DeleteKind(r.Context(), d.Pool, pid, key); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, arch.ErrKindInUse) {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type nodeUpsertRequest struct {
	Slug        string         `json:"slug"`
	ParentSlug  string         `json:"parent_slug"`
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
	LinkedRepo  string         `json:"linked_repo"`
	LinkedPath  string         `json:"linked_path"`
	Position    *int           `json:"position"`
}

// UpsertNodeHandler creates or updates a node by slug.
func UpsertNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		var req nodeUpsertRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		n, err := arch.UpsertNode(r.Context(), d.Pool, pid, arch.UpsertParams{
			Slug: req.Slug, ParentSlug: req.ParentSlug, Kind: req.Kind, Name: req.Name,
			DescriptionMD: req.Description, Metadata: req.Metadata,
			LinkedRepo: req.LinkedRepo, LinkedPath: req.LinkedPath, Position: req.Position,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, n)
	})
}

// ListNodesHandler returns every node of the project (or only roots).
func ListNodesHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		q := r.URL.Query()
		nodes, err := arch.ListNodes(r.Context(), d.Pool, pid, q.Get("parent_slug"), q.Get("root_only") == "true")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
	})
}

// GetNodeHandler returns a node by slug, plus its links and incident edges.
func GetNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		slug := r.PathValue("slug")
		n, err := arch.GetNode(r.Context(), d.Pool, pid, slug)
		if errors.Is(err, arch.ErrNodeNotFound) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		children, _ := arch.ListNodes(r.Context(), d.Pool, pid, slug, false)
		edges, _ := arch.ListEdges(r.Context(), d.Pool, pid, arch.EdgeFilter{NodeSlug: slug})
		links, _ := arch.ListLinks(r.Context(), d.Pool, pid, slug)
		writeJSON(w, http.StatusOK, map[string]any{
			"node":     n,
			"children": children,
			"edges":    edges,
			"links":    links,
		})
	})
}

// RemoveNodeHandler deletes a node (with optional cascade).
func RemoveNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		slug := r.PathValue("slug")
		cascade := r.URL.Query().Get("cascade") == "true"
		if err := arch.RemoveNode(r.Context(), d.Pool, pid, slug, cascade); err != nil {
			if errors.Is(err, arch.ErrNodeNotFound) {
				writeError(w, http.StatusNotFound, "node not found")
				return
			}
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type moveNodeRequest struct {
	ParentSlug string `json:"parent_slug"`
}

// MoveNodeHandler reparents a node.
func MoveNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		slug := r.PathValue("slug")
		var req moveNodeRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		n, err := arch.MoveNode(r.Context(), d.Pool, pid, slug, req.ParentSlug)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, n)
	})
}

type edgeUpsertRequest struct {
	FromSlug    string `json:"from_slug"`
	ToSlug      string `json:"to_slug"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// UpsertEdgeHandler creates or updates an edge.
func UpsertEdgeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		var req edgeUpsertRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		e, err := arch.UpsertEdge(r.Context(), d.Pool, pid, arch.EdgeUpsertParams{
			FromSlug: req.FromSlug, ToSlug: req.ToSlug, Kind: req.Kind,
			Label: req.Label, DescriptionMD: req.Description,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, e)
	})
}

// ListEdgesHandler returns edges, optionally filtered by a node.
func ListEdgesHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		q := r.URL.Query()
		f := arch.EdgeFilter{
			NodeSlug:  q.Get("node_slug"),
			Direction: q.Get("direction"),
			Kind:      q.Get("kind"),
		}
		edges, err := arch.ListEdges(r.Context(), d.Pool, pid, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"edges": edges})
	})
}

// RemoveEdgeHandler deletes an edge by id.
func RemoveEdgeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		eid, err := uuid.Parse(r.PathValue("edge_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid edge id")
			return
		}
		if err := arch.RemoveEdge(r.Context(), d.Pool, pid, eid); err != nil {
			if errors.Is(err, arch.ErrEdgeNotFound) {
				writeError(w, http.StatusNotFound, "edge not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type linkRequest struct {
	DocPath string     `json:"doc_path"`
	TaskID  *uuid.UUID `json:"task_id"`
}

// LinkNodeHandler attaches a doc or task to a node.
func LinkNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		slug := r.PathValue("slug")
		var req linkRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		switch {
		case req.DocPath != "":
			if err := arch.LinkDoc(r.Context(), d.Pool, pid, slug, req.DocPath); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case req.TaskID != nil:
			if err := arch.LinkTask(r.Context(), d.Pool, pid, uuid.Nil, *req.TaskID, slug); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "either doc_path or task_id is required")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// UnlinkNodeHandler removes a previously-attached doc or task.
func UnlinkNodeHandler(d ArchDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := d.parseProject(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.requireAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		slug := r.PathValue("slug")
		var req linkRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		switch {
		case req.DocPath != "":
			if err := arch.UnlinkDoc(r.Context(), d.Pool, pid, slug, req.DocPath); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case req.TaskID != nil:
			if err := arch.UnlinkTask(r.Context(), d.Pool, pid, slug, *req.TaskID); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "either doc_path or task_id is required")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
