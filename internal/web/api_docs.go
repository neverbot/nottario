package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/markdown"
)

// DocsDeps wires the docs HTTP endpoints.
type DocsDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
}

func (d DocsDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

func (d DocsDeps) authorship(c identity.Caller) docs.Authorship {
	a := docs.Authorship{}
	uid := c.UserID
	a.UserID = &uid
	if c.Source == identity.SourceToken {
		tid := c.TokenID
		a.TokenID = &tid
	}
	return a
}

// resolveScope reads the (scope, project_id) pair from the request,
// applies the access rules, and returns the canonical form. Admins
// see everything; non-admin callers see only projects they are
// members of, plus all global documents.
func (d DocsDeps) resolveScope(ctx context.Context, c identity.Caller, scopeStr, projectIDStr string) (docs.Scope, *uuid.UUID, error) {
	scope := docs.Scope(scopeStr)
	if scope == "" {
		scope = docs.ScopeProject
	}
	if !docs.ValidScope(scope) {
		return "", nil, errors.New("invalid scope")
	}
	if scope == docs.ScopeGlobal {
		return scope, nil, nil
	}
	if projectIDStr == "" {
		return "", nil, errors.New("project_id is required when scope=project")
	}
	pid, err := uuid.Parse(projectIDStr)
	if err != nil {
		return "", nil, errors.New("project_id must be a uuid")
	}
	if err := identity.RequireProjectScope(c, pid); err != nil {
		return "", nil, err
	}
	if !c.IsAdmin {
		roles, err := identity.UserRoleIDs(ctx, d.Pool, c.UserID, pid)
		if err != nil {
			return "", nil, err
		}
		if len(roles) == 0 {
			return "", nil, errors.New("not a project member")
		}
	}
	return scope, &pid, nil
}

// requireWriteGlobal limits write access to global docs to admins.
func requireWriteGlobal(c identity.Caller, scope docs.Scope) error {
	if scope == docs.ScopeGlobal && !c.IsAdmin {
		return errors.New("only admins can modify global documents")
	}
	return nil
}

// ListDocsHandler returns lightweight summaries.
func ListDocsHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		scope, pid, err := d.resolveScope(r.Context(), c, q.Get("scope"), q.Get("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		f := docs.ListFilter{
			Scope:      scope,
			ProjectID:  pid,
			PathPrefix: q.Get("path_prefix"),
			Kind:       docs.Kind(q.Get("kind")),
		}
		list, err := docs.List(r.Context(), d.Pool, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"documents": list})
	})
}

// ReadDocHandler fetches a single document.
func ReadDocHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		path := q.Get("path")
		if strings.TrimSpace(path) == "" {
			writeError(w, http.StatusBadRequest, "path is required")
			return
		}
		scope, pid, err := d.resolveScope(r.Context(), c, q.Get("scope"), q.Get("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		doc, err := docs.Read(r.Context(), d.Pool, scope, pid, path)
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Render markdown → HTML server-side so the docs reader can
		// drop the chrome in directly without a second round-trip.
		if html, rerr := markdown.Render(r.Context(), d.Pool, doc.ContentMD, doc.ProjectID); rerr == nil {
			doc.ContentHTML = html
		} else {
			log.Printf("api docs.read: markdown render failed for %q: %v", path, rerr)
		}
		writeJSON(w, http.StatusOK, doc)
	})
}

type writeDocRequest struct {
	Scope           string `json:"scope"`
	ProjectID       string `json:"project_id"`
	Path            string `json:"path"`
	Kind            string `json:"kind"`
	ContentMD       string `json:"content_md"`
	Message         string `json:"message"`
	ExpectedVersion *int   `json:"expected_version"`
}

