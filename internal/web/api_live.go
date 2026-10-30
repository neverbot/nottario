package web

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/live"
)

type liveTaskRef struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	ProjectID uuid.UUID `json:"project_id"`
}

type liveRoleRef struct {
	ID   uuid.UUID `json:"id"`
	Key  string    `json:"key"`
	Name string    `json:"name"`
}

type liveStepView struct {
	ID       uuid.UUID           `json:"id"`
	Position int                 `json:"position"`
	Text     string              `json:"text"`
	Session  *questionSessionRef `json:"session"`
}

type liveWorkingView struct {
	Task           liveTaskRef         `json:"task"`
	Role           *liveRoleRef        `json:"role"`
	Assignee       *questionUserRef    `json:"assignee"`
	StartedAt      *time.Time          `json:"started_at"`
	LastActivityAt time.Time           `json:"last_activity_at"`
	Idle           bool                `json:"idle"`
	Session        *questionSessionRef `json:"session"`
	Steps          []liveStepView      `json:"steps"`
	Progress       *live.Progress      `json:"progress"`
	QuestionsOpen  int                 `json:"questions_open"`
}

type liveFinishedView struct {
	Task     liveTaskRef      `json:"task"`
	State    string           `json:"state"`
	Assignee *questionUserRef `json:"assignee"`
	EndedAt  time.Time        `json:"ended_at"`
}

// LiveHandler serves GET /api/live?project_id=. Without project_id it
// covers every project the caller is a member of.
func LiveHandler(d TaskDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var projectIDs []uuid.UUID
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
			projectIDs = []uuid.UUID{pid}
		} else if c.Source == identity.SourceToken {
			if err := d.ensureProjectAccess(r.Context(), c, c.ProjectID); err != nil {
				writeProjectAccessError(w, err)
				return
			}
			projectIDs = []uuid.UUID{c.ProjectID}
		} else {
			ids, err := d.memberProjectIDs(r.Context(), c.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			projectIDs = ids
		}
		now := time.Now().UTC()
		board, err := live.Build(r.Context(), d.Pool, projectIDs, c.UserID, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		waiting, err := buildQuestionViews(r.Context(), d.Pool, c.UserID, board.Waiting)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Roles and assignees, one batched query each.
		var roleIDs, userIDs []uuid.UUID
		for _, it := range board.Working {
			if it.RoleID != nil {
				roleIDs = append(roleIDs, *it.RoleID)
			}
			if it.AssigneeUserID != nil {
				userIDs = append(userIDs, *it.AssigneeUserID)
			}
		}
		for _, f := range board.Finished {
			if f.AssigneeUserID != nil {
				userIDs = append(userIDs, *f.AssigneeUserID)
			}
		}
		db := dbq.New(d.Pool)
		roleByID := map[uuid.UUID]liveRoleRef{}
		if roleIDs = uniqueIDs(roleIDs); len(roleIDs) > 0 {
			rows, err := db.GetRolesBrief(r.Context(), roleIDs)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, row := range rows {
				roleByID[row.ID] = liveRoleRef{ID: row.ID, Key: row.Key, Name: row.Label}
			}
		}
		userByID := map[uuid.UUID]questionUserRef{}
		if userIDs = uniqueIDs(userIDs); len(userIDs) > 0 {
			rows, err := db.GetUsersBrief(r.Context(), userIDs)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, row := range rows {
				userByID[row.ID] = questionUserRef{ID: row.ID, Login: row.GithubLogin, DisplayName: row.DisplayName, AvatarURL: row.AvatarUrl}
			}
		}
		assignee := func(id *uuid.UUID) *questionUserRef {
			if id == nil {
				return nil
			}
			if u, ok := userByID[*id]; ok {
				return &u
			}
			return nil
		}

		working := make([]liveWorkingView, 0, len(board.Working))
		for _, it := range board.Working {
			v := liveWorkingView{
				Task:      liveTaskRef{ID: it.TaskID, Title: it.Title, Type: it.Type, ProjectID: it.ProjectID},
				Assignee:  assignee(it.AssigneeUserID),
				StartedAt: it.StartedAt, LastActivityAt: it.LastActivityAt, Idle: it.Idle,
				Progress: it.Progress, QuestionsOpen: it.QuestionsOpen,
				Steps: make([]liveStepView, 0, len(it.Steps)),
			}
			if it.RoleID != nil {
				if role, ok := roleByID[*it.RoleID]; ok {
					v.Role = &role
				}
			}
			if it.Session != nil {
				v.Session = &questionSessionRef{ID: it.Session.ID, Label: it.Session.Label, Key: it.Session.Key, Client: it.Session.Client}
			}
			for _, s := range it.Steps {
				sv := liveStepView{ID: s.ID, Position: s.Position, Text: s.Text}
				if s.Session != nil {
					sv.Session = &questionSessionRef{ID: s.Session.ID, Label: s.Session.Label, Key: s.Session.Key, Client: s.Session.Client}
				}
				v.Steps = append(v.Steps, sv)
			}
			working = append(working, v)
		}
		finished := make([]liveFinishedView, 0, len(board.Finished))
		for _, f := range board.Finished {
			finished = append(finished, liveFinishedView{
				Task:     liveTaskRef{ID: f.TaskID, Title: f.Title, Type: f.Type, ProjectID: f.ProjectID},
				State:    f.State,
				Assignee: assignee(f.AssigneeUserID),
				EndedAt:  f.EndedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"now":       now,
			"projects":  board.Projects,
			"waiting":   waiting,
			"working":   working,
			"finished":  finished,
			"truncated": board.Truncated,
		})
	})
}
