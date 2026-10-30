package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
)

type questionOptionInput struct {
	Label       string `json:"label" jsonschema:"what the human picks, up to 120 characters"`
	Description string `json:"description,omitempty" jsonschema:"one line on what it implies, up to 300 characters"`
}

type questionsAskInput struct {
	ProjectID         string                `json:"project_id" jsonschema:"project uuid"`
	TaskID            string                `json:"task_id" jsonschema:"task uuid the question is about"`
	StepID            string                `json:"step_id,omitempty" jsonschema:"step uuid, when the question is about one step"`
	Question          string                `json:"question" jsonschema:"markdown, up to 4000 characters: context, what you need, what you will do with each answer"`
	Options           []questionOptionInput `json:"options,omitempty" jsonschema:"2 to 9 choices; omit for a free-text question. The human can always answer with their own text instead"`
	Recommended       int                   `json:"recommended,omitempty" jsonschema:"1-based index of the option you recommend"`
	RecommendedReason string                `json:"recommended_reason,omitempty" jsonschema:"why, one line"`
	Multi             bool                  `json:"multi,omitempty" jsonschema:"the human may pick several options"`
	sessionFields
}

type questionsTakeInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	Session   string `json:"session" jsonschema:"your conversation id, the one you asked with"`
}

type questionsListInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	State     string `json:"state,omitempty" jsonschema:"'open' (default), 'answered', 'withdrawn' or 'all'"`
	TaskID    string `json:"task_id,omitempty" jsonschema:"only this task's questions"`
	Session   string `json:"session,omitempty" jsonschema:"only the questions asked by this conversation id of yours"`
	Limit     int    `json:"limit,omitempty" jsonschema:"rows to return, 1..200; default 50"`
}

type questionsWithdrawInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	ID        string `json:"id" jsonschema:"question uuid"`
	Reason    string `json:"reason,omitempty" jsonschema:"why it is no longer needed, one line"`
}

// questions.list page bounds.
const (
	questionsListDefault = 50
	questionsListMax     = 200
)

// questionExcerptLen is how much of a question agents get back: they
// wrote it, so the start is enough to recognise it.
const questionExcerptLen = 200

// slimQuestion is a question as list rows and tasks.get carry it.
type slimQuestion struct {
	ID       uuid.UUID       `json:"id"`
	TaskID   uuid.UUID       `json:"task_id"`
	StepID   *uuid.UUID      `json:"step_id,omitempty"`
	State    questions.State `json:"state"`
	Kind     questions.Kind  `json:"kind"`
	Question string          `json:"question"`
	Age      string          `json:"age"`
	Answered bool            `json:"answered,omitempty"`
}

// takenAnswer is one answer as questions.take hands it over.
type takenAnswer struct {
	ID         uuid.UUID  `json:"id"`
	TaskID     uuid.UUID  `json:"task_id"`
	StepID     *uuid.UUID `json:"step_id,omitempty"`
	Question   string     `json:"question"`
	Choices    []string   `json:"choices,omitempty"`
	Text       string     `json:"text,omitempty"`
	Note       string     `json:"note,omitempty"`
	AnsweredBy string     `json:"answered_by,omitempty"`
}

// excerpt cuts s to its first n characters, marking the cut.
func excerpt(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func slimQuestionOf(q *questions.Question) slimQuestion {
	return slimQuestion{
		ID:       q.ID,
		TaskID:   q.TaskID,
		StepID:   q.StepID,
		State:    q.State,
		Kind:     q.Kind,
		Question: excerpt(q.Body, questionExcerptLen),
		Age:      agentTime(q.CreatedAt),
		Answered: q.State == questions.StateAnswered,
	}
}

func (v *taskView) answer(q *questions.Question) takenAnswer {
	out := takenAnswer{
		ID:         q.ID,
		TaskID:     q.TaskID,
		StepID:     q.StepID,
		Question:   excerpt(q.Body, questionExcerptLen),
		AnsweredBy: v.user(q.AnsweredByUserID),
	}
	if q.Answer == nil {
		return out
	}
	labels := make(map[string]string, len(q.Options))
	for _, o := range q.Options {
		labels[o.Key] = o.Label
	}
	for _, k := range q.Answer.Keys {
		if l, ok := labels[k]; ok {
			out.Choices = append(out.Choices, l)
		}
	}
	out.Text = q.Answer.Text
	out.Note = q.Answer.Note
	return out
}

// questionError turns a questions or tasks error into the tool error
// an agent can act on.
func questionError(err error) (*sdk.CallToolResult, any, error) {
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		return toolError("task not found")
	case errors.Is(err, tasks.ErrStepNotFound):
		return toolError("step not found on this task")
	}
	return toolError(err.Error())
}

