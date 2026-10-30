package web

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/questions"
)

type mcpQuestionRow struct {
	ID       string `json:"id"`
	TaskID   string `json:"task_id"`
	StepID   string `json:"step_id"`
	State    string `json:"state"`
	Kind     string `json:"kind"`
	Question string `json:"question"`
	Age      string `json:"age"`
	Answered bool   `json:"answered"`
}

type mcpTakenAnswer struct {
	ID         string   `json:"id"`
	TaskID     string   `json:"task_id"`
	Question   string   `json:"question"`
	Choices    []string `json:"choices"`
	Text       string   `json:"text"`
	Note       string   `json:"note"`
	AnsweredBy string   `json:"answered_by"`
}

type mcpTake struct {
	Answers []mcpTakenAnswer `json:"answers"`
	Open    int              `json:"open"`
}

func TestMCPQuestions(t *testing.T) {
	f := newMCPFixture(t, 9811, "questioner")
	pool := f.pgx(t)
	pid := uuid.MustParse(f.projectID)
	uid := uuid.MustParse(f.userID)

	var task struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "pick a database", "claim": true, "session": "conv-1",
	}, &task)

	ask := func(args map[string]any) string {
		t.Helper()
		args["project_id"] = f.projectID
		args["task_id"] = task.ID
		var out struct {
			ID    string `json:"id"`
			State string `json:"state"`
		}
		f.callJSON(t, "nottario.questions.ask", args, &out)
		if out.ID == "" || out.State != "open" {
			t.Fatalf("ask = %+v", out)
		}
		return out.ID
	}
	answer := func(id string, r questions.Reply) {
		t.Helper()
		if _, err := questions.Answer(f.ctx, pool, pid, uuid.MustParse(id), uid, r); err != nil {
			t.Fatalf("Answer: %v", err)
		}
	}
	take := func(session string) mcpTake {
		t.Helper()
		var out mcpTake
		f.callJSON(t, "nottario.questions.take", map[string]any{"project_id": f.projectID, "session": session}, &out)
		return out
	}

	// Asking needs a session, and nothing is written without one.
	msg := f.callExpectErr(t, "nottario.questions.ask", map[string]any{
		"project_id": f.projectID, "task_id": task.ID, "question": "no session?",
	})
	if !strings.Contains(msg, "session is required") {
		t.Fatalf("ask without session: %q", msg)
	}
	var n int
	if err := pool.QueryRow(f.ctx, `SELECT count(*) FROM task_questions`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("questions after a refused ask = %d err=%v", n, err)
	}

	// A text question, answered directly, is taken exactly once.
	textID := ask(map[string]any{"question": "Which region should the replica run in?", "session": "conv-1"})
	if got := take("conv-1"); len(got.Answers) != 0 || got.Open != 1 {
		t.Fatalf("take before answer = %+v", got)
	}
	answer(textID, questions.Reply{Text: "eu-west-1"})
	got := take("conv-1")
	if len(got.Answers) != 1 || got.Open != 0 {
		t.Fatalf("take = %+v", got)
	}
	a := got.Answers[0]
	if a.ID != textID || a.TaskID != task.ID || a.Text != "eu-west-1" || a.AnsweredBy != "me" || len(a.Choices) != 0 ||
		!strings.HasPrefix(a.Question, "Which region") {
		t.Fatalf("answer = %+v", a)
	}
	if again := take("conv-1"); len(again.Answers) != 0 {
		t.Fatalf("second take = %+v", again)
	}
	// A session that never asked gets an empty take, not an error.
	if none := take("never-asked"); len(none.Answers) != 0 || none.Open != 0 {
		t.Fatalf("unknown session take = %+v", none)
	}

	// A choice question with a recommendation; the label comes back.
	choiceID := ask(map[string]any{
		"question": "Postgres or SQLite?",
		"options": []map[string]any{
			{"label": "Postgres", "description": "one more service"},
			{"label": "SQLite"},
		},
		"recommended": 1, "recommended_reason": "already in compose",
		"session": "conv-1",
	})
	answer(choiceID, questions.Reply{Keys: []string{"2"}, Note: "keep it light"})
	got = take("conv-1")
	if len(got.Answers) != 1 || len(got.Answers[0].Choices) != 1 || got.Answers[0].Choices[0] != "SQLite" ||
		got.Answers[0].Note != "keep it light" || got.Answers[0].Text != "" {
		t.Fatalf("choice take = %+v", got)
	}

	// list defaults to open; a bad state names the valid ones.
	openID := ask(map[string]any{"question": "Ship on Friday?", "session": "conv-1"})
	var list struct {
		Questions []mcpQuestionRow `json:"questions"`
		HasMore   bool             `json:"has_more"`
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID}, &list)
	if len(list.Questions) != 1 || list.Questions[0].ID != openID || list.Questions[0].State != "open" ||
		list.Questions[0].Kind != "text" || list.Questions[0].Age == "" || list.HasMore {
		t.Fatalf("list = %+v", list)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "all", "limit": 2}, &list)
	if len(list.Questions) != 2 || !list.HasMore {
		t.Fatalf("list all limit 2 = %+v", list)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "answered", "session": "conv-1"}, &list)
	if len(list.Questions) != 2 || !list.Questions[0].Answered {
		t.Fatalf("list answered = %+v", list)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "session": "never-asked"}, &list)
	if len(list.Questions) != 0 {
		t.Fatalf("list unknown session = %+v", list)
	}
	if msg := f.callExpectErr(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "pending"}); !strings.Contains(msg, "open, answered, withdrawn, all") {
		t.Fatalf("bad state: %q", msg)
	}

	// tasks.list rows carry questions_open; tasks.get carries the open
	// question (the delivered answers are gone from it).
	var rows struct {
		Tasks []struct {
			ID            string `json:"id"`
			QuestionsOpen int    `json:"questions_open"`
		} `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID}, &rows)
	if len(rows.Tasks) != 1 || rows.Tasks[0].QuestionsOpen != 1 {
		t.Fatalf("tasks.list = %+v", rows)
	}
	var detail struct {
		Questions []mcpQuestionRow `json:"questions"`
	}
	f.callJSON(t, "nottario.tasks.get", map[string]any{"project_id": f.projectID, "task_id": task.ID}, &detail)
	if len(detail.Questions) != 1 || detail.Questions[0].ID != openID {
		t.Fatalf("tasks.get questions = %+v", detail.Questions)
	}

	// withdraw an open question; withdrawing it again fails.
	var w mcpQuestionRow
	f.callJSON(t, "nottario.questions.withdraw", map[string]any{"project_id": f.projectID, "id": openID, "reason": "decided"}, &w)
	if w.ID != openID || w.State != "withdrawn" {
		t.Fatalf("withdraw = %+v", w)
	}
	f.callExpectErr(t, "nottario.questions.withdraw", map[string]any{"project_id": f.projectID, "id": openID})

	// Closing a task with an open question withdraws it and says so.
	ask(map[string]any{"question": "Anything else?", "session": "conv-1"})
	var closed struct {
		Task struct {
			QuestionsOpen int `json:"questions_open"`
		} `json:"task"`
		WithdrawnQuestions int    `json:"withdrawn_questions"`
		Notice             string `json:"notice"`
	}
	f.callJSON(t, "nottario.tasks.close", map[string]any{"project_id": f.projectID, "task_id": task.ID}, &closed)
	if closed.WithdrawnQuestions != 1 || closed.Task.QuestionsOpen != 0 ||
		!strings.Contains(closed.Notice, "1 open question was withdrawn") {
		t.Fatalf("close = %+v", closed)
	}

	// set_state wont_do withdraws too.
	var other struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "drop it", "claim": true}, &other)
	f.callJSON(t, "nottario.questions.ask", map[string]any{
		"project_id": f.projectID, "task_id": other.ID, "question": "Really drop it?", "session": "conv-1",
	}, nil)
	var dropped struct {
		WithdrawnQuestions int    `json:"withdrawn_questions"`
		Notice             string `json:"notice"`
	}
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{"project_id": f.projectID, "task_id": other.ID, "state": "wont_do"}, &dropped)
	if dropped.WithdrawnQuestions != 1 || dropped.Notice == "" {
		t.Fatalf("set_state wont_do = %+v", dropped)
	}

	// Every ask notified the asking human (the actor is not skipped).
	var body string
	if err := pool.QueryRow(f.ctx,
		`SELECT body FROM notifications WHERE user_id = $1 AND kind = 'question_asked' ORDER BY created_at LIMIT 1`,
		uid).Scan(&body); err != nil {
		t.Fatalf("question_asked notification: %v", err)
	}
	if body != `Question on "pick a database": Which region should the replica run in?` {
		t.Fatalf("notification body = %q", body)
	}
}
