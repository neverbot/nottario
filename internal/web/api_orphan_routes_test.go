package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// The inconsistencies endpoint exists to surface work that drifted:
// a task that depends on another one, but is already done while its
// precondition is open again. That state is reachable by reopening a
// finished precondition, which is exactly how it happens in real use.
func TestApiTasks_Inconsistencies(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 13801, "incons", "Incons", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "Incons", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "incons-token", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	first, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "groundwork"}, by)
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "builds on it"}, by)
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if err := tasks.AddDependency(ctx, pool, second.ID, first.ID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	ts := httptest.NewServer(NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token
	url := ts.URL + "/api/projects/" + p.ID.String() + "/tasks/inconsistencies"

	read := func() []map[string]any {
		t.Helper()
		r := doRaw(t, "GET", url, auth, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("inconsistencies: %d %s", r.StatusCode, r.Body)
		}
		var body struct {
			Inconsistencies []map[string]any `json:"inconsistencies"`
		}
		if err := json.Unmarshal(r.Body, &body); err != nil {
			t.Fatalf("decode %s: %v", r.Body, err)
		}
		return body.Inconsistencies
	}

	if got := read(); len(got) != 0 {
		t.Fatalf("a healthy backlog reports %v", got)
	}

	// Finish both, then reopen the precondition: the dependent is now
	// done on top of work that is open again.
	if _, err := tasks.SetState(ctx, pool, first.ID, tasks.StateDone, &u.ID); err != nil {
		t.Fatalf("SetState first done: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, second.ID, tasks.StateDone, &u.ID); err != nil {
		t.Fatalf("SetState second done: %v", err)
	}
	if _, err := tasks.SetState(ctx, pool, first.ID, tasks.StateTodo, &u.ID); err != nil {
		t.Fatalf("reopen first: %v", err)
	}

	got := read()
	if len(got) != 1 {
		t.Fatalf("reopening a finished precondition should surface one inconsistency, got %v", got)
	}
	if got[0]["task_id"] != first.ID.String() {
		t.Errorf("inconsistency points at %v, want the reopened task %s", got[0]["task_id"], first.ID)
	}
	if !strings.Contains(string(mustJSON(got[0])), second.ID.String()) {
		t.Errorf("the report does not name the dependent that is already done: %v", got[0])
	}

	// An outsider's token cannot read another project's drift.
	other, _, _ := identity.UpsertFromGithub(ctx, pool, 13802, "incons-out", "Out", "")
	otherProj, _ := identity.CreateProject(ctx, pool, "Incons Out", "", "", "", other.ID)
	otherToken, _, _ := identity.IssueToken(ctx, pool, other.ID, otherProj.ID, "out", nil)
	if r := doRaw(t, "GET", url, "Bearer "+otherToken, nil); r.StatusCode == http.StatusOK {
		t.Errorf("a foreign token read the inconsistencies: %s", r.Body)
	}
}

// Architecture revisions are the diagram's history: a checkpoint
// freezes the current graph under a version number, and this route
// serves one back. It is what the history view reads.
func TestApiArch_GetRevision(t *testing.T) {
	f := setupArch(t)

	// A node, so the checkpoint has something to freeze.
	r := doRaw(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "gateway", "name": "Gateway", "kind": "service",
	}))
	if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusCreated {
		t.Fatalf("create node: %d %s", r.StatusCode, r.Body)
	}
	// Checkpointing is an agent operation with no HTTP route, so the
	// test takes it the way the MCP tool does.
	if _, err := arch.Checkpoint(t.Context(), f.pool, uuid.MustParse(f.projectID),
		arch.Authorship{UserID: f.ownerID}, "first cut"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	hist := doRaw(t, "GET", f.url("/history"), f.authOwner, nil)
	if hist.StatusCode != http.StatusOK {
		t.Fatalf("history: %d %s", hist.StatusCode, hist.Body)
	}
	if !strings.Contains(string(hist.Body), "first cut") {
		t.Fatalf("history does not list the checkpoint: %s", hist.Body)
	}

	rev := doRaw(t, "GET", f.url("/revisions/1"), f.authOwner, nil)
	if rev.StatusCode != http.StatusOK {
		t.Fatalf("revision 1: %d %s", rev.StatusCode, rev.Body)
	}
	if !strings.Contains(string(rev.Body), "gateway") {
		t.Errorf("revision 1 does not contain the node that existed when it was taken: %s", rev.Body)
	}

	for _, tc := range []struct {
		name, path string
		want       int
	}{
		{"a version that was never taken", "/revisions/99", http.StatusNotFound},
		{"not a number", "/revisions/latest", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := doRaw(t, "GET", f.url(tc.path), f.authOwner, nil)
			if r.StatusCode != tc.want {
				t.Errorf("%s = %d, want %d (%s)", tc.path, r.StatusCode, tc.want, r.Body)
			}
		})
	}

	if r := doRaw(t, "GET", f.url("/revisions/1"), f.authOutsider, nil); r.StatusCode == http.StatusOK {
		t.Errorf("an outsider read the revision: %s", r.Body)
	}
}