// WriteDocHandler creates or updates a document.
func WriteDocHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var req writeDocRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scope, pid, err := d.resolveScope(r.Context(), c, req.Scope, req.ProjectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := requireWriteGlobal(c, scope); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if req.ExpectedVersion == nil {
			log.Printf("api docs.write: deprecated call without expected_version (user=%s path=%q scope=%s)", c.UserID, req.Path, scope)
		}
		doc, err := docs.Write(r.Context(), d.Pool, docs.WriteParams{
			Scope:           scope,
			ProjectID:       pid,
			Path:            req.Path,
			Kind:            docs.Kind(req.Kind),
			ContentMD:       req.ContentMD,
			Message:         req.Message,
			ExpectedVersion: req.ExpectedVersion,
		}, d.authorship(c))
		var vc *docs.VersionConflictError
		if errors.As(err, &vc) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":           "version_conflict",
				"current_version": vc.CurrentVersion,
				"message":         "re-read the document and retry with the latest current_version",
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, doc)
	})
}

type deleteDocRequest struct {
	Scope           string `json:"scope"`
	ProjectID       string `json:"project_id"`
	Path            string `json:"path"`
	Message         string `json:"message"`
	ExpectedVersion *int   `json:"expected_version"`
}

// DeleteDocHandler soft-deletes a document.
func DeleteDocHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var req deleteDocRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scope, pid, err := d.resolveScope(r.Context(), c, req.Scope, req.ProjectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := requireWriteGlobal(c, scope); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if req.ExpectedVersion == nil {
			log.Printf("api docs.delete: deprecated call without expected_version (user=%s path=%q scope=%s)", c.UserID, req.Path, scope)
		}
		err = docs.DeleteWithParams(r.Context(), d.Pool, docs.DeleteParams{
			Scope: scope, ProjectID: pid, Path: req.Path, Message: req.Message,
			ExpectedVersion: req.ExpectedVersion,
		}, d.authorship(c))
		var vc *docs.VersionConflictError
		if errors.As(err, &vc) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":           "version_conflict",
				"current_version": vc.CurrentVersion,
				"message":         "re-read the document and retry with the latest current_version",
			})
			return
		}
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// SearchDocsHandler runs a full-text search.
func SearchDocsHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		query := q.Get("q")
		scope, pid, err := d.resolveScope(r.Context(), c, q.Get("scope"), q.Get("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hits, err := docs.Search(r.Context(), d.Pool, query, docs.SearchFilter{
			Scope:     scope,
			ProjectID: pid,
			Kind:      docs.Kind(q.Get("kind")),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
	})
}

// ReadDocVersionHandler returns one historical version of a document.
// Used by the docs UI history popover to load a previous version
// read-only without rolling back the live document.
func ReadDocVersionHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		path := q.Get("path")
		if strings.TrimSpace(path) == "" {
			writeError(w, http.StatusBadRequest, "path is required")
			return
		}
		v, err := strconv.Atoi(q.Get("version"))
		if err != nil || v < 1 {
			writeError(w, http.StatusBadRequest, "version must be a positive integer")
			return
		}
		scope, pid, err := d.resolveScope(r.Context(), c, q.Get("scope"), q.Get("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		doc, err := docs.Read(r.Context(), d.Pool, scope, pid, path)
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ver, err := docs.ReadVersion(r.Context(), d.Pool, doc.ID, v)
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if html, rerr := markdown.Render(r.Context(), d.Pool, ver.ContentMD, doc.ProjectID); rerr == nil {
			ver.ContentHTML = html
		} else {
			log.Printf("api docs.read-version: markdown render failed for %q v%d: %v", path, v, rerr)
		}
		writeJSON(w, http.StatusOK, ver)
	})
}

// HistoryDocHandler lists historical versions of a document.
func HistoryDocHandler(d DocsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		q := r.URL.Query()
		path := q.Get("path")
		scope, pid, err := d.resolveScope(r.Context(), c, q.Get("scope"), q.Get("project_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		doc, err := docs.Read(r.Context(), d.Pool, scope, pid, path)
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		versions, err := docs.History(r.Context(), d.Pool, doc.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
	})
}
