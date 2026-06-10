package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// archFixture spins up the real router with two users: owner is the
// instance admin (first GitHub user) and therefore has access to
// every project; outsider has no membership and exercises 404 paths.
type archFixture struct {
	ts           *httptest.Server
	authOwner    string
	authOutsider string
	projectID    string
}

func setupArch(t *testing.T) *archFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	owner, _, err := identity.UpsertFromGithub(ctx, pool, 13401, "arch-owner", "Owner", "")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "Arch", "", "", "", owner.ID, nil)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	ownerToken, _, _ := identity.IssueToken(ctx, pool, owner.ID, p.ID, "owner-token", nil)
	outsider, _, _ := identity.UpsertFromGithub(ctx, pool, 13402, "arch-outsider", "Outsider", "")
	// Outsider's token lives in their own project — separate scope so
	// it can never authenticate against the owner's project under
	// per-project token semantics.
	outProj, err := identity.CreateProject(ctx, pool, "Arch-Out", "", "", "", outsider.ID, nil)
	if err != nil {
		t.Fatalf("outsider project: %v", err)
	}
	outsiderToken, _, _ := identity.IssueToken(ctx, pool, outsider.ID, outProj.ID, "out-token", nil)
	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &archFixture{
		ts:           ts,
		authOwner:    "Bearer " + ownerToken,
		authOutsider: "Bearer " + outsiderToken,
		projectID:    p.ID.String(),
	}
}

func (f *archFixture) url(path string) string {
	return f.ts.URL + "/api/projects/" + f.projectID + "/arch" + path
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// ---- Auth gates ----

func TestApiArch_Unauthenticated(t *testing.T) {
	f := setupArch(t)
	r := doRaw(t, "GET", f.url("/kinds"), "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("kinds list: got %d, want 401", r.StatusCode)
	}
}

func TestApiArch_OutsiderRejected(t *testing.T) {
	// The outsider's token is scoped to their OWN project, not the
	// owner's. The scope guard on /api/projects/{id}/... rejects with
	// 403 before any membership check fires. Per-project tokens make
	// this the authoritative non-member rejection path.
	f := setupArch(t)
	r := doRaw(t, "GET", f.url("/kinds"), f.authOutsider, nil)
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("kinds: got %d, want 403 for non-member (token scoped to other project)", r.StatusCode)
	}
}

// Sweeps every handler's auth check (401) and access check (404)
// in one place. Touches each handler so its first 8 lines count
// in coverage.
func TestApiArch_AuthSweep(t *testing.T) {
	f := setupArch(t)
	cases := []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/kinds", nil},
		{"POST", "/kinds", mustJSON(map[string]any{"key": "x", "label": "X"})},
		{"DELETE", "/kinds/external", nil},
		{"GET", "/nodes", nil},
		{"POST", "/nodes", mustJSON(map[string]any{"slug": "x", "kind": "service", "name": "X"})},
		{"GET", "/nodes/sys", nil},
		{"DELETE", "/nodes/sys", nil},
		{"POST", "/nodes/sys/move", mustJSON(map[string]any{"parent_slug": ""})},
		{"POST", "/nodes/sys/links", mustJSON(map[string]any{"doc_path": "x"})},
		{"POST", "/nodes/sys/unlinks", mustJSON(map[string]any{"doc_path": "x"})},
		{"GET", "/edges", nil},
		{"POST", "/edges", mustJSON(map[string]any{"from_slug": "a", "to_slug": "b", "kind": "calls"})},
		{"DELETE", "/edges/00000000-0000-0000-0000-000000000000", nil},
	}
	for _, c := range cases {
		// 401: no auth header.
		r := doRaw(t, c.method, f.url(c.path), "", c.body)
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s no-auth: got %d, want 401", c.method, c.path, r.StatusCode)
		}
		// 403: outsider's token is scoped to their own project; the
		// scope guard rejects before any membership check.
		r = doRaw(t, c.method, f.url(c.path), f.authOutsider, c.body)
		if r.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s outsider: got %d, want 403", c.method, c.path, r.StatusCode)
		}
	}

	// Bad project_id (not a uuid) → 400.
	url := f.ts.URL + "/api/projects/not-a-uuid/arch/kinds"
	r := doRaw(t, "GET", url, f.authOwner, nil)
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad project_id: %d", r.StatusCode)
	}
}

