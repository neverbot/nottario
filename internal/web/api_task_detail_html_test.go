package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// Opening a task must be one request. The dialog used to render its
// description and every comment with a separate markdown request, so a
// long discussion fired dozens of requests that queued behind the
// browser's few connections and left blocks stuck on "Rendering…".
func TestApiTasks_DetailCarriesRenderedHTML(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14301, "detail-html", "Detail", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "Detail HTML", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	task, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "discussed at length",
		DescriptionMD: "# Plan\n\nSome **bold** text.\n\n<script>alert(1)</script>\n",
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	const comments = 12
	for i := 0; i < comments; i++ {
		if _, err := tasks.AddComment(ctx, pool, task.ID, "comment with *emphasis*", by); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	r := doRaw(t, "GET", ts.URL+"/api/projects/"+p.ID.String()+"/tasks/"+task.ID.String(), "Bearer "+token, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("detail: %d %s", r.StatusCode, r.Body)
	}
	var detail struct {
		DescriptionHTML string `json:"description_html"`
		Comments        []struct {
			Body     string `json:"body"`
			BodyHTML string `json:"body_html"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(r.Body, &detail); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(detail.DescriptionHTML, "<strong>bold</strong>") {
		t.Errorf("description not rendered: %q", detail.DescriptionHTML)
	}
	if strings.Contains(detail.DescriptionHTML, "<script") {
		t.Errorf("rendered description is not sanitised: %q", detail.DescriptionHTML)
	}
	if len(detail.Comments) != comments {
		t.Fatalf("got %d comments, want %d", len(detail.Comments), comments)
	}
	for i, c := range detail.Comments {
		if !strings.Contains(c.BodyHTML, "<em>emphasis</em>") {
			t.Errorf("comment %d not rendered: %q", i, c.BodyHTML)
		}
		// The markdown is still there, for editing.
		if c.Body == "" {
			t.Errorf("comment %d lost its markdown", i)
		}
	}
}

// Agents read markdown; HTML would only inflate what every MCP call
// costs. The rendering is a web-dialog concern and stays there.
func TestMCP_TaskGetStaysMarkdown(t *testing.T) {
	f := newMCPFixture(t, 13395, "task-get-markdown")
	var created map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "t", "description": "**bold**",
	}, &created)
	f.callJSON(t, "nottario.tasks.add_comment", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "body": "*hi*",
	}, nil)
	var got map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "include_comments": true,
	}, &got)
	raw := mustJSONString(got)
	if strings.Contains(raw, "_html") || strings.Contains(raw, "<strong>") {
		t.Errorf("MCP task payload carries rendered HTML: %s", raw)
	}
}
