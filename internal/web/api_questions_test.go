package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

type questionsResp struct {
	Questions []struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		BodyHTML string `json:"body_html"`
		Task     *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
		Project *struct {
			Slug string `json:"slug"`
		} `json:"project"`
		Step *struct {
			Text string `json:"text"`
		} `json:"step"`
		Session *struct {
			Label string `json:"label"`
			Key   string `json:"key"`
		} `json:"session"`
		AskedBy *struct {
			Login string `json:"login"`
		} `json:"asked_by"`
		AnsweredBy *struct {
			Login string `json:"login"`
		} `json:"answered_by"`
	} `json:"questions"`
}

func TestApiQuestions(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 15501, "web-q", "Web Q", "")
	if err != nil {
		t.Fatal(err)
	}
	stranger, _, err := identity.UpsertFromGithub(ctx, pool, 15502, "web-q-stranger", "Stranger", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Web questions", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := identity.CreateProject(ctx, pool, "Foreign questions", "", "", "", stranger.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	strangerToken, _, err := identity.IssueToken(ctx, pool, stranger.ID, foreign.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "ask me"}, by)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := tasks.AddSteps(ctx, pool, tk.ID, []string{"first step"}, by)
	if err != nil {
		t.Fatal(err)
	}
	ftk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: foreign.ID, Type: tasks.TypeTask, Title: "theirs"}, tasks.Authorship{UserID: &stranger.ID})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: "conv-1", Label: "Backend agent", Client: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	q, err := questions.Ask(ctx, pool, questions.AskParams{
		ProjectID: p.ID, TaskID: tk.ID, StepID: &steps[0].ID, SessionID: &sess.ID, AskedBy: by,
		Body: "Which **one**?", Options: []questions.Option{{Label: "A"}, {Label: "B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fq, err := questions.Ask(ctx, pool, questions.AskParams{
		ProjectID: foreign.ID, TaskID: ftk.ID, AskedBy: tasks.Authorship{UserID: &stranger.ID}, Body: "secret?",
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token
	answerURL := ts.URL + "/api/projects/" + p.ID.String() + "/questions/" + q.ID.String() + "/answer"

	// The list is scoped to the caller's projects and carries the view.
	var list questionsResp
	doJSON(t, "GET", ts.URL+"/api/questions?state=all", auth, nil, &list)
	if len(list.Questions) != 1 || list.Questions[0].ID != q.ID.String() {
		t.Fatalf("list = %+v", list.Questions)
	}
	v := list.Questions[0]
	if v.Task == nil || v.Task.Title != "ask me" || v.Project == nil || v.Project.Slug != p.Slug ||
		v.Step == nil || v.Step.Text != "first step" || v.AskedBy == nil || v.AskedBy.Login != "web-q" || v.BodyHTML == "" ||
		v.Session == nil || v.Session.Label != "Backend agent" || v.Session.Key != "conv-1" {
		t.Fatalf("view = %+v", v)
	}
	var byProject questionsResp
	doJSON(t, "GET", ts.URL+"/api/questions?project_id="+p.ID.String()+"&state=open", auth, nil, &byProject)
	if len(byProject.Questions) != 1 {
		t.Fatalf("by project = %+v", byProject.Questions)
	}
	if r := doRaw(t, "GET", ts.URL+"/api/questions?project_id="+foreign.ID.String(), auth, nil); r.StatusCode == http.StatusOK {
		t.Fatalf("foreign project list: %d %s", r.StatusCode, r.Body)
	}
	if r := doRaw(t, "GET", ts.URL+"/api/questions?state=bogus", auth, nil); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad state: %d", r.StatusCode)
	}

	// Answer, then edit.
	if r := doRaw(t, "POST", answerURL, auth, []byte(`{"keys":["9"]}`)); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid answer: %d %s", r.StatusCode, r.Body)
	}
	var ans struct {
		Question struct {
			State      string `json:"state"`
			Answer     struct{ Keys []string }
			AnsweredBy *struct {
				Login string `json:"login"`
			} `json:"answered_by"`
		} `json:"question"`
	}
	doJSON(t, "POST", answerURL, auth, []byte(`{"keys":["1"]}`), &ans)
	if ans.Question.State != "answered" || ans.Question.Answer.Keys[0] != "1" || ans.Question.AnsweredBy == nil || ans.Question.AnsweredBy.Login != "web-q" {
		t.Fatalf("answer = %+v", ans)
	}
	doJSON(t, "PUT", answerURL, auth, []byte(`{"keys":["2"],"note":"changed my mind"}`), &ans)
	if ans.Question.Answer.Keys[0] != "2" {
		t.Fatalf("edit = %+v", ans)
	}

	// Once the asking session takes it, the answer is frozen.
	if taken, _, err := questions.Take(ctx, pool, p.ID, sess.ID); err != nil || len(taken) != 1 {
		t.Fatalf("take: %v %v", taken, err)
	}
	r := doRaw(t, "PUT", answerURL, auth, []byte(`{"keys":["1"]}`))
	var conflict struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(r.Body, &conflict)
	if r.StatusCode != http.StatusConflict || conflict.Code != "already_delivered" {
		t.Fatalf("edit after take: %d %s", r.StatusCode, r.Body)
	}

	// A withdrawn question answers 409 withdrawn.
	q2, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: tk.ID, AskedBy: by, Body: "second?"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := questions.Withdraw(ctx, pool, p.ID, q2.ID, u.ID, "never mind"); err != nil {
		t.Fatal(err)
	}
	r = doRaw(t, "POST", ts.URL+"/api/projects/"+p.ID.String()+"/questions/"+q2.ID.String()+"/answer", auth, []byte(`{"text":"x"}`))
	conflict.Code = ""
	_ = json.Unmarshal(r.Body, &conflict)
	if r.StatusCode != http.StatusConflict || conflict.Code != "withdrawn" {
		t.Fatalf("answer withdrawn: %d %s", r.StatusCode, r.Body)
	}

	// A non-member cannot see or answer it, and the question of another
	// project is not reachable through this project's path.
	sAuth := "Bearer " + strangerToken
	if r := doRaw(t, "POST", answerURL, sAuth, []byte(`{"keys":["1"]}`)); r.StatusCode != http.StatusForbidden && r.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger answer: %d", r.StatusCode)
	}
	if r := doRaw(t, "POST", ts.URL+"/api/projects/"+p.ID.String()+"/questions/"+fq.ID.String()+"/answer", auth, []byte(`{"text":"x"}`)); r.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-project question: %d %s", r.StatusCode, r.Body)
	}

	// The task detail lists the task's questions in every state.
	var detail struct {
		Questions []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"questions"`
	}
	doJSON(t, "GET", ts.URL+"/api/projects/"+p.ID.String()+"/tasks/"+tk.ID.String(), auth, nil, &detail)
	if len(detail.Questions) != 2 {
		t.Fatalf("detail questions = %+v", detail.Questions)
	}

	// Closing the task reports the questions it withdrew.
	q3, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: tk.ID, AskedBy: by, Body: "third?"})
	if err != nil {
		t.Fatal(err)
	}
	_ = q3
	var closed struct {
		ID                 string `json:"id"`
		WithdrawnQuestions int    `json:"withdrawn_questions"`
	}
	doJSON(t, "POST", ts.URL+"/api/projects/"+p.ID.String()+"/tasks/"+tk.ID.String()+"/state", auth, []byte(`{"state":"wont_do"}`), &closed)
	if closed.ID != tk.ID.String() || closed.WithdrawnQuestions != 1 {
		t.Fatalf("close = %+v", closed)
	}
}
