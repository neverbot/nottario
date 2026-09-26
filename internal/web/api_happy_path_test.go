package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/testutil"
)

// walk drives the API the way the web app does: one signed-in person,
// a session cookie, JSON in and JSON out. Each step asserts the
// success path of a route and, where it is cheap, that the change is
// really there afterwards.
//
// The scope battery next door proves the API keeps strangers out.
// This proves it lets the owner in, which is the half a security test
// cannot see: a server that refused everything would pass that one.
type walk struct {
	t      *testing.T
	ts     *httptest.Server
	cookie *http.Cookie
	base   string
}

type step struct {
	status int
	body   []byte
}

func (s step) decode(t *testing.T, dst any) {
	t.Helper()
	if err := json.Unmarshal(s.body, dst); err != nil {
		t.Fatalf("decode %s: %v", s.body, err)
	}
}

func (s step) contains(sub string) bool { return strings.Contains(string(s.body), sub) }

// call performs the request and fails the test unless the status is
// 2xx: every call in this file is a documented success path.
func (w *walk) call(method, path string, body any) step {
	w.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			w.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(w.t.Context(), method, w.ts.URL+path, reader)
	if err != nil {
		w.t.Fatalf("%s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(w.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.t.Fatalf("%s %s = %d, want 2xx: %s", method, path, resp.StatusCode, raw)
	}
	return step{status: resp.StatusCode, body: raw}
}

func (w *walk) project(path string) string { return w.base + path }

func newWalk(t *testing.T) (*walk, uuid.UUID, uuid.UUID) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")

	me, _, err := identity.UpsertFromGithub(ctx, pool, 14201, "walker", "Walker", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	mate, _, err := identity.UpsertFromGithub(ctx, pool, 14202, "teammate", "Teammate", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub mate: %v", err)
	}
	sess, err := identity.NewSession(ctx, pool, me.ID, "walkthrough", "127.0.0.1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	hub := realtime.New(nil)
	ts := httptest.NewServer(NewServer(Deps{
		Pool:                 pool,
		Resolver:             identity.NewResolver(pool, key, false),
		Hub:                  hub,
		Notifier:             notifications.New(pool, hub, nil, true),
		NotificationsEnabled: true,
	}))
	t.Cleanup(ts.Close)
	return &walk{
		t:      t,
		ts:     ts,
		cookie: &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)},
	}, me.ID, mate.ID
}

