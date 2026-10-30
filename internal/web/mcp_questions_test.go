package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
)

type mcpQuestionRow struct {
	ID       string `json:"id"`
	TaskID   string `json:"task_id"`
	StepID   string `json:"step_id"`
	State    string `json:"state"`
	Kind     string `json:"kind"`
	Question string `json:"question"`
	AskedAt  string `json:"asked_at"`
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
		list.Questions[0].Kind != "text" || list.Questions[0].AskedAt == "" || list.HasMore {
		t.Fatalf("list = %+v", list)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "all", "limit": 2}, &list)
	if len(list.Questions) != 2 || !list.HasMore {
		t.Fatalf("list all limit 2 = %+v", list)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "answered", "session": "conv-1"}, &list)
	if len(list.Questions) != 2 || list.Questions[0].State != "answered" {
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
	if msg := f.callExpectErr(t, "nottario.questions.withdraw", map[string]any{"project_id": f.projectID, "id": openID}); msg != questions.ErrClosed.Error() {
		t.Fatalf("second withdraw: %q", msg)
	}

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

func TestMCPQuestionsEdges(t *testing.T) {
	f := newMCPFixture(t, 9813, "questions-edges")
	pool := f.pgx(t)
	pid := uuid.MustParse(f.projectID)
	uid := uuid.MustParse(f.userID)

	newTask := func(title string) string {
		t.Helper()
		var out struct {
			ID string `json:"id"`
		}
		f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": title, "claim": true}, &out)
		return out.ID
	}
	ask := func(args map[string]any) string {
		t.Helper()
		args["project_id"] = f.projectID
		args["session"] = "edges"
		var out struct {
			ID string `json:"id"`
		}
		f.callJSON(t, "nottario.questions.ask", args, &out)
		return out.ID
	}
	askErr := func(args map[string]any) string {
		t.Helper()
		args["project_id"] = f.projectID
		args["session"] = "edges-refused"
		return f.callExpectErr(t, "nottario.questions.ask", args)
	}
	task := newTask("edges host")

	// A question scoped to a step carries the step.
	var steps struct {
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
	}
	f.callJSON(t, "nottario.tasks.add_steps", map[string]any{"project_id": f.projectID, "task_id": task, "texts": []string{"choose"}}, &steps)
	stepQ := ask(map[string]any{"task_id": task, "step_id": steps.Steps[0].ID, "question": "Which one?"})
	var list struct {
		Questions []mcpQuestionRow `json:"questions"`
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "task_id": task}, &list)
	if len(list.Questions) != 1 || list.Questions[0].ID != stepQ || list.Questions[0].StepID != steps.Steps[0].ID {
		t.Fatalf("step question = %+v", list.Questions)
	}

	// Refused questions, each with its message; none files a question.
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"task_id": task, "step_id": uuid.NewString(), "question": "q"}, "step not found"},
		{map[string]any{"task_id": uuid.NewString(), "question": "q"}, "task not found"},
		{map[string]any{"task_id": task, "question": "q", "options": []map[string]any{{"label": "a"}, {"label": "b"}}, "recommended": 3}, "recommended must be an option number between 1 and 2"},
		{map[string]any{"task_id": task, "question": "q", "multi": true}, "multi needs options"},
		{map[string]any{"task_id": task, "question": "   "}, "question is empty"},
	} {
		if msg := askErr(c.args); msg != c.want {
			t.Errorf("ask %v = %q, want %q", c.args, msg, c.want)
		}
	}
	var qn int
	if err := pool.QueryRow(f.ctx, `SELECT count(*) FROM task_questions WHERE task_id = $1`, task).Scan(&qn); err != nil || qn != 1 {
		t.Fatalf("questions after refused asks = %d err=%v", qn, err)
	}
	// An invalid question is refused before the session is recorded.
	var n int
	invalid := f.callExpectErr(t, "nottario.questions.ask", map[string]any{
		"project_id": f.projectID, "task_id": task, "question": "q", "multi": true, "session": "never-written",
	})
	if invalid != "multi needs options" {
		t.Fatalf("invalid ask = %q", invalid)
	}
	if err := pool.QueryRow(f.ctx, `SELECT count(*) FROM agent_sessions WHERE session_key = 'never-written'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("an invalid question recorded its session: %d err=%v", n, err)
	}

	// A closed task takes no questions.
	closed := newTask("closed host")
	f.callJSON(t, "nottario.tasks.close", map[string]any{"project_id": f.projectID, "task_id": closed}, nil)
	if msg := askErr(map[string]any{"task_id": closed, "question": "too late?"}); !strings.Contains(msg, "the task is closed") {
		t.Fatalf("ask on a closed task = %q", msg)
	}

	// An answered question cannot be withdrawn.
	if _, err := questions.Answer(f.ctx, pool, pid, uuid.MustParse(stepQ), uid, questions.Reply{Text: "that one"}); err != nil {
		t.Fatal(err)
	}
	if msg := f.callExpectErr(t, "nottario.questions.withdraw", map[string]any{"project_id": f.projectID, "id": stepQ}); !strings.Contains(msg, "already answered") {
		t.Fatalf("withdraw answered = %q", msg)
	}

	// Another project's question is invisible here.
	otherProj, err := identity.CreateProject(f.ctx, pool, "Edges other", "", "", "", uid)
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := tasks.Create(f.ctx, pool, tasks.CreateParams{ProjectID: otherProj.ID, Type: tasks.TypeTask, Title: "elsewhere"}, tasks.Authorship{UserID: &uid})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := questions.Ask(f.ctx, pool, questions.AskParams{ProjectID: otherProj.ID, TaskID: otherTask.ID, AskedBy: tasks.Authorship{UserID: &uid}, Body: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	f.callJSON(t, "nottario.questions.list", map[string]any{"project_id": f.projectID, "state": "all"}, &list)
	for _, q := range list.Questions {
		if q.ID == foreign.ID.String() {
			t.Fatal("list shows another project's question")
		}
	}
	if msg := f.callExpectErr(t, "nottario.questions.withdraw", map[string]any{"project_id": f.projectID, "id": foreign.ID.String()}); msg != questions.ErrNotFound.Error() {
		t.Fatalf("withdraw foreign = %q", msg)
	}

	// Only the user who asked may withdraw: a second member is refused.
	openQ := ask(map[string]any{"task_id": task, "question": "Still open?"})
	member, _, err := identity.UpsertFromGithub(f.ctx, pool, 9814, "questions-member", "Member", "")
	if err != nil {
		t.Fatal(err)
	}
	roles, err := identity.ListRoles(f.ctx, pool, pid)
	if err != nil || len(roles) == 0 {
		t.Fatalf("roles: %v", err)
	}
	if err := identity.AddMembership(f.ctx, pool, member.ID, pid, roles[0].ID); err != nil {
		t.Fatal(err)
	}
	memberToken, _, err := identity.IssueToken(f.ctx, pool, member.ID, pid, "member", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "qa-member", Version: "v0"}, nil)
	ms, err := client.Connect(f.ctx, &sdk.StreamableClientTransport{
		Endpoint:             f.baseURL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{rt: http.DefaultTransport, token: memberToken}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	res, err := ms.CallTool(f.ctx, &sdk.CallToolParams{Name: "nottario.questions.withdraw", Arguments: map[string]any{"project_id": f.projectID, "id": openQ}})
	if err != nil {
		t.Fatal(err)
	}
	if tc, _ := res.Content[0].(*sdk.TextContent); !res.IsError || tc == nil || tc.Text != "only the user who asked this question can withdraw it" {
		t.Fatalf("withdraw by another member = %+v", res.Content)
	}

	// Closing over HTTP withdraws the open question too.
	r := doRaw(t, "POST", f.baseURL+"/api/projects/"+f.projectID+"/tasks/"+task+"/state", "Bearer "+f.token, []byte(`{"state":"wont_do"}`))
	if r.StatusCode != http.StatusOK {
		t.Fatalf("HTTP set_state = %d %s", r.StatusCode, r.Body)
	}
	var state string
	if err := pool.QueryRow(f.ctx, `SELECT state FROM task_questions WHERE id = $1`, openQ).Scan(&state); err != nil || state != "withdrawn" {
		t.Fatalf("question after HTTP close = %q err=%v", state, err)
	}
}
