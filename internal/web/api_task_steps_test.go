package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

type stepView struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Done     bool   `json:"done"`
}

func stepViewTexts(steps []stepView) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Text)
	}
	return out
}

// The task dialog edits a task's checklist through four routes and
// reads it back in the task detail; the board reads the progress off
// every task row.
func TestApiTaskSteps(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14401, "web-steps", "Steps", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Web steps", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "with steps"}, by)
	if err != nil {
		t.Fatal(err)
	}
	other, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "other"}, by)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token
	base := ts.URL + "/api/projects/" + p.ID.String() + "/tasks/"
	stepsURL := base + tk.ID.String() + "/steps"

	call := func(method, url, body string, want int) []byte {
		t.Helper()
		var b []byte
		if body != "" {
			b = []byte(body)
		}
		r := doRaw(t, method, url, auth, b)
		if r.StatusCode != want {
			t.Fatalf("%s %s: %d %s, want %d", method, url, r.StatusCode, r.Body, want)
		}
		return r.Body
	}
	list := func(raw []byte) []stepView {
		t.Helper()
		var out struct {
			Steps []stepView `json:"steps"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out.Steps
	}

	steps := list(call("POST", stepsURL, `{"texts":["write test","make it pass","commit"]}`, http.StatusCreated))
	if want := []string{"write test", "make it pass", "commit"}; !reflect.DeepEqual(stepViewTexts(steps), want) {
		t.Fatalf("after add: %v", stepViewTexts(steps))
	}

	var ticked stepView
	if err := json.Unmarshal(call("PATCH", stepsURL+"/"+steps[0].ID, `{"done":true}`, http.StatusOK), &ticked); err != nil {
		t.Fatal(err)
	}
	if !ticked.Done {
		t.Errorf("tick: %+v", ticked)
	}

	moved := list(call("PUT", stepsURL+"/"+steps[2].ID+"/position", `{"position":0}`, http.StatusOK))
	if want := []string{"commit", "write test", "make it pass"}; !reflect.DeepEqual(stepViewTexts(moved), want) {
		t.Errorf("after move: %v", stepViewTexts(moved))
	}
	call("DELETE", stepsURL+"/"+steps[1].ID, "", http.StatusNoContent)

	// The detail carries the list and the task row the progress.
	var detail struct {
		Task  tasks.Task `json:"task"`
		Steps []stepView `json:"steps"`
	}
	if err := json.Unmarshal(call("GET", base+tk.ID.String(), "", http.StatusOK), &detail); err != nil {
		t.Fatal(err)
	}
	if want := []string{"commit", "write test"}; !reflect.DeepEqual(stepViewTexts(detail.Steps), want) {
		t.Errorf("detail steps: %v", stepViewTexts(detail.Steps))
	}
	if detail.Task.Steps == nil || *detail.Task.Steps != (tasks.StepProgress{Done: 1, Total: 2}) {
		t.Errorf("detail progress: %+v", detail.Task.Steps)
	}

	var rows struct {
		Tasks []tasks.Task `json:"tasks"`
	}
	if err := json.Unmarshal(call("GET", ts.URL+"/api/projects/"+p.ID.String()+"/tasks", "", http.StatusOK), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows.Tasks) != 2 {
		t.Fatalf("list has %d tasks, want 2", len(rows.Tasks))
	}
	for _, row := range rows.Tasks {
		if row.ID == tk.ID && (row.Steps == nil || row.Steps.Total != 2) {
			t.Errorf("list row progress: %+v", row.Steps)
		}
		if row.ID == other.ID && row.Steps != nil {
			t.Errorf("task without steps has progress %+v", row.Steps)
		}
	}

	// Rejections.
	call("POST", stepsURL, `{"texts":["two\nlines"]}`, http.StatusBadRequest)
	call("POST", stepsURL, `{"texts":[]}`, http.StatusBadRequest)
	call("PATCH", stepsURL+"/"+steps[0].ID, `{}`, http.StatusBadRequest)
	call("PUT", stepsURL+"/"+steps[0].ID+"/position", `{}`, http.StatusBadRequest)
	call("PATCH", stepsURL+"/not-a-uuid", `{"done":true}`, http.StatusBadRequest)
	// A step addressed through another task is not found.
	otherSteps := base + other.ID.String() + "/steps/" + steps[0].ID
	call("PATCH", otherSteps, `{"done":false}`, http.StatusNotFound)
	call("DELETE", otherSteps, "", http.StatusNotFound)
	call("PUT", otherSteps+"/position", `{"position":0}`, http.StatusNotFound)
}