// One person creates a project and configures it: the settings pages,
// end to end.
func TestHappyPath_ProjectSetup(t *testing.T) {
	w, meID, mateID := newWalk(t)

	created := w.call("POST", "/api/projects", map[string]any{
		"name": "Walkthrough", "description": "created by the happy-path test",
		"primary_language": "Go", "project_type": "service",
	})
	var project struct {
		ID   uuid.UUID `json:"id"`
		Slug string    `json:"slug"`
	}
	created.decode(t, &project)
	if project.ID == uuid.Nil || project.Slug == "" {
		t.Fatalf("create project returned %s", created.body)
	}
	w.base = "/api/projects/" + project.ID.String()

	if list := w.call("GET", "/api/projects", nil); !list.contains("Walkthrough") {
		t.Errorf("the new project is missing from the listing: %s", list.body)
	}
	if one := w.call("GET", w.base, nil); !one.contains(project.Slug) {
		t.Errorf("reading the project back: %s", one.body)
	}
	// Reachable by slug as well as by uuid, which is what the app's
	// own links use.
	w.call("GET", "/api/projects/"+project.Slug, nil)

	w.call("PATCH", w.base, map[string]any{
		"name": "Walkthrough Renamed", "description": "d", "primary_language": "Go",
		"project_type": "service", "default_view": "board/kanban", "cycle_label": "sprint",
	})
	w.call("PATCH", w.project("/mcp"), map[string]any{"mcp_page_size": 25})
	w.call("PATCH", w.project("/default_view"), map[string]any{"default_view": "board/gantt"})
	after := w.call("GET", w.base, nil)
	if !after.contains("Walkthrough Renamed") || !after.contains("board/gantt") {
		t.Errorf("the project settings did not stick: %s", after.body)
	}

	// Roles: the project starts with four seeded ones.
	roles := w.call("GET", w.project("/roles"), nil)
	var roleList struct {
		Roles []struct {
			ID  uuid.UUID `json:"id"`
			Key string    `json:"key"`
		} `json:"roles"`
	}
	roles.decode(t, &roleList)
	if len(roleList.Roles) < 4 {
		t.Fatalf("a new project should come with its roles seeded: %s", roles.body)
	}
	newRole := w.call("POST", w.project("/roles"), map[string]any{
		"key": "ops", "label": "Ops", "color": "#334455",
	})
	var role struct {
		ID uuid.UUID `json:"id"`
	}
	newRole.decode(t, &role)
	w.call("PATCH", w.project("/roles/"+role.ID.String()), map[string]any{
		"key": "ops", "label": "Operations", "color": "#445566",
	})
	ids := []string{role.ID.String()}
	for _, r := range roleList.Roles {
		ids = append(ids, r.ID.String())
	}
	w.call("POST", w.project("/roles/reorder"), map[string]any{"role_ids": ids})
	if renamed := w.call("GET", w.project("/roles"), nil); !renamed.contains("Operations") {
		t.Errorf("the role rename did not stick: %s", renamed.body)
	}
	w.call("DELETE", w.project("/roles/"+role.ID.String()), nil)

	// Priorities.
	w.call("GET", w.project("/priorities"), nil)
	w.call("POST", w.project("/priorities"), map[string]any{"key": "urgent", "value": 95, "position": 4})
	if p := w.call("GET", w.project("/priorities"), nil); !p.contains("urgent") {
		t.Errorf("the new priority bucket is missing: %s", p.body)
	}
	w.call("DELETE", w.project("/priorities/urgent"), nil)

	// Members: add a teammate, give them a role, then take it away.
	w.call("GET", w.project("/members"), nil)
	w.call("POST", w.project("/members"), map[string]any{
		"user_id": mateID.String(), "role_id": roleList.Roles[0].ID.String(),
	})
	members := w.call("GET", w.project("/members"), nil)
	if !members.contains("teammate") {
		t.Errorf("the teammate was not added: %s", members.body)
	}
	w.call("DELETE", w.project("/members/"+mateID.String()+"/"+roleList.Roles[0].ID.String()), nil)
	w.call("DELETE", w.project("/members/"+mateID.String()), nil)

	// Ownership can be handed over and taken back.
	w.call("PATCH", w.project("/owner"), map[string]any{"owner_user_id": mateID.String()})
	w.call("PATCH", w.project("/owner"), map[string]any{"owner_user_id": meID.String()})

	// API tokens for agents.
	issued := w.call("POST", "/api/projects/"+project.ID.String()+"/tokens", map[string]any{"name": "an agent"})
	var token struct {
		Plaintext string `json:"plaintext"`
		Token     struct {
			ID uuid.UUID `json:"id"`
		} `json:"token"`
	}
	issued.decode(t, &token)
	// The secret is shown exactly once, here, and stored hashed.
	if !strings.HasPrefix(token.Plaintext, identity.TokenPrefix) {
		t.Errorf("issuing a token did not return the secret once: %s", issued.body)
	}
	if listed := w.call("GET", "/api/projects/"+project.ID.String()+"/tokens", nil); !listed.contains("an agent") {
		t.Errorf("token listing: %s", listed.body)
	}
	w.call("DELETE", "/api/projects/"+project.ID.String()+"/tokens/"+token.Token.ID.String(), nil)

	// Finally, the project can be deleted.
	w.call("DELETE", w.base, nil)
}