// questionsProject parses and checks the project of a questions tool.
func questionsProject(ctx context.Context, d Deps, projectID string) (uuid.UUID, error) {
	pid, err := uuid.Parse(projectID)
	if err != nil {
		return uuid.Nil, errors.New("project_id must be a uuid")
	}
	if err := requireProjectAccess(ctx, d, pid); err != nil {
		return uuid.Nil, err
	}
	return pid, nil
}

// findSession looks the caller's session up by key without creating
// it; nil when the conversation never registered here.
func findSession(ctx context.Context, d Deps, pid uuid.UUID, c identity.Caller, key string) (*uuid.UUID, error) {
	s, err := agents.Find(ctx, d.Pool, pid, c.UserID, key)
	if err != nil || s == nil {
		return nil, err
	}
	return &s.ID, nil
}

// taskQuestions is what tasks.get carries: the task's open questions
// and the answers its agent has not taken yet. Nil when there are none.
func taskQuestions(ctx context.Context, d Deps, pid, tid uuid.UUID) ([]slimQuestion, error) {
	qs, err := questions.List(ctx, d.Pool, questions.ListFilter{
		ProjectIDs: []uuid.UUID{pid}, TaskID: &tid, Limit: questions.MaxListLimit,
	})
	if err != nil {
		return nil, err
	}
	var out []slimQuestion
	for i := range qs {
		q := &qs[i]
		if q.State == questions.StateOpen || (q.State == questions.StateAnswered && q.DeliveredAt == nil) {
			out = append(out, slimQuestionOf(q))
		}
	}
	return out, nil
}

// withdrawal is what closing a task did to its open questions.
type withdrawal struct {
	n   int
	err error
}

// withdrawTaskQuestions withdraws a closed task's open questions and
// clears the count on t, so the task in the response reads right.
func withdrawTaskQuestions(ctx context.Context, d Deps, t *tasks.Task) withdrawal {
	n, err := questions.WithdrawOpenForTask(ctx, d.Pool, t.ID, "task closed")
	if err == nil {
		t.QuestionsOpen = max(t.QuestionsOpen-n, 0)
	}
	return withdrawal{n: n, err: err}
}

// withdrawalNotice reports a withdrawal on a close response:
// withdrawn_questions and a sentence added to the notice. The state
// change already happened, so a failure is reported as
// questions_error, never as a tool error (a retry would fail on the
// closed task).
func withdrawalNotice(out map[string]any, w withdrawal) map[string]any {
	if w.err != nil {
		out["questions_error"] = "the task closed, but withdrawing its open questions failed: " + w.err.Error()
		return out
	}
	if w.n == 0 {
		return out
	}
	out["withdrawn_questions"] = w.n
	msg := fmt.Sprintf("%d open questions were withdrawn with the task.", w.n)
	if w.n == 1 {
		msg = "1 open question was withdrawn with the task."
	}
	if prev, ok := out["notice"].(string); ok && prev != "" {
		msg = prev + " " + msg
	}
	out["notice"] = msg
	return out
}

