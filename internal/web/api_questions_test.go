package web

import (
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		ForMe    bool   `json:"for_me"`
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
	key := []byte("k")
	cookieFor := func(uid uuid.UUID) *http.Cookie {
		t.Helper()
		sess, err := identity.NewSession(ctx, pool, uid, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	}
	call := func(method, path string, c *http.Cookie, body string) *rawResp {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rd)
		req.Header.Set("Content-Type", "application/json")
		if c != nil {
			req.AddCookie(c)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return &rawResp{StatusCode: resp.StatusCode, Body: raw}
	}
	getJSON := func(path string, c *http.Cookie, out any) {
		t.Helper()
		r := call("GET", path, c, "")
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, r.StatusCode, r.Body)
		}
		if err := json.Unmarshal(r.Body, out); err != nil {
			t.Fatal(err)
		}
	}
	sendJSON := func(method, path string, c *http.Cookie, body string, out any) {
		t.Helper()
		r := call(method, path, c, body)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, r.StatusCode, r.Body)
		}
		if err := json.Unmarshal(r.Body, out); err != nil {
			t.Fatal(err)
		}
	}
	me, strangerC := cookieFor(u.ID), cookieFor(stranger.ID)
	answerURL := "/api/projects/" + p.ID.String() + "/questions/" + q.ID.String() + "/answer"

	// The list is scoped to the caller's projects and carries the view.
	var list questionsResp
	getJSON("/api/questions?state=all", me, &list)
	if len(list.Questions) != 1 || list.Questions[0].ID != q.ID.String() {
		t.Fatalf("list = %+v", list.Questions)
	}
	v := list.Questions[0]
	if v.Task == nil || v.Task.Title != "ask me" || v.Project == nil || v.Project.Slug != p.Slug ||
		v.Step == nil || v.Step.Text != "first step" || v.AskedBy == nil || v.AskedBy.Login != "web-q" || v.BodyHTML == "" ||
		v.Session == nil || v.Session.Label != "Backend agent" || v.Session.Key != "conv-1" || !v.ForMe {
		t.Fatalf("view = %+v", v)
	}
	var byProject questionsResp
	getJSON("/api/questions?project_id="+p.ID.String()+"&state=open", me, &byProject)
	if len(byProject.Questions) != 1 {
		t.Fatalf("by project = %+v", byProject.Questions)
	}
	if r := call("GET", "/api/questions?project_id="+p.ID.String(), strangerC, ""); r.StatusCode == http.StatusOK {
		t.Fatalf("foreign project list: %d %s", r.StatusCode, r.Body)
	}
	if r := call("GET", "/api/questions?state=bogus", me, ""); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad state: %d", r.StatusCode)
	}

	// An API token cannot answer: that is for a person in the web UI.
	if r := doRaw(t, "POST", ts.URL+answerURL, "Bearer "+token, []byte(`{"keys":["1"]}`)); r.StatusCode != http.StatusForbidden {
		t.Fatalf("token answer: %d %s", r.StatusCode, r.Body)
	}
	if r := doRaw(t, "PUT", ts.URL+answerURL, "Bearer "+token, []byte(`{"keys":["1"]}`)); r.StatusCode != http.StatusForbidden {
		t.Fatalf("token edit: %d %s", r.StatusCode, r.Body)
	}

	// Answer, then edit.
	if r := call("POST", answerURL, me, string([]byte(`{"keys":["9"]}`))); r.StatusCode != http.StatusBadRequest {
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
	sendJSON("POST", answerURL, me, string([]byte(`{"keys":["1"]}`)), &ans)
	if ans.Question.State != "answered" || ans.Question.Answer.Keys[0] != "1" || ans.Question.AnsweredBy == nil || ans.Question.AnsweredBy.Login != "web-q" {
		t.Fatalf("answer = %+v", ans)
	}
	sendJSON("PUT", answerURL, me, string([]byte(`{"keys":["2"],"note":"changed my mind"}`)), &ans)
	if ans.Question.Answer.Keys[0] != "2" {
		t.Fatalf("edit = %+v", ans)
	}

	// Once the asking session takes it, the answer is frozen.
	if taken, _, err := questions.Take(ctx, pool, p.ID, sess.ID); err != nil || len(taken) != 1 {
		t.Fatalf("take: %v %v", taken, err)
	}
	r := call("PUT", answerURL, me, string([]byte(`{"keys":["1"]}`)))
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
	r = call("POST", "/api/projects/"+p.ID.String()+"/questions/"+q2.ID.String()+"/answer", me, `{"text":"x"}`)
	conflict.Code = ""
	_ = json.Unmarshal(r.Body, &conflict)
	if r.StatusCode != http.StatusConflict || conflict.Code != "withdrawn" {
		t.Fatalf("answer withdrawn: %d %s", r.StatusCode, r.Body)
	}

	// A non-member cannot see or answer it, and the question of another
	// project is not reachable through this project's path.
	if r := call("POST", answerURL, strangerC, string([]byte(`{"keys":["1"]}`))); r.StatusCode != http.StatusForbidden && r.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger answer: %d", r.StatusCode)
	}
	if r := call("POST", "/api/projects/"+p.ID.String()+"/questions/"+fq.ID.String()+"/answer", me, `{"text":"x"}`); r.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-project question: %d %s", r.StatusCode, r.Body)
	}

	// The task detail lists the task's questions in every state.
	var detail struct {
		Questions []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"questions"`
	}
	getJSON("/api/projects/"+p.ID.String()+"/tasks/"+tk.ID.String(), me, &detail)
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
	sendJSON("POST", "/api/projects/"+p.ID.String()+"/tasks/"+tk.ID.String()+"/state", me, string([]byte(`{"state":"wont_do"}`)), &closed)
	if closed.ID != tk.ID.String() || closed.WithdrawnQuestions != 1 {
		t.Fatalf("close = %+v", closed)
	}
}

// The cross-project list covers the projects a person belongs to, even
// for an admin; one project can still be asked for by id.
func TestApiQuestions_ListScope(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	mk := func(id int64, login string) *identity.User {
		u, _, err := identity.UpsertFromGithub(ctx, pool, id, login, login, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	both, onlyA, admin := mk(15601, "q-both"), mk(15602, "q-a"), mk(15603, "q-admin")
	if _, err := pool.Exec(ctx, `UPDATE users SET is_admin = true WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	pa, err := identity.CreateProject(ctx, pool, "Scope A", "", "", "", both.ID)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := identity.CreateProject(ctx, pool, "Scope B", "", "", "", both.ID)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := identity.CreateProject(ctx, pool, "Scope C", "", "", "", onlyA.ID)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := identity.ListRoles(ctx, pool, pa.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("roles: %v %v", roles, err)
	}
	if err := identity.AddMembership(ctx, pool, onlyA.ID, pa.ID, roles[0].ID); err != nil {
		t.Fatal(err)
	}
	ask := func(p *identity.Project, asker *identity.User, body string) {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: body}, tasks.Authorship{UserID: &asker.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: tk.ID, AskedBy: tasks.Authorship{UserID: &asker.ID}, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	ask(pa, both, "in A for both")
	ask(pa, onlyA, "in A for onlyA")
	ask(pb, both, "in B")
	ask(pc, onlyA, "in C")

	key := []byte("k")
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)
	excerpts := func(u *identity.User, query string) map[string]bool {
		t.Helper()
		sess, err := identity.NewSession(ctx, pool, u.ID, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest("GET", ts.URL+"/api/questions?state=all"+query, nil)
		req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Questions []struct {
				Excerpt string `json:"excerpt"`
			} `json:"questions"`
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list %s: %d", query, resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, q := range out.Questions {
			got[q.Excerpt] = true
		}
		return got
	}
	check := func(name string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
		}
	}
	check("member of A and B sees both", excerpts(both, ""), "in A for both", "in A for onlyA", "in B")
	check("member of A only", excerpts(onlyA, ""), "in A for both", "in A for onlyA", "in C")
	check("admin, no membership, no project_id", excerpts(admin, ""))
	check("admin with project_id", excerpts(admin, "&project_id="+pc.ID.String()), "in C")
	check("recipient=me", excerpts(onlyA, "&recipient=me"), "in A for onlyA", "in C")
}