// The board: a task through its whole life, with the panels the
// detail dialog shows.
func TestHappyPath_TaskLifecycle(t *testing.T) {
	w, meID, _ := newWalk(t)
	created := w.call("POST", "/api/projects", map[string]any{"name": "Board Walk"})
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	created.decode(t, &project)
	w.base = "/api/projects/" + project.ID.String()

	var roleList struct {
		Roles []struct {
			ID uuid.UUID `json:"id"`
		} `json:"roles"`
	}
	w.call("GET", w.project("/roles"), nil).decode(t, &roleList)

	newTask := func(title string) uuid.UUID {
		t.Helper()
		var task struct {
			ID uuid.UUID `json:"id"`
		}
		w.call("POST", w.project("/tasks"), map[string]any{
			"title": title, "type": "task", "priority_key": "high",
			"target_role_id": roleList.Roles[0].ID.String(),
		}).decode(t, &task)
		if task.ID == uuid.Nil {
			t.Fatalf("creating %q returned no id", title)
		}
		return task.ID
	}

	groundwork := newTask("groundwork")
	feature := newTask("the feature itself")

	// Dependencies: the feature waits for the groundwork.
	w.call("POST", w.project("/tasks/"+feature.String()+"/dependencies"), map[string]any{
		"depends_on_id": groundwork.String(),
	})
	deps := w.call("GET", w.project("/tasks/dependencies"), nil)
	if !deps.contains(groundwork.String()) {
		t.Errorf("the dependency is not listed: %s", deps.body)
	}
	// "What should I do next" skips the blocked one.
	next := w.call("GET", w.project("/tasks/next"), nil)
	if next.contains(feature.String()) {
		t.Errorf("next offered a blocked task: %s", next.body)
	}

	// Editing: fields, then the text pair with its optimistic check.
	w.call("PATCH", w.project("/tasks/"+groundwork.String()), map[string]any{
		"assignee_user_id": meID.String(), "priority": 70,
	})
	one := w.call("GET", w.project("/tasks/"+groundwork.String()), nil)
	var detail struct {
		Task struct {
			UpdatedAt string `json:"updated_at"`
			Priority  int    `json:"priority"`
		} `json:"task"`
	}
	one.decode(t, &detail)
	if detail.Task.Priority != 70 {
		t.Errorf("priority = %d after the patch", detail.Task.Priority)
	}
	w.call("PATCH", w.project("/tasks/"+groundwork.String()+"/text"), map[string]any{
		"title": "groundwork, clarified", "description": "with a body now",
		"expected_updated_at": detail.Task.UpdatedAt,
	})

	// Comments.
	comment := w.call("POST", w.project("/tasks/"+groundwork.String()+"/comments"), map[string]any{
		"body": "first note",
	})
	var created2 struct {
		ID uuid.UUID `json:"id"`
	}
	comment.decode(t, &created2)
	w.call("PATCH", w.project("/tasks/"+groundwork.String()+"/comments/"+created2.ID.String()), map[string]any{
		"body": "first note, edited",
	})

	// Commits: link, then unlink.
	w.call("POST", w.project("/tasks/"+groundwork.String()+"/commits"), map[string]any{
		"repo": "neverbot/nottario", "sha": "abc1234", "message": "feat: groundwork",
	})
	withCommit := w.call("GET", w.project("/tasks/"+groundwork.String()), nil)
	if !withCommit.contains("abc1234") || !withCommit.contains("first note, edited") {
		t.Errorf("the task detail lost its commit or comment: %s", withCommit.body)
	}

	// State: finish the precondition, then the feature.
	w.call("POST", w.project("/tasks/"+groundwork.String()+"/state"), map[string]any{"state": "done"})
	w.call("POST", w.project("/tasks/"+feature.String()+"/state"), map[string]any{"state": "doing"})
	w.call("POST", w.project("/tasks/"+feature.String()+"/state"), map[string]any{"state": "done"})

	board := w.call("GET", w.project("/tasks?include_children=true"), nil)
	if !board.contains("the feature itself") {
		t.Errorf("the board listing lost a task: %s", board.body)
	}
	w.call("GET", w.project("/tasks/inconsistencies"), nil)

	// Cycles: where the board's sprint selector reads from.
	w.call("GET", w.project("/cycles"), nil)
	w.call("GET", w.project("/cycles/current"), nil)
	ended := w.call("POST", w.project("/cycles/end"), map[string]any{"next_name": "sprint-2"})
	if !ended.contains("sprint-2") {
		t.Errorf("ending the cycle did not open the next one: %s", ended.body)
	}

	// Tidy up the way a person would.
	w.call("DELETE", w.project("/tasks/"+feature.String()+"/dependencies/"+groundwork.String()), nil)
	w.call("DELETE", w.project("/tasks/"+groundwork.String()+"/commits"), map[string]any{
		"repo": "neverbot/nottario", "sha": "abc1234",
	})
	w.call("DELETE", w.project("/tasks/"+groundwork.String()+"/comments/"+created2.ID.String()), nil)
	w.call("DELETE", w.project("/tasks/"+feature.String()), nil)
}

