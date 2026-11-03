package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
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

type questionsRecordAnswerInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	Session   string `json:"session" jsonschema:"your conversation id, the one you asked with; only the conversation that asked can record the answer"`
	ID        string `json:"id" jsonschema:"question uuid, one you filed with questions.ask"`
	Choices   []int  `json:"choices,omitempty" jsonschema:"for a choice question: the 1-based option numbers the human picked, in the order you listed the options in questions.ask"`
	Text      string `json:"text,omitempty" jsonschema:"the human's answer in their own words: the answer to a free-text question, or their own wording instead of the options"`
	Note      string `json:"note,omitempty" jsonschema:"anything else the human added"`
	Channel   string `json:"channel,omitempty" jsonschema:"where the human answered: 'chat' (default) or 'other'"`
	Outcome   string `json:"outcome,omitempty" jsonschema:"optional: the decision the answer led to, as in questions.set_outcome; up to 300 characters"`
}

type questionsSetOutcomeInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	Session   string `json:"session" jsonschema:"your conversation id, the one you asked with; only the conversation that asked can record the outcome"`
	ID        string `json:"id" jsonschema:"question uuid, one you filed with questions.ask and whose answer you have"`
	Outcome   string `json:"outcome" jsonschema:"one formal sentence stating the decision the answer led to, up to 300 characters, in the task's language; e.g. 'Decided: keep client-specific commands in a registry with a generic fallback.'"`
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
	AskedAt  string          `json:"asked_at"`
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
		AskedAt:  agentTime(q.CreatedAt),
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

// questionError turns an error from the questions tools into a tool
// error. Expected failures get a message the agent can act on; anything
// else is logged and reads as an internal error, so database details
// never reach the agent's context.
func questionError(tool string, err error) (*sdk.CallToolResult, any, error) {
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		return toolError("task not found")
	case errors.Is(err, tasks.ErrStepNotFound):
		return toolError("step not found")
	case errors.Is(err, questions.ErrNotFound), errors.Is(err, questions.ErrClosed),
		errors.Is(err, questions.ErrAlreadyDelivered), errors.Is(err, questions.ErrNotAsker),
		errors.Is(err, questions.ErrNotAskerRecord), errors.Is(err, questions.ErrAlreadyAnswered),
		errors.Is(err, questions.ErrNotAskingSession), errors.Is(err, questions.ErrNotAgentQuestion),
		errors.Is(err, questions.ErrNotAskerOutcome), errors.Is(err, questions.ErrNotDelivered),
		questions.IsInvalid(err):
		return toolError(err.Error())
	}
	log.Printf("mcp %s: %v", tool, err)
	return toolError("internal error")
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

// questionSession checks the session key a questions tool was given.
// required, when not empty, says why the tool needs one; a blank key is
// then an error. Otherwise only an absent key is fine.
func questionSession(key, required string) error {
	if required != "" && strings.TrimSpace(key) == "" {
		return errors.New("session is required: " + required)
	}
	return checkSession(sessionFields{Session: key})
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
	qs, err := questions.PendingForTask(ctx, d.Pool, pid, tid)
	if err != nil {
		return nil, err
	}
	var out []slimQuestion
	for i := range qs {
		out = append(out, slimQuestionOf(&qs[i]))
	}
	return out, nil
}