// ---- Kinds: list / upsert / delete (in-use guard) ----

func TestApiArch_KindsLifecycle(t *testing.T) {
	f := setupArch(t)

	// List default catalogue (auto-seeded on first touch).
	var ks struct {
		Kinds []map[string]any `json:"kinds"`
	}
	doJSON(t, "GET", f.url("/kinds"), f.authOwner, nil, &ks)
	if len(ks.Kinds) < 5 {
		t.Errorf("expected ≥5 default kinds, got %d", len(ks.Kinds))
	}

	// Upsert custom kind.
	body := mustJSON(map[string]any{"key": "worker", "label": "Worker", "color": "#abcdef"})
	r := doRaw(t, "POST", f.url("/kinds"), f.authOwner, body)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("upsert kind: %d %s", r.StatusCode, r.Body)
	}

	// Delete an unused kind succeeds.
	r = doRaw(t, "DELETE", f.url("/kinds/worker"), f.authOwner, nil)
	if r.StatusCode >= 300 {
		t.Errorf("delete unused kind: %d %s", r.StatusCode, r.Body)
	}

	// Insert a node using the default `service` kind, then deleting
	// that kind must fail with 409 (in-use).
	body = mustJSON(map[string]any{"slug": "svc", "kind": "service", "name": "Svc"})
	r = doRaw(t, "POST", f.url("/nodes"), f.authOwner, body)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("upsert node: %d %s", r.StatusCode, r.Body)
	}
	r = doRaw(t, "DELETE", f.url("/kinds/service"), f.authOwner, nil)
	if r.StatusCode != http.StatusBadRequest && r.StatusCode != http.StatusConflict {
		t.Errorf("delete in-use kind: got %d, want 400 or 409", r.StatusCode)
	}
}

// ---- Nodes: insert / list / get / move / cycles / cascade ----

func TestApiArch_Nodes(t *testing.T) {
	f := setupArch(t)

	// Bad payload → 400.
	r := doRaw(t, "POST", f.url("/nodes"), f.authOwner, []byte("not-json"))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad payload: %d", r.StatusCode)
	}

	// Empty slug → 400 from repo validation.
	r = doRaw(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"kind": "service", "name": "x",
	}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("empty slug: %d", r.StatusCode)
	}

	// Insert root.
	doJSON(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "sys", "kind": "system", "name": "System",
		"metadata": map[string]any{"env": "prod"},
	}), nil)

	// Insert child.
	doJSON(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "sys.api", "parent_slug": "sys", "kind": "service", "name": "API",
	}), nil)

	// Missing parent → 400.
	r = doRaw(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "orphan", "parent_slug": "ghost", "kind": "service", "name": "x",
	}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("missing parent: %d", r.StatusCode)
	}

	// List root_only.
	var list struct {
		Nodes []map[string]any `json:"nodes"`
	}
	doJSON(t, "GET", f.url("/nodes")+"?root_only=true", f.authOwner, nil, &list)
	if len(list.Nodes) != 1 || list.Nodes[0]["slug"] != "sys" {
		t.Errorf("list roots: %+v", list.Nodes)
	}

	// List by parent.
	doJSON(t, "GET", f.url("/nodes")+"?parent_slug=sys", f.authOwner, nil, &list)
	if len(list.Nodes) != 1 || list.Nodes[0]["slug"] != "sys.api" {
		t.Errorf("list by parent: %+v", list.Nodes)
	}

	// Get node returns the envelope.
	var get map[string]any
	doJSON(t, "GET", f.url("/nodes/sys.api"), f.authOwner, nil, &get)
	node, _ := get["node"].(map[string]any)
	if node["slug"] != "sys.api" || node["kind"] != "service" {
		t.Errorf("get node: %+v", node)
	}

	// 404 on missing slug.
	r = doRaw(t, "GET", f.url("/nodes/ghost"), f.authOwner, nil)
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("missing node: %d", r.StatusCode)
	}

	// Move sys.api to root.
	doJSON(t, "POST", f.url("/nodes/sys.api/move"), f.authOwner,
		mustJSON(map[string]any{"parent_slug": ""}), nil)

	// Move creates cycle: move sys under sys.api (now a root sibling).
	doJSON(t, "POST", f.url("/nodes/sys.api/move"), f.authOwner,
		mustJSON(map[string]any{"parent_slug": "sys"}), nil)
	r = doRaw(t, "POST", f.url("/nodes/sys/move"), f.authOwner,
		mustJSON(map[string]any{"parent_slug": "sys.api"}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("cycle move: %d %s", r.StatusCode, r.Body)
	}

	// Remove without cascade on a parent → 409 conflict.
	r = doRaw(t, "DELETE", f.url("/nodes/sys"), f.authOwner, nil)
	if r.StatusCode != http.StatusConflict && r.StatusCode != http.StatusBadRequest {
		t.Errorf("remove parent w/o cascade: %d %s", r.StatusCode, r.Body)
	}

	// Remove with cascade succeeds.
	r = doRaw(t, "DELETE", f.url("/nodes/sys?cascade=true"), f.authOwner, nil)
	if r.StatusCode >= 300 {
		t.Errorf("cascade remove: %d %s", r.StatusCode, r.Body)
	}
}