// The architecture diagram: kinds, nodes, nesting, edges, links and
// the history behind them.
func TestHappyPath_ArchitectureDiagram(t *testing.T) {
	w, _, _ := newWalk(t)
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	w.call("POST", "/api/projects", map[string]any{"name": "Arch Walk"}).decode(t, &project)
	w.base = "/api/projects/" + project.ID.String()

	w.call("GET", w.project("/arch/kinds"), nil)
	w.call("POST", w.project("/arch/kinds"), map[string]any{
		"key": "datastore", "label": "Datastore", "color": "#2d6", "description": "state at rest",
	})

	var backend struct {
		Node struct {
			ID uuid.UUID `json:"id"`
		} `json:"node"`
		ID uuid.UUID `json:"id"`
	}
	w.call("POST", w.project("/arch/nodes"), map[string]any{
		"slug": "backend", "kind": "datastore", "name": "Backend",
		"description": "the Go binary",
	}).decode(t, &backend)
	backendID := backend.Node.ID
	if backendID == uuid.Nil {
		backendID = backend.ID
	}
	w.call("POST", w.project("/arch/nodes"), map[string]any{
		"slug": "db", "kind": "datastore", "name": "Postgres",
	})
	w.call("POST", w.project("/arch/nodes/db/move"), map[string]any{"parent_slug": "backend"})
	node := w.call("GET", w.project("/arch/nodes/db"), nil)
	if backendID != uuid.Nil && !node.contains(backendID.String()) {
		t.Errorf("the node was not reparented under %s: %s", backendID, node.body)
	}

	w.call("POST", w.project("/arch/edges"), map[string]any{
		"from_slug": "backend", "to_slug": "db", "kind": "datastore", "label": "reads and writes",
	})
	edges := w.call("GET", w.project("/arch/edges"), nil)
	var edgeList struct {
		Edges []struct {
			ID uuid.UUID `json:"id"`
		} `json:"edges"`
	}
	edges.decode(t, &edgeList)
	if len(edgeList.Edges) != 1 {
		t.Fatalf("edges = %s", edges.body)
	}

	// A node can point at a document and at a task.
	var task struct {
		ID uuid.UUID `json:"id"`
	}
	w.call("POST", w.project("/tasks"), map[string]any{"title": "wire the db", "type": "task"}).decode(t, &task)
	w.call("POST", "/api/docs/write", map[string]any{
		"scope": "project", "project_id": project.ID.String(),
		"path": "arch/db.md", "content": "# DB\n", "expected_version": 0, "message": "init",
	})
	w.call("POST", w.project("/arch/nodes/db/links"), map[string]any{"doc_path": "arch/db.md"})
	w.call("POST", w.project("/arch/nodes/db/links"), map[string]any{"task_id": task.ID.String()})
	linked := w.call("GET", w.project("/arch/nodes/db"), nil)
	if !linked.contains("arch/db.md") {
		t.Errorf("the document link is missing: %s", linked.body)
	}
	w.call("POST", w.project("/arch/nodes/db/unlinks"), map[string]any{"doc_path": "arch/db.md"})
	w.call("POST", w.project("/arch/nodes/db/unlinks"), map[string]any{"task_id": task.ID.String()})

	w.call("GET", w.project("/arch/nodes"), nil)
	w.call("GET", w.project("/arch/history"), nil)

	w.call("DELETE", w.project("/arch/edges/"+edgeList.Edges[0].ID.String()), nil)
	w.call("DELETE", w.project("/arch/nodes/db"), nil)
	w.call("DELETE", w.project("/arch/nodes/backend"), nil)
	w.call("DELETE", w.project("/arch/kinds/datastore"), nil)
}