// withdrawalNotice reports on a close response the open questions the
// close withdrew: withdrawn_questions and a sentence added to the
// notice.
func withdrawalNotice(out map[string]any, n int) map[string]any {
	if n == 0 {
		return out
	}
	out["withdrawn_questions"] = n
	msg := fmt.Sprintf("%d open questions were withdrawn with the task.", n)
	if n == 1 {
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
		if err := questionSession(in.Session, "pass your conversation id so the answer can come back to you"); err != nil {
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
		opts := make([]questions.Option, 0, len(in.Options))
		for _, o := range in.Options {
			opts = append(opts, questions.Option{Label: o.Label, Description: o.Description})
		}
		p := questions.AskParams{
			ProjectID:         pid,
			TaskID:            tid,
			StepID:            stepID,
			AskedBy:           authorshipFor(c),
			Body:              in.Question,
			Options:           opts,
			Recommended:       in.Recommended,
			RecommendedReason: in.RecommendedReason,
			Multi:             in.Multi,
		}
		// A bad question writes nothing, not even the session.
		if err := questions.ValidateAsk(p); err != nil {
			return questionError("questions.ask", err)
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		if p.SessionID, err = touchSession(ctx, req, d, pid, c, in.sessionFields); err != nil {
			return questionError("questions.ask", err)
		}
		q, err := questions.Ask(ctx, d.Pool, p)
		if err != nil {
			return questionError("questions.ask", err)
		}
		if d.Notifier.Enabled() && q.RecipientUserID != nil {
			if t, err := tasks.Get(ctx, d.Pool, tid); err == nil {
				d.Notifier.OnQuestionAsked(ctx, t, *q.RecipientUserID, q.Body)
			}
		}
		return jsonResult(map[string]any{"id": q.ID, "state": q.State})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.take",
		Description: "Returns the answers to your session's questions that you have not seen yet and marks them delivered: {answers: [{id, task_id, step_id, question, choices, text, note, answered_by}], open}. Call it at the start of every turn, after each subagent returns and before ending a turn. After acting on each answer, record the decision it led to with questions.set_outcome.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsTakeInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := questionSession(in.Session, "pass the conversation id you asked with"); err != nil {
			return toolError(err.Error())
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		sid, err := findSession(ctx, d, pid, c, in.Session)
		if err != nil {
			return questionError("questions.take", err)
		}
		answers := []takenAnswer{}
		if sid == nil {
			return jsonResult(map[string]any{"answers": answers, "open": 0})
		}
		qs, open, err := questions.Take(ctx, d.Pool, pid, *sid)
		if err != nil {
			return questionError("questions.take", err)
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
			return questionError("questions.list", err)
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
		if err := questionSession(in.Session, ""); err != nil {
			return toolError(err.Error())
		}
		if in.Session != "" {
			sid, err := findSession(ctx, d, pid, c, in.Session)
			if err != nil {
				return questionError("questions.list", err)
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
			return questionError("questions.list", err)
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
		Description: "Withdraws an open question you asked and no longer need answered; only its asker can. Closing a task withdraws its open questions.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsWithdrawInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return toolError("id must be a uuid")
		}
		q, err := questions.Withdraw(ctx, d.Pool, pid, id, c.UserID, in.Reason)
		if err != nil {
			return questionError("questions.withdraw", err)
		}
		return jsonResult(slimQuestionOf(q))
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.record_answer",
		Description: "ONLY for recording an answer the human already gave you outside Nottario (for example in the chat) to a question you filed with questions.ask in this same conversation (pass your session), so it reads answered in Nottario too. Never use it to answer on the human's behalf, to guess, or to close a question nobody answered. Pass their picks as choices and/or their own words as text. Marks the question answered and delivered, so questions.take will not return it. Pass outcome to record the decision it led to in the same call (see questions.set_outcome). Returns {id, state, outcome}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsRecordAnswerInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := questionSession(in.Session, "pass the conversation id you asked with"); err != nil {
			return toolError(err.Error())
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return toolError("id must be a uuid")
		}
		sid, err := findSession(ctx, d, pid, c, in.Session)
		if err != nil {
			return questionError("questions.record_answer", err)
		}
		// An unknown conversation matches no question: the domain
		// refuses it after the asker check.
		session := uuid.Nil
		if sid != nil {
			session = *sid
		}
		reply := questions.Reply{Text: in.Text, Note: in.Note}
		for _, n := range in.Choices {
			reply.Keys = append(reply.Keys, strconv.Itoa(n))
		}
		q, err := questions.RecordAnswer(ctx, d.Pool, pid, id, c.UserID, session, authorshipFor(c).TokenID, in.Channel, reply, in.Outcome)
		if err != nil {
			return questionError("questions.record_answer", err)
		}
		return jsonResult(map[string]any{"id": q.ID, "state": q.State, "outcome": q.Outcome})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.questions.set_outcome",
		Description: "Records the decision an answered question led to, on a question you filed with questions.ask in this same conversation (pass your session) once its answer has reached you. Write one formal sentence stating the decision, for someone reading the task later: not a transcript of the chat, not the human's wording; in the task's language; up to 300 characters. Calling it again replaces the outcome. Returns {id, state, outcome}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in questionsSetOutcomeInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := questionSession(in.Session, "pass the conversation id you asked with"); err != nil {
			return toolError(err.Error())
		}
		pid, err := questionsProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return toolError("id must be a uuid")
		}
		sid, err := findSession(ctx, d, pid, c, in.Session)
		if err != nil {
			return questionError("questions.set_outcome", err)
		}
		// An unknown conversation matches no question: the domain
		// refuses it after the asker check.
		session := uuid.Nil
		if sid != nil {
			session = *sid
		}
		q, err := questions.SetOutcome(ctx, d.Pool, pid, id, c.UserID, session, in.Outcome)
		if err != nil {
			return questionError("questions.set_outcome", err)
		}
		return jsonResult(map[string]any{"id": q.ID, "state": q.State, "outcome": q.Outcome})
	})
}
