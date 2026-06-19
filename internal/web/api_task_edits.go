package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/neverbot/nottario/internal/tasks"
)

// commentIDFromPath parses the {comment_id} route parameter.
func commentIDFromPath(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("comment_id"))
}

type editTaskTextRequest struct {
	Title             *string    `json:"title"`
	Description       *string    `json:"description"`
	TargetRoleID      *uuid.UUID `json:"target_role_id"`
	UnsetTargetRole   bool       `json:"unset_target_role"`
	ExpectedUpdatedAt time.Time  `json:"expected_updated_at"`
}

// EditTaskTextHandler edits the title / description / target_role of a
// task. Title and description: any project member. target_role: admin
// only. Optimistic concurrency via expected_updated_at; stale → 409.
func EditTaskTextHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req editTaskTextRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if (req.TargetRoleID != nil || req.UnsetTargetRole) && !c.IsAdmin {
			writeError(w, http.StatusForbidden, "only admins can change role")
			return
		}
		t, err := tasks.UpdateText(r.Context(), d.Pool, tid, tasks.UpdateTextParams{
			Title:             req.Title,
			DescriptionMD:     req.Description,
			TargetRoleID:      req.TargetRoleID,
			UnsetTargetRole:   req.UnsetTargetRole,
			CallerUserID:      c.UserID,
			ExpectedUpdatedAt: req.ExpectedUpdatedAt,
		})
		if errors.Is(err, tasks.ErrConflict) {
			// Send the current row back so the UI can present the merge.
			current, _ := tasks.Get(r.Context(), d.Pool, tid)
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "stale",
				"current": current,
			})
			return
		}
		if errors.Is(err, tasks.ErrNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, t)
	})
}

type editCommentRequest struct {
	Body              string    `json:"body"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}

// EditCommentHandler edits a comment's body. Allowed for the comment's
// author OR a project admin. Optimistic concurrency via
// expected_updated_at.
func EditCommentHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		cid, err := commentIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid comment id")
			return
		}
		existing, err := tasks.GetComment(r.Context(), d.Pool, cid)
		if errors.Is(err, tasks.ErrNotFound) || (existing != nil && existing.TaskID != tid) {
			writeError(w, http.StatusNotFound, "comment not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !canModifyComment(c.UserID, c.IsAdmin, existing) {
			writeError(w, http.StatusForbidden, "only the author or an admin can edit this comment")
			return
		}
		var req editCommentRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cm, err := tasks.UpdateComment(r.Context(), d.Pool, cid, tasks.UpdateCommentParams{
			Body:              req.Body,
			CallerUserID:      c.UserID,
			ExpectedUpdatedAt: req.ExpectedUpdatedAt,
		})
		if errors.Is(err, tasks.ErrConflict) {
			current, _ := tasks.GetComment(r.Context(), d.Pool, cid)
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "stale",
				"current": current,
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cm)
	})
}

// DeleteCommentHandler removes a comment. Allowed for its author OR a
// project admin. Hard-delete; no tombstone.
func DeleteCommentHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		pid, err := projectIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
			writeProjectAccessError(w, err)
			return
		}
		tid, err := taskIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		cid, err := commentIDFromPath(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid comment id")
			return
		}
		existing, err := tasks.GetComment(r.Context(), d.Pool, cid)
		if errors.Is(err, tasks.ErrNotFound) || (existing != nil && existing.TaskID != tid) {
			writeError(w, http.StatusNotFound, "comment not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !canModifyComment(c.UserID, c.IsAdmin, existing) {
			writeError(w, http.StatusForbidden, "only the author or an admin can delete this comment")
			return
		}
		if err := tasks.DeleteComment(r.Context(), d.Pool, cid); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// canModifyComment encodes the "author OR admin" rule for edit + delete
// of a task comment. A nil author_user_id (very old MCP-only comment
// with no human author) is only modifiable by an admin.
func canModifyComment(callerID uuid.UUID, isAdmin bool, c *tasks.Comment) bool {
	if isAdmin {
		return true
	}
	if c == nil || c.AuthorUserID == nil {
		return false
	}
	return *c.AuthorUserID == callerID
}