// Documents: write, read, list, search, history, and the delete that
// keeps the history.
func TestHappyPath_Documents(t *testing.T) {
	w, _, _ := newWalk(t)
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	w.call("POST", "/api/projects", map[string]any{"name": "Docs Walk"}).decode(t, &project)
	pid := project.ID.String()
	const path = "context/handbook.md"
	file := "---\ntitle: Handbook\n---\n\n# Handbook\n\nHow we work: carefully.\n"

	w.call("POST", "/api/docs/write", map[string]any{
		"scope": "project", "project_id": pid, "path": path,
		"content": file, "expected_version": 0, "message": "first version",
	})
	q := url.Values{"scope": {"project"}, "project_id": {pid}, "path": {path}}
	read := w.call("GET", "/api/docs/read?"+q.Encode(), nil)
	var doc struct {
		Content        string `json:"content"`
		CurrentVersion int    `json:"current_version"`
		ContentHTML    string `json:"content_html"`
	}
	read.decode(t, &doc)
	if doc.Content != file {
		t.Errorf("the document came back changed:\n got %q\nwant %q", doc.Content, file)
	}
	if !strings.Contains(doc.ContentHTML, "<h1") || strings.Contains(doc.ContentHTML, "title:") {
		t.Errorf("rendered html should show the body without the frontmatter: %s", doc.ContentHTML)
	}

	w.call("POST", "/api/docs/write", map[string]any{
		"scope": "project", "project_id": pid, "path": path,
		"content": file + "\nAnd a second paragraph.\n", "expected_version": doc.CurrentVersion,
		"message": "second version",
	})

	listed := w.call("GET", "/api/docs?"+url.Values{"scope": {"project"}, "project_id": {pid}}.Encode(), nil)
	if !listed.contains(path) {
		t.Errorf("the document is missing from the listing: %s", listed.body)
	}
	found := w.call("GET", "/api/docs/search?"+url.Values{
		"scope": {"project"}, "project_id": {pid}, "q": {"carefully"},
	}.Encode(), nil)
	if !found.contains(path) {
		t.Errorf("full-text search did not find the document: %s", found.body)
	}
	history := w.call("GET", "/api/docs/history?"+q.Encode(), nil)
	if !history.contains("first version") || !history.contains("second version") {
		t.Errorf("history is missing a version: %s", history.body)
	}
	v1 := w.call("GET", "/api/docs/read-version?"+url.Values{
		"scope": {"project"}, "project_id": {pid}, "path": {path}, "version": {"1"},
	}.Encode(), nil)
	if v1.contains("second paragraph") {
		t.Errorf("version 1 should predate the second edit: %s", v1.body)
	}

	w.call("POST", "/api/docs/delete", map[string]any{
		"scope": "project", "project_id": pid, "path": path,
		"expected_version": 2, "message": "no longer needed",
	})
	if still := w.call("GET", "/api/docs?"+url.Values{"scope": {"project"}, "project_id": {pid}}.Encode(), nil); still.contains(path) {
		t.Errorf("a deleted document still shows in the listing: %s", still.body)
	}
}

// What the topbar and the profile page read: search across domains,
// the user directory, the notification bell and its preferences, the
// version banner, and the skill bundle agents install.
func TestHappyPath_AccountAndInstanceSurfaces(t *testing.T) {
	w, _, _ := newWalk(t)
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	w.call("POST", "/api/projects", map[string]any{"name": "Surfaces"}).decode(t, &project)
	w.base = "/api/projects/" + project.ID.String()
	w.call("POST", w.project("/tasks"), map[string]any{"title": "findable by search", "type": "task"})

	me := w.call("GET", "/api/me", nil)
	if !me.contains("walker") {
		t.Errorf("/api/me: %s", me.body)
	}
	w.call("GET", "/api/me/tokens", nil)
	w.call("GET", "/api/users", nil)

	hits := w.call("GET", "/api/search?"+url.Values{
		"project_id": {project.ID.String()}, "q": {"findable"},
	}.Encode(), nil)
	if !hits.contains("findable by search") {
		t.Errorf("search did not find the task: %s", hits.body)
	}

	w.call("GET", "/api/notifications", nil)
	w.call("GET", "/api/notifications/unread_count", nil)
	w.call("POST", "/api/notifications/read_all", nil)
	prefs := w.call("GET", "/api/me/notification_preferences", nil)
	if !prefs.contains(notifications.KindTaskAssigned) {
		t.Errorf("preferences do not list the known kinds: %s", prefs.body)
	}
	w.call("PATCH", "/api/me/notification_preferences", map[string]any{
		notifications.KindTaskAssigned: false,
	})
	after := w.call("GET", "/api/me/notification_preferences", nil)
	if !after.contains(fmt.Sprintf("%q:false", notifications.KindTaskAssigned)) {
		t.Errorf("the preference toggle did not stick: %s", after.body)
	}

	w.call("GET", "/api/version/status", nil)
	w.call("POST", "/api/markdown/render", map[string]any{"content": "**hi**"})

	// Unauthenticated instance surfaces the browser and agents hit.
	for _, path := range []string{"/healthz", "/version", "/skill", "/skill/domains/tasks.md"} {
		resp, err := http.Get(w.ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d: %s", path, resp.StatusCode, body)
		}
		if len(body) == 0 {
			t.Errorf("GET %s returned an empty body", path)
		}
	}
	resp, err := http.Get(w.ts.URL + "/static/styles.css")
	if err != nil {
		t.Fatalf("static asset: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the embedded stylesheet is not served: %d", resp.StatusCode)
	}
	spa, err := http.Get(w.ts.URL + "/projects/whatever/board/kanban")
	if err != nil {
		t.Fatalf("spa route: %v", err)
	}
	_ = spa.Body.Close()
	if spa.StatusCode != http.StatusOK {
		t.Errorf("an app route should serve the SPA shell, got %d", spa.StatusCode)
	}
}
