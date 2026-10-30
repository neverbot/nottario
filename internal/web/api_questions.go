package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/markdown"
	"github.com/neverbot/nottario/internal/questions"
)

type questionTaskRef struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	ProjectID uuid.UUID `json:"project_id"`
}

type questionProjectRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type questionStepRef struct {
	ID   uuid.UUID `json:"id"`
	Text string    `json:"text"`
}

type questionSessionRef struct {
	ID     uuid.UUID `json:"id"`
	Label  string    `json:"label"`
	Key    string    `json:"key"`
	Client string    `json:"client"`
}

type questionUserRef struct {
	ID          uuid.UUID `json:"id"`
	Login       string    `json:"login"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
}

// questionView is a question as the web pages show it: the stored
// question plus what the page needs without further calls.
type questionView struct {
	questions.Question
	BodyHTML   string              `json:"body_html,omitempty"`
	Task       *questionTaskRef    `json:"task,omitempty"`
	Project    *questionProjectRef `json:"project,omitempty"`
	Step       *questionStepRef    `json:"step,omitempty"`
	Session    *questionSessionRef `json:"session,omitempty"`
	AskedBy    *questionUserRef    `json:"asked_by,omitempty"`
	AnsweredBy *questionUserRef    `json:"answered_by,omitempty"`
}

// buildQuestionViews decorates questions with their task, project,
// step, session and users, one batched query for each kind.
func buildQuestionViews(ctx context.Context, pool *pgxpool.Pool, qs []questions.Question) ([]questionView, error) {
	var taskIDs, projectIDs, stepIDs, sessionIDs, userIDs []uuid.UUID
	for _, q := range qs {
		taskIDs = append(taskIDs, q.TaskID)
		projectIDs = append(projectIDs, q.ProjectID)
		if q.StepID != nil {
			stepIDs = append(stepIDs, *q.StepID)
		}
		if q.SessionID != nil {
			sessionIDs = append(sessionIDs, *q.SessionID)
		}
		if q.AskedByUserID != nil {
			userIDs = append(userIDs, *q.AskedByUserID)
		}
		if q.AnsweredByUserID != nil {
			userIDs = append(userIDs, *q.AnsweredByUserID)
		}
	}
	out := make([]questionView, 0, len(qs))
	if len(qs) == 0 {
		return out, nil
	}
	db := dbq.New(pool)

	taskRows, err := db.GetTasksBrief(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	taskByID := make(map[uuid.UUID]questionTaskRef, len(taskRows))
	for _, r := range taskRows {
		taskByID[r.ID] = questionTaskRef{ID: r.ID, Title: r.Title, ProjectID: r.ProjectID}
	}
	projectRows, err := db.GetProjectsBrief(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	projectByID := make(map[uuid.UUID]questionProjectRef, len(projectRows))
	for _, r := range projectRows {
		projectByID[r.ID] = questionProjectRef{ID: r.ID, Name: r.Name, Slug: r.Slug}
	}
	stepByID := map[uuid.UUID]questionStepRef{}
	if len(stepIDs) > 0 {
		rows, err := db.GetStepsBrief(ctx, stepIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			stepByID[r.ID] = questionStepRef{ID: r.ID, Text: r.Text}
		}
	}
	sessions, err := agents.ByIDs(ctx, pool, sessionIDs)
	if err != nil {
		return nil, err
	}
	userByID := map[uuid.UUID]questionUserRef{}
	if len(userIDs) > 0 {
		rows, err := db.GetUsersBrief(ctx, userIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			userByID[r.ID] = questionUserRef{ID: r.ID, Login: r.GithubLogin, DisplayName: r.DisplayName, AvatarURL: r.AvatarUrl}
		}
	}

	for _, q := range qs {
		v := questionView{Question: q}
		if q.Body != "" {
			pid := q.ProjectID
			// A render failure ships the markdown without HTML.
			if html, err := markdown.Render(ctx, pool, q.Body, &pid); err == nil {
				v.BodyHTML = html
			}
		}
		if t, ok := taskByID[q.TaskID]; ok {
			v.Task = &t
		}
		if p, ok := projectByID[q.ProjectID]; ok {
			v.Project = &p
		}
		if q.StepID != nil {
			if s, ok := stepByID[*q.StepID]; ok {
				v.Step = &s
			}
		}
		if q.SessionID != nil {
			if s, ok := sessions[*q.SessionID]; ok {
				v.Session = &questionSessionRef{ID: s.ID, Label: s.Label, Key: s.Key, Client: s.Client}
			}
		}
		if q.AskedByUserID != nil {
			if u, ok := userByID[*q.AskedByUserID]; ok {
				v.AskedBy = &u
			}
		}
		if q.AnsweredByUserID != nil {
			if u, ok := userByID[*q.AnsweredByUserID]; ok {
				v.AnsweredBy = &u
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// visibleProjectIDs lists the projects whose questions the caller may
// see: every one they belong to (all of them for an admin), narrowed
// to the token's own project for an API token.
func (d TaskDeps) visibleProjectIDs(ctx context.Context, c identity.Caller) ([]uuid.UUID, error) {
	if c.Source == identity.SourceToken {
		if err := d.ensureProjectAccess(ctx, c, c.ProjectID); err != nil {
			return nil, err
		}
		return []uuid.UUID{c.ProjectID}, nil
	}
	projects, err := identity.ListProjects(ctx, d.Pool, c.UserID, c.IsAdmin)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

// ListQuestionsHandler serves GET /api/questions?project_id=&state=&limit=.
// Without project_id it covers every project the caller can see.
func ListQuestionsHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		f := questions.ListFilter{}
		if raw := r.URL.Query().Get("state"); raw != "" {
			st, err := questions.ParseState(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			f.State = st
		}
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			f.Limit = n
		}
		if raw := r.URL.Query().Get("project_id"); raw != "" {
			pid, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid project id")
				return
			}
			if err := d.ensureProjectAccess(r.Context(), c, pid); err != nil {
				writeProjectAccessError(w, err)
				return
			}
			f.ProjectIDs = []uuid.UUID{pid}
		} else {
			ids, err := d.visibleProjectIDs(r.Context(), c)
			if err != nil {
				writeProjectAccessError(w, err)
				return
			}
			f.ProjectIDs = ids
		}
		qs, err := questions.List(r.Context(), d.Pool, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		views, err := buildQuestionViews(r.Context(), d.Pool, qs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"questions": views})
	})
}

type answerQuestionRequest struct {
	Keys []string `json:"keys"`
	Text string   `json:"text"`
	Note string   `json:"note"`
}

// AnswerQuestionHandler records the caller's answer (POST) or replaces
// it while the asking agent has not taken it yet (PUT).
func AnswerQuestionHandler(d TaskDeps) http.Handler {
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
		qid, err := uuid.Parse(r.PathValue("question_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid question id")
			return
		}
		var req answerQuestionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		q, err := questions.Answer(r.Context(), d.Pool, pid, qid, c.UserID,
			questions.Reply{Keys: req.Keys, Text: req.Text, Note: req.Note})
		switch {
		case errors.Is(err, questions.ErrNotFound):
			writeError(w, http.StatusNotFound, "question not found")
			return
		case errors.Is(err, questions.ErrAlreadyDelivered):
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "already_delivered"})
			return
		case errors.Is(err, questions.ErrClosed):
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "withdrawn"})
			return
		case err != nil && questions.IsInvalid(err):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		views, err := buildQuestionViews(r.Context(), d.Pool, []questions.Question{*q})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"question": views[0]})
	})
}