func registerQuestions(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.ask",
		Description: "Files a question only the human can answer, on a task (and optionally a step). Needs your session: answers go back to it. Free text, or 2-9 options with an optional recommendation. Returns {id}. Keep working on what does not depend on the answer; collect answers with questions.take.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsAskInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Session) == "" {
			return toolError("session is required: pass your conversation id so the answer can come back to you")
		}
		if err := checkSession(in.sessionFields); err != nil {
			return toolError(err.Error())
		}
		pid, tid, err := parseProjectAndTask(in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		var stepID *uuid.UUID
		if in.StepID != "" {
			id, err := parseStepID(in.StepID)
			if err != nil {
				return toolError(err.Error())
			}
			stepID = &id
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		session, err := touchSession(ctx, req, d, pid, c, in.sessionFields)
		if err != nil {
			return toolError("recording the session failed, so the question was not filed: " + err.Error())
		}
		opts := make([]questions.Option, 0, len(in.Options))
		for _, o := range in.Options {
			opts = append(opts, questions.Option{Label: o.Label, Description: o.Description})
		}
		q, err := questions.Ask(ctx, d.Pool, questions.AskParams{
			ProjectID:         pid,
			TaskID:            tid,
			StepID:            stepID,
			SessionID:         session,
			AskedBy:           authorshipFor(c),
			Body:              in.Question,
			Options:           opts,
			Recommended:       in.Recommended,
			RecommendedReason: in.RecommendedReason,
			Multi:             in.Multi,
		})
		if err != nil {
			return questionError(err)
		}
		if d.Notifier.Enabled() {
			if t, err := tasks.Get(ctx, d.Pool, tid); err == nil {
				d.Notifier.OnQuestionAsked(ctx, t, c.UserID, q.Body)
			}
		}
		return jsonResult(map[string]any{"id": q.ID, "state": q.State})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.take",
		Description: "Returns the answers to your session's questions that you have not seen yet and marks them delivered: {answers: [{id, task_id, step_id, question, choices, text, note, answered_by}], open}. Call it at the start of every turn, after each subagent returns and before ending a turn.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsTakeInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Session) == "" {
			return toolError("session is required: pass the conversation id you asked with")
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		sid, err := findSession(ctx, d, pid, c, in.Session)
		if err != nil {
			return toolError(err.Error())
		}
		answers := []takenAnswer{}
		if sid == nil {
			return jsonResult(map[string]any{"answers": answers, "open": 0})
		}
		qs, open, err := questions.Take(ctx, d.Pool, pid, *sid)
		if err != nil {
			return toolError(err.Error())
		}
		v := newTaskView(ctx, d)
		for i := range qs {
			answers = append(answers, v.answer(&qs[i]))
		}
		return jsonResult(map[string]any{"answers": answers, "open": open})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.list",
		Description: "Lists questions: state 'open' (default), 'answered', 'withdrawn' or 'all'; filter by task_id or session. Slim rows {questions, has_more}; limit defaults to 50 (max 200), narrow the filter when has_more.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsListInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		raw := in.State
		if strings.TrimSpace(raw) == "" {
			raw = string(questions.StateOpen)
		}
		state, err := questions.ParseState(raw)
		if err != nil {
			return toolError(err.Error())
		}
		f := questions.ListFilter{ProjectIDs: []uuid.UUID{pid}, State: state}
		if in.TaskID != "" {
			tid, err := uuid.Parse(in.TaskID)
			if err != nil {
				return toolError("task_id must be a uuid")
			}
			f.TaskID = &tid
		}
		rows := []slimQuestion{}
		if in.Session != "" {
			sid, err := findSession(ctx, d, pid, c, in.Session)
			if err != nil {
				return toolError(err.Error())
			}
			if sid == nil {
				return jsonResult(map[string]any{"questions": rows, "has_more": false})
			}
			f.SessionID = sid
		}
		limit := in.Limit
		switch {
		case limit <= 0:
			limit = questionsListDefault
		case limit > questionsListMax:
			limit = questionsListMax
		}
		f.Limit = limit + 1
		qs, err := questions.List(ctx, d.Pool, f)
		if err != nil {
			return toolError(err.Error())
		}
		more := len(qs) > limit
		if more {
			qs = qs[:limit]
		}
		for i := range qs {
			rows = append(rows, slimQuestionOf(&qs[i]))
		}
		return jsonResult(map[string]any{"questions": rows, "has_more": more})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.withdraw",
		Description: "Withdraws an open question you no longer need answered. Closing a task withdraws its open questions.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsWithdrawInput) (*sdk.CallToolResult, any, error) {
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return toolError("id must be a uuid")
		}
		q, err := questions.Withdraw(ctx, d.Pool, pid, id, in.Reason)
		if err != nil {
			return questionError(err)
		}
		return jsonResult(slimQuestionOf(q))
	})
}
