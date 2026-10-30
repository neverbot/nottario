package web

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/markdown"
	"github.com/neverbot/nottario/internal/questions"
)

type questionTaskRef struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
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

// questionView is a question as the web pages show it. Ids of the
// task, project, step, session and users live in the nested refs only;
// the raw body is replaced by its rendered HTML and a short excerpt,
// and the API token behind an MCP question stays server side (ViaMCP).
type questionView struct {
	ID                uuid.UUID           `json:"id"`
	Kind              questions.Kind      `json:"kind"`
	State             questions.State     `json:"state"`
	Excerpt           string              `json:"excerpt"`
	BodyHTML          string              `json:"body_html,omitempty"`
	Options           []questions.Option  `json:"options,omitempty"`
	Multi             bool                `json:"multi,omitempty"`
	RecommendedKey    string              `json:"recommended_key,omitempty"`
	RecommendedReason string              `json:"recommended_reason,omitempty"`
	Answer            *questions.Reply    `json:"answer,omitempty"`
	AnswerTextHTML    string              `json:"answer_text_html,omitempty"`
	AnswerNoteHTML    string              `json:"answer_note_html,omitempty"`
	AnsweredAt        *time.Time          `json:"answered_at,omitempty"`
	DeliveredAt       *time.Time          `json:"delivered_at,omitempty"`
	WithdrawnAt       *time.Time          `json:"withdrawn_at,omitempty"`
	WithdrawnReason   string              `json:"withdrawn_reason,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	ViaMCP            bool                `json:"via_mcp"`
	ForMe             bool                `json:"for_me"`
	Task              *questionTaskRef    `json:"task,omitempty"`
	Project           *questionProjectRef `json:"project,omitempty"`
	Step              *questionStepRef    `json:"step,omitempty"`
	Session           *questionSessionRef `json:"session,omitempty"`
	AskedBy           *questionUserRef    `json:"asked_by,omitempty"`
	AnsweredBy        *questionUserRef    `json:"answered_by,omitempty"`
}

// excerptLen is how many characters of the question the one-line
// excerpt keeps.
const excerptLen = 140

var (
	mdChip     = regexp.MustCompile(`\[\[(?:[^\[\]|]*\|)?([^\[\]|]*)\]\]`)
	mdLink     = regexp.MustCompile(`\[([^\[\]]*)\]\([^()\s]*\)`)
	mdHeading  = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	mdStrong   = regexp.MustCompile(`(\*\*|__)(\S|\S.*?\S)(\*\*|__)`)
	mdEmphStar = regexp.MustCompile(`(^|[^\w*])\*(\S|\S[^*]*?\S)\*([^\w*]|$)`)
	mdEmphUnd  = regexp.MustCompile(`(^|[^\w])_(\S|\S[^_]*?\S)_([^\w]|$)`)
	mdCode     = regexp.MustCompile("`+([^`]*)`+")
)

// stripMarkdown drops the common inline markers so an excerpt reads as
// plain text: chip tokens and links keep their text, headings lose
// the hashes, emphasis and code lose their delimiters.
func stripMarkdown(s string) string {
	s = mdChip.ReplaceAllString(s, "$1")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdHeading.ReplaceAllString(s, "")
	s = mdCode.ReplaceAllString(s, "$1")
	s = mdStrong.ReplaceAllString(s, "$2")
	// The edge characters are part of the match, so a second pass
	// catches emphasis that shared one with its neighbour.
	for range 2 {
		s = mdEmphStar.ReplaceAllString(s, "$1$2$3")
		s = mdEmphUnd.ReplaceAllString(s, "$1$2$3")
	}
	return s
}

// excerptOf collapses a question's whitespace into one line of at most
// excerptLen characters, without markdown markers.
func excerptOf(body string) string {
	line := strings.Join(strings.Fields(stripMarkdown(body)), " ")
	r := []rune(line)
	if len(r) <= excerptLen {
		return line
	}
	return strings.TrimSpace(string(r[:excerptLen-1])) + "\u2026"
}

// uniqueIDs drops repeated ids, keeping the first occurrence's order.
func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// buildQuestionViews decorates questions with their task, project,
// step, session and users, one batched query for each kind. viewer is
// the caller: it sets for_me on the questions addressed to them.
func buildQuestionViews(ctx context.Context, pool *pgxpool.Pool, viewer uuid.UUID, qs []questions.Question) ([]questionView, error) {
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
	taskIDs, projectIDs, stepIDs = uniqueIDs(taskIDs), uniqueIDs(projectIDs), uniqueIDs(stepIDs)
	sessionIDs, userIDs = uniqueIDs(sessionIDs), uniqueIDs(userIDs)
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
		taskByID[r.ID] = questionTaskRef{ID: r.ID, Title: r.Title}
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
		v := questionView{
			ID: q.ID, Kind: q.Kind, State: q.State, Excerpt: excerptOf(q.Body),
			Options: q.Options, Multi: q.Multi,
			RecommendedKey: q.RecommendedKey, RecommendedReason: q.RecommendedReason,
			Answer: q.Answer, AnsweredAt: q.AnsweredAt, DeliveredAt: q.DeliveredAt,
			WithdrawnAt: q.WithdrawnAt, WithdrawnReason: q.WithdrawnReason,
			CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
			ViaMCP: q.AskedByTokenID != nil,
			ForMe:  q.RecipientUserID != nil && *q.RecipientUserID == viewer,
		}
		pid := q.ProjectID
		// A render failure ships the view without that HTML.
		render := func(md string) string {
			if md == "" {
				return ""
			}
			html, err := markdown.Render(ctx, pool, md, &pid)
			if err != nil {
				return ""
			}
			return html
		}
		v.BodyHTML = render(q.Body)
		if q.Answer != nil {
			v.AnswerTextHTML = render(q.Answer.Text)
			v.AnswerNoteHTML = render(q.Answer.Note)
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

// memberProjectIDs lists the projects the user belongs to. Admins get
// no extra projects here: the cross-project list is about the work a
// person takes part in, and an admin can still ask for one project.
func (d TaskDeps) memberProjectIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	projects, err := identity.ListProjects(ctx, d.Pool, userID, false)
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
			if c.Source == identity.SourceToken {
				// A token covers its own project only.
				if err := d.ensureProjectAccess(r.Context(), c, c.ProjectID); err != nil {
					writeProjectAccessError(w, err)
					return
				}
				f.ProjectIDs = []uuid.UUID{c.ProjectID}
			} else {
				ids, err := d.memberProjectIDs(r.Context(), c.UserID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				f.ProjectIDs = ids
			}
		}
		switch r.URL.Query().Get("recipient") {
		case "":
		case "me":
			uid := c.UserID
			f.RecipientUserID = &uid
		default:
			writeError(w, http.StatusBadRequest, "invalid recipient; valid: me")
			return
		}
		qs, err := questions.List(r.Context(), d.Pool, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		views, err := buildQuestionViews(r.Context(), d.Pool, c.UserID, qs)
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
		if c.Source == identity.SourceToken {
			writeError(w, http.StatusForbidden, "questions are answered by a person in the web UI")
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
		case errors.Is(err, questions.ErrNotRecipient):
			writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error(), "code": "not_recipient"})
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
		views, err := buildQuestionViews(r.Context(), d.Pool, c.UserID, []questions.Question{*q})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"question": views[0]})
	})
}
