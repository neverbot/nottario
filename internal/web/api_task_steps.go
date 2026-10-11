package web

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// stepTarget is what every step route resolves before acting: who is
// calling and which task (and step) of which project.
type stepTarget struct {
	caller identity.Caller
	taskID uuid.UUID
	stepID uuid.UUID
}

// resolveStepTarget authenticates the caller, checks project access
// and that the task belongs to the project, and parses {step_id} when
// withStep is set. On failure it has written the response.
func (d TaskDeps) resolveStepTarget(w http.ResponseWriter, r *http.Request, withStep bool) (stepTarget, bool) {
	var out stepTarget
	c, ok := d.caller(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return out, false
	}
	pid, err := projectIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return out, false
	}
	if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
		writeProjectAccessError(w, err)
		return out, false
	}
	tid, err := taskIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return out, false
	}
	if err := tasks.RequireInProject(r.Context(), d.Pool, pid, tid); err != nil {
		writeStepError(w, err)
		return out, false
	}
	out.caller, out.taskID = c, tid
	if withStep {
		sid, err := uuid.Parse(r.PathValue("step_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid step id")
			return out, false
		}
		out.stepID = sid
	}
	return out, true
}

func writeStepError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	case errors.Is(err, tasks.ErrStepNotFound):
		writeError(w, http.StatusNotFound, "step not found")
	case tasks.IsInvalid(err):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

type addStepsRequest struct {
	Texts []string `json:"texts"`
}

// AddStepsHandler appends steps to a task and answers with the whole
// list.
func AddStepsHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at, ok := d.resolveStepTarget(w, r, false)
		if !ok {
			return
		}
		var req addStepsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		steps, err := tasks.AddSteps(r.Context(), d.Pool, at.taskID, req.Texts, d.authorship(at.caller))
		if err != nil {
			writeStepError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"steps": steps})
	})
}

type updateStepRequest struct {
	Text *string `json:"text"`
	Done *bool   `json:"done"`
}

// UpdateStepHandler edits a step's text, ticks or unticks it.
func UpdateStepHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at, ok := d.resolveStepTarget(w, r, true)
		if !ok {
			return
		}
		var req updateStepRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s, err := tasks.UpdateStep(r.Context(), d.Pool, at.taskID, at.stepID,
			tasks.UpdateStepParams{Text: req.Text, Done: req.Done}, d.authorship(at.caller))
		if err != nil {
			writeStepError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s)
	})
}

// RemoveStepHandler deletes a step.
func RemoveStepHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at, ok := d.resolveStepTarget(w, r, true)
		if !ok {
			return
		}
		if err := tasks.RemoveStep(r.Context(), d.Pool, at.taskID, at.stepID); err != nil {
			writeStepError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type moveStepRequest struct {
	Position *int `json:"position"`
}

// MoveStepHandler puts a step at a new position and answers with the
// whole list.
func MoveStepHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at, ok := d.resolveStepTarget(w, r, true)
		if !ok {
			return
		}
		var req moveStepRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Position == nil {
			writeError(w, http.StatusBadRequest, "position is required")
			return
		}
		steps, err := tasks.MoveStep(r.Context(), d.Pool, at.taskID, at.stepID, *req.Position)
		if err != nil {
			writeStepError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"steps": steps})
	})
}