// ---- Edges: insert / list / remove ----

func TestApiArch_Edges(t *testing.T) {
	f := setupArch(t)

	mk := func(slug string) {
		t.Helper()
		doJSON(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
			"slug": slug, "kind": "service", "name": slug,
		}), nil)
	}
	mk("a")
	mk("b")
	mk("c")

	// Bad payload → 400.
	r := doRaw(t, "POST", f.url("/edges"), f.authOwner, []byte("nope"))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad edge payload: %d", r.StatusCode)
	}

	// Self-loop → 400.
	r = doRaw(t, "POST", f.url("/edges"), f.authOwner, mustJSON(map[string]any{
		"from_slug": "a", "to_slug": "a", "kind": "calls",
	}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("self loop: %d %s", r.StatusCode, r.Body)
	}

	// Insert two edges.
	var ab map[string]any
	doJSON(t, "POST", f.url("/edges"), f.authOwner, mustJSON(map[string]any{
		"from_slug": "a", "to_slug": "b", "kind": "calls",
	}), &ab)
	abID, _ := ab["id"].(string)
	doJSON(t, "POST", f.url("/edges"), f.authOwner, mustJSON(map[string]any{
		"from_slug": "b", "to_slug": "c", "kind": "uses",
	}), nil)

	// List all → 2.
	var list struct {
		Edges []map[string]any `json:"edges"`
	}
	doJSON(t, "GET", f.url("/edges"), f.authOwner, nil, &list)
	if len(list.Edges) != 2 {
		t.Errorf("list edges: %+v", list.Edges)
	}

	// Filter by node + direction.
	doJSON(t, "GET", f.url("/edges")+"?node_slug=b&direction=in", f.authOwner, nil, &list)
	if len(list.Edges) != 1 || list.Edges[0]["from_slug"] != "a" {
		t.Errorf("filter in b: %+v", list.Edges)
	}

	// Filter with unknown node → 400.
	r = doRaw(t, "GET", f.url("/edges")+"?node_slug=ghost", f.authOwner, nil)
	if r.StatusCode != http.StatusBadRequest && r.StatusCode != http.StatusInternalServerError {
		t.Errorf("unknown node filter: %d", r.StatusCode)
	}

	// Remove edge.
	r = doRaw(t, "DELETE", f.url("/edges/"+abID), f.authOwner, nil)
	if r.StatusCode >= 300 {
		t.Errorf("remove edge: %d %s", r.StatusCode, r.Body)
	}
	// Remove again → 404.
	r = doRaw(t, "DELETE", f.url("/edges/"+abID), f.authOwner, nil)
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("remove edge again: %d %s", r.StatusCode, r.Body)
	}

	// Remove with malformed uuid → 400.
	r = doRaw(t, "DELETE", f.url("/edges/not-a-uuid"), f.authOwner, nil)
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed edge id: %d", r.StatusCode)
	}
}

