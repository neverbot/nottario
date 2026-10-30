package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestExcerptOfStripsMarkdown(t *testing.T) {
	cases := map[string]string{
		"Which **one** do you want?":                    "Which one do you want?",
		"use __bold__ and *em* and _under_ here":        "use bold and em and under here",
		"run `go test` first":                           "run go test first",
		"## Heading\nthen text":                         "Heading then text",
		"see [the docs](https://x.test/a_b) now":        "see the docs now",
		"relates to [[task:abc|Fix login]] and [[doc]]": "relates to Fix login and doc",
		"see [[task:abc]] now":                          "see task:abc now",
		"*a* *b* and _c_ _d_":                           "a b and c d",
		"snake_case_name and 2*3*4 stay":                "snake_case_name and 2*3*4 stay",
		"  spaced \n\n out  ":                           "spaced out",
	}
	for in, want := range cases {
		if got := excerptOf(in); got != want {
			t.Errorf("excerptOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApiLive(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 15701, "live-api", "Live API", "")
	if err != nil {
		t.Fatal(err)
	}
	stranger, _, err := identity.UpsertFromGithub(ctx, pool, 15702, "live-api-stranger", "Stranger", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := identity.UpsertFromGithub(ctx, pool, 15703, "live-api-admin", "Admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET is_admin = true WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Live api", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := identity.CreateProject(ctx, pool, "Live foreign", "", "", "", stranger.ID)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("roles: %v %v", roles, err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	role := roles[0]
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "in flight", TargetRoleID: &role.ID}, by)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := tasks.AddSteps(ctx, pool, tk.ID, []string{"s one", "s two"}, by)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: "conv-9", Label: "Agent nine", Client: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.StartStep(ctx, pool, tk.ID, steps[0].ID, &sess.ID, by); err != nil {
		t.Fatal(err)
	}
	done, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "just done"}, by)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.SetState(ctx, pool, done.ID, tasks.StateDoing, &u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.SetState(ctx, pool, done.ID, tasks.StateDone, &u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: tk.ID, StepID: &steps[0].ID, SessionID: &sess.ID, AskedBy: by, Body: "Which **one**?"}); err != nil {
		t.Fatal(err)
	}
	ftk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: foreign.ID, Type: tasks.TypeTask, Title: "theirs"}, tasks.Authorship{UserID: &stranger.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.SetState(ctx, pool, ftk.ID, tasks.StateDoing, &stranger.ID); err != nil {
		t.Fatal(err)
	}

	key := []byte("k")
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)
	call := func(method, path string, uid uuid.UUID, body string) *rawResp {
		t.Helper()
		sess, err := identity.NewSession(ctx, pool, uid, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rd)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return &rawResp{StatusCode: resp.StatusCode, Body: raw}
	}

	type liveResp struct {
		Now      string `json:"now"`
		Projects []struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			Slug             string `json:"slug"`
			IdleAfterMinutes int    `json:"idle_after_minutes"`
		} `json:"projects"`
		Waiting []struct {
			Excerpt string `json:"excerpt"`
			ForMe   bool   `json:"for_me"`
			Task    *struct {
				Title string `json:"title"`
			} `json:"task"`
		} `json:"waiting"`
		Working []struct {
			Task struct {
				ID        string `json:"id"`
				Title     string `json:"title"`
				Type      string `json:"type"`
				ProjectID string `json:"project_id"`
			} `json:"task"`
			Role *struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"role"`
			Assignee *struct {
				Login       string `json:"login"`
				DisplayName string `json:"display_name"`
			} `json:"assignee"`
			StartedAt      *string `json:"started_at"`
			LastActivityAt string  `json:"last_activity_at"`
			Idle           bool    `json:"idle"`
			Session        *struct {
				Key    string `json:"key"`
				Label  string `json:"label"`
				Client string `json:"client"`
			} `json:"session"`
			Steps []struct {
				Position int    `json:"position"`
				Text     string `json:"text"`
				Session  *struct {
					Key string `json:"key"`
				} `json:"session"`
			} `json:"steps"`
			Progress *struct {
				Done  int `json:"done"`
				Total int `json:"total"`
			} `json:"progress"`
			QuestionsOpen int `json:"questions_open"`
		} `json:"working"`
		Truncated struct {
			Working, Finished, Waiting bool
		} `json:"truncated"`
		Finished []struct {
			Task struct {
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"task"`
			State   string `json:"state"`
			EndedAt string `json:"ended_at"`
		} `json:"finished"`
	}
	get := func(path string, uid uuid.UUID) liveResp {
		t.Helper()
		r := call("GET", path, uid, "")
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, r.StatusCode, r.Body)
		}
		var out liveResp
		if err := json.Unmarshal(r.Body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	lv := get("/api/live", u.ID)
	if lv.Now == "" || len(lv.Projects) != 1 || lv.Projects[0].Slug != p.Slug || lv.Projects[0].IdleAfterMinutes != 30 {
		t.Fatalf("projects = %+v", lv.Projects)
	}
	if len(lv.Working) != 1 {
		t.Fatalf("working = %+v", lv.Working)
	}
	w := lv.Working[0]
	if w.Task.Title != "in flight" || w.Task.Type != "task" || w.Task.ProjectID != p.ID.String() ||
		w.Role == nil || w.Role.Key != role.Key || w.Role.Name != role.Label ||
		w.Assignee == nil || w.Assignee.Login != "live-api" || w.Assignee.DisplayName != "Live API" || !strings.HasSuffix(w.LastActivityAt, "Z") || lv.Truncated.Working || lv.Truncated.Finished || lv.Truncated.Waiting || w.StartedAt == nil || w.LastActivityAt == "" || w.Idle ||
		w.Session == nil || w.Session.Key != "conv-9" || w.Session.Label != "Agent nine" ||
		len(w.Steps) != 1 || w.Steps[0].Text != "s one" || w.Steps[0].Session == nil || w.Steps[0].Session.Key != "conv-9" ||
		w.Progress == nil || w.Progress.Total != 2 || w.QuestionsOpen != 1 {
		t.Fatalf("working item = %+v", w)
	}
	if len(lv.Waiting) != 1 || lv.Waiting[0].Excerpt != "Which one?" || !lv.Waiting[0].ForMe || lv.Waiting[0].Task == nil {
		t.Fatalf("waiting = %+v", lv.Waiting)
	}
	if len(lv.Finished) != 1 || lv.Finished[0].Task.Title != "just done" || lv.Finished[0].Task.Type != "task" || lv.Finished[0].State != "done" || lv.Finished[0].EndedAt == "" {
		t.Fatalf("finished = %+v", lv.Finished)
	}

	// Scoping: a stranger sees only their own project; a project they
	// are not in is refused; an admin without membership sees nothing
	// across projects but can ask for one.
	sv := get("/api/live", stranger.ID)
	if len(sv.Working) != 1 || sv.Working[0].Task.Title != "theirs" || len(sv.Waiting) != 0 {
		t.Fatalf("stranger = %+v", sv)
	}
	if r := call("GET", "/api/live?project_id="+p.ID.String(), stranger.ID, ""); r.StatusCode == http.StatusOK {
		t.Fatalf("foreign project: %d %s", r.StatusCode, r.Body)
	}
	if r := call("GET", "/api/live?project_id=nope", u.ID, ""); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad project id: %d", r.StatusCode)
	}
	if av := get("/api/live", admin.ID); len(av.Working) != 0 || len(av.Projects) != 0 {
		t.Fatalf("admin across projects = %+v", av)
	}
	if av := get("/api/live?project_id="+p.ID.String(), admin.ID); len(av.Working) != 1 || len(av.Waiting) != 0 {
		t.Fatalf("admin one project = %+v", av)
	}

	// The idle setting: read in the project payload, written through
	// the update handler, validated.
	var proj struct {
		LiveIdleMinutes int `json:"live_idle_minutes"`
	}
	r := call("GET", "/api/projects/"+p.ID.String(), u.ID, "")
	if err := json.Unmarshal(r.Body, &proj); err != nil || proj.LiveIdleMinutes != 30 {
		t.Fatalf("project payload: %s", r.Body)
	}
	patch := `{"name":"Live api","description":"","live_idle_minutes":%d}`
	for _, bad := range []string{"4", "1441"} {
		r := call("PATCH", "/api/projects/"+p.ID.String(), admin.ID, strings.Replace(patch, "%d", bad, 1))
		var e struct {
			Code string `json:"code"`
		}
		if r.StatusCode != http.StatusBadRequest || json.Unmarshal(r.Body, &e) != nil || e.Code != "live_idle_minutes" {
			t.Fatalf("live_idle_minutes %s: %d %s", bad, r.StatusCode, r.Body)
		}
	}
	r = call("PATCH", "/api/projects/"+p.ID.String(), admin.ID, strings.Replace(patch, "%d", "90", 1))
	if r.StatusCode != http.StatusOK || json.Unmarshal(r.Body, &proj) != nil || proj.LiveIdleMinutes != 90 {
		t.Fatalf("update: %d %s", r.StatusCode, r.Body)
	}
	// Omitting the field leaves it alone.
	r = call("PATCH", "/api/projects/"+p.ID.String(), admin.ID, `{"name":"Live api","description":""}`)
	if r.StatusCode != http.StatusOK || json.Unmarshal(r.Body, &proj) != nil || proj.LiveIdleMinutes != 90 {
		t.Fatalf("update without the field: %d %s", r.StatusCode, r.Body)
	}
	if got := get("/api/live?project_id="+p.ID.String(), u.ID); got.Projects[0].IdleAfterMinutes != 90 {
		t.Fatalf("idle_after_minutes = %d", got.Projects[0].IdleAfterMinutes)
	}
	// live and questions are valid landing views now.
	for _, v := range []string{"live", "questions"} {
		if r := call("PATCH", "/api/projects/"+p.ID.String(), admin.ID, `{"name":"Live api","description":"","default_view":"`+v+`"}`); r.StatusCode != http.StatusOK {
			t.Fatalf("default_view %s: %d %s", v, r.StatusCode, r.Body)
		}
	}
}
