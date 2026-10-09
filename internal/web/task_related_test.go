package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// relatedView is the related block both the web detail and MCP
// tasks.get return, reduced to the titles of each relation so a test
// reads as the graph it built.
type relatedView struct {
	Parent *struct {
		Title string `json:"title"`
		State string `json:"state"`
		Type  string `json:"type"`
	} `json:"parent"`
	Children  []struct{ Title string } `json:"children"`
	DependsOn []struct{ Title string } `json:"depends_on"`
	Blocks    []struct{ Title string } `json:"blocks"`
}

func titles(rows []struct{ Title string }) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

// checkRelated asserts one task's related block against the graph the
// tests build: feature F with children A and B, A depends on C, D
// depends on A.
func checkRelated(t *testing.T, where string, got relatedView, parent string, children, dependsOn, blocks []string) {
	t.Helper()
	switch {
	case parent == "" && got.Parent != nil:
		t.Errorf("%s: parent = %q, want none", where, got.Parent.Title)
	case parent != "" && got.Parent == nil:
		t.Errorf("%s: no parent, want %q", where, parent)
	case parent != "" && (got.Parent.Title != parent || got.Parent.Type != "feature" || got.Parent.State == ""):
		t.Errorf("%s: parent = %+v, want feature %q with a state", where, *got.Parent, parent)
	}
	for _, c := range []struct {
		name      string
		got, want []string
	}{
		{"children", titles(got.Children), children},
		{"depends_on", titles(got.DependsOn), dependsOn},
		{"blocks", titles(got.Blocks), blocks},
	} {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: %s = %v, want %v", where, c.name, c.got, c.want)
		}
	}
}

// The task dialog shows how a task connects to the others without a
// request per related task: the detail carries the parent feature, the
// subtasks, what the task waits for and what waits for it.
func TestApiTasks_DetailCarriesRelatedTasks(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14311, "detail-related", "Related", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Detail related", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string, typ tasks.Type, parent *uuid.UUID) uuid.UUID {
		t.Helper()
		x, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: typ, Title: title, ParentTaskID: parent}, by)
		if err != nil {
			t.Fatalf("Create %s: %v", title, err)
		}
		return x.ID
	}
	f := mk("F", tasks.TypeFeature, nil)
	a := mk("A", tasks.TypeTask, &f)
	mk("B", tasks.TypeTask, &f)
	c := mk("C", tasks.TypeTask, nil)
	d := mk("D", tasks.TypeTask, nil)
	if err := tasks.AddDependency(ctx, pool, a, c); err != nil {
		t.Fatal(err)
	}
	if err := tasks.AddDependency(ctx, pool, d, a); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	get := func(id uuid.UUID) relatedView {
		t.Helper()
		r := doRaw(t, "GET", ts.URL+"/api/projects/"+p.ID.String()+"/tasks/"+id.String(), "Bearer "+token, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("detail: %d %s", r.StatusCode, r.Body)
		}
		var body struct {
			Related *relatedView `json:"related"`
		}
		if err := json.Unmarshal(r.Body, &body); err != nil {
			t.Fatal(err)
		}
		if body.Related == nil {
			t.Fatalf("detail has no related block: %s", r.Body)
		}
		return *body.Related
	}

	checkRelated(t, "A", get(a), "F", []string{}, []string{"C"}, []string{"D"})
	checkRelated(t, "F", get(f), "", []string{"A", "B"}, []string{}, []string{})
	checkRelated(t, "C", get(c), "", []string{}, []string{}, []string{"A"})
}

// Agents get the same picture from tasks.get on every call, without
// asking for it.
func TestMCP_TasksGet_AlwaysReturnsRelated(t *testing.T) {
	f := newMCPFixture(t, 14312, "mcp-related")
	mk := func(title, typ, parent string) string {
		t.Helper()
		args := map[string]any{"project_id": f.projectID, "title": title, "type": typ}
		if parent != "" {
			args["parent_task_id"] = parent
		}
		var out struct {
			ID string `json:"id"`
		}
		f.callJSON(t, "nottario.tasks.create", args, &out)
		return out.ID
	}
	dep := func(task, on string) {
		t.Helper()
		f.callJSON(t, "nottario.tasks.add_dependency", map[string]any{
			"project_id": f.projectID, "task_id": task, "depends_on_id": on,
		}, nil)
	}
	feature := mk("F", "feature", "")
	a := mk("A", "task", feature)
	mk("B", "task", feature)
	c := mk("C", "task", "")
	d := mk("D", "task", "")
	dep(a, c)
	dep(d, a)

	get := func(id string) relatedView {
		t.Helper()
		var out struct {
			Related *relatedView `json:"related"`
		}
		f.callJSON(t, "nottario.tasks.get", map[string]any{"project_id": f.projectID, "task_id": id}, &out)
		if out.Related == nil {
			t.Fatal("tasks.get returned no related block")
		}
		return *out.Related
	}
	checkRelated(t, "A", get(a), "F", []string{}, []string{"C"}, []string{"D"})
	checkRelated(t, "F", get(feature), "", []string{"A", "B"}, []string{}, []string{})

	// include_deps is gone: related replaces it.
	f.callExpectErr(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID, "task_id": a, "include_deps": true,
	})
}