// ---- Links: link / unlink doc and task ----

func TestApiArch_Links(t *testing.T) {
	f := setupArch(t)

	doJSON(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "svc", "kind": "service", "name": "Svc",
	}), nil)

	// Link a doc.
	r := doRaw(t, "POST", f.url("/nodes/svc/links"), f.authOwner,
		mustJSON(map[string]any{"doc_path": "x/y.md"}))
	if r.StatusCode >= 300 {
		t.Errorf("link doc: %d %s", r.StatusCode, r.Body)
	}

	// Link a task.
	taskID := uuid.New()
	r = doRaw(t, "POST", f.url("/nodes/svc/links"), f.authOwner,
		mustJSON(map[string]any{"task_id": taskID.String()}))
	if r.StatusCode >= 300 {
		t.Errorf("link task: %d %s", r.StatusCode, r.Body)
	}

	// Link with neither → 400.
	r = doRaw(t, "POST", f.url("/nodes/svc/links"), f.authOwner,
		mustJSON(map[string]any{}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("link empty: %d", r.StatusCode)
	}

	// get_node now shows the links.
	var get map[string]any
	doJSON(t, "GET", f.url("/nodes/svc"), f.authOwner, nil, &get)
	links, _ := get["links"].([]any)
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Unlink doc.
	r = doRaw(t, "POST", f.url("/nodes/svc/unlinks"), f.authOwner,
		mustJSON(map[string]any{"doc_path": "x/y.md"}))
	if r.StatusCode >= 300 {
		t.Errorf("unlink doc: %d %s", r.StatusCode, r.Body)
	}

	// Unlink task.
	r = doRaw(t, "POST", f.url("/nodes/svc/unlinks"), f.authOwner,
		mustJSON(map[string]any{"task_id": taskID.String()}))
	if r.StatusCode >= 300 {
		t.Errorf("unlink task: %d %s", r.StatusCode, r.Body)
	}

	// Unlink with neither → 400.
	r = doRaw(t, "POST", f.url("/nodes/svc/unlinks"), f.authOwner,
		mustJSON(map[string]any{}))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("unlink empty: %d", r.StatusCode)
	}
}

// ---- JSON shape regression ----

func TestApiArch_NodeJSONShape(t *testing.T) {
	f := setupArch(t)
	// Insert a root with no parent and no repo/path — the frontend
	// relies on these fields surfacing as nullable JSON.
	doJSON(t, "POST", f.url("/nodes"), f.authOwner, mustJSON(map[string]any{
		"slug": "root", "kind": "system", "name": "R",
	}), nil)
	r := doRaw(t, "GET", f.url("/nodes/root"), f.authOwner, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("get root: %d %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(string(r.Body), `"parent_id":null`) {
		t.Errorf("expected parent_id:null in body: %s", r.Body)
	}
	if !strings.Contains(string(r.Body), `"linked_repo":null`) {
		t.Errorf("expected linked_repo:null in body: %s", r.Body)
	}
	// metadata defaults to an empty object, not omitted.
	if !strings.Contains(string(r.Body), `"metadata":{}`) {
		t.Errorf("expected empty metadata object: %s", r.Body)
	}
}

// Make sure mustJSON and json.Marshal stay in agreement for nested
// payloads (defensive — this catches accidental field renames).
func TestApiArch_MustJSONRoundTrip(t *testing.T) {
	want := map[string]any{"a": 1.0, "b": []any{"x"}}
	var got map[string]any
	_ = json.NewDecoder(bytes.NewReader(mustJSON(want))).Decode(&got)
	if got["a"] != 1.0 {
		t.Errorf("round-trip: %+v", got)
	}
}
