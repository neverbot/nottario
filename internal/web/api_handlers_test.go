// Covers the remaining web HTTP surfaces in one file: projects
// CRUD, users list, tokens edge cases, skill bundle endpoints, and
// the docs handlers that aren't already exercised by the versioning
// suite. Each surface is one Test function.
package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// handlersFixture: an instance admin + a regular outsider with their
// own tokens, plus a project owned by admin.
type handlersFixture struct {
	ts           *httptest.Server
	authAdmin    string
	authOutsider string
	adminID      string
	outsiderID   string
	projectID    string
}

func setupHandlersFixture(t *testing.T) *handlersFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	admin, _, err := identity.UpsertFromGithub(ctx, pool, 13501, "admin-h", "Admin", "")
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	p, _ := identity.CreateProject(ctx, pool, "Handlers", "", "", "", admin.ID)
	adminToken, _, _ := identity.IssueToken(ctx, pool, admin.ID, p.ID, "admin", nil)
	outsider, _, _ := identity.UpsertFromGithub(ctx, pool, 13502, "outsider-h", "Out", "")
	outProj, _ := identity.CreateProject(ctx, pool, "Handlers-Out", "", "", "", outsider.ID)
	outsiderToken, _, _ := identity.IssueToken(ctx, pool, outsider.ID, outProj.ID, "out", nil)

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &handlersFixture{
		ts:           ts,
		authAdmin:    "Bearer " + adminToken,
		authOutsider: "Bearer " + outsiderToken,
		adminID:      admin.ID.String(),
		outsiderID:   outsider.ID.String(),
		projectID:    p.ID.String(),
	}
}

// ---- /api/projects ----

func TestApiProjects_ListAndGet(t *testing.T) {
	f := setupHandlersFixture(t)

	r := doRaw(t, "GET", f.ts.URL+"/api/projects", "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("list no-auth: %d", r.StatusCode)
	}

	var list struct {
		Projects []map[string]any `json:"projects"`
	}
	doJSON(t, "GET", f.ts.URL+"/api/projects", f.authAdmin, nil, &list)
	if len(list.Projects) == 0 {
		t.Error("admin should see at least the seeded project")
	}

	// Get by id.
	var p map[string]any
	doJSON(t, "GET", f.ts.URL+"/api/projects/"+f.projectID, f.authAdmin, nil, &p)
	if p["id"] != f.projectID {
		t.Errorf("get id mismatch: %+v", p)
	}

	// Probing a random project id with a project-scoped token hits
	// the scope guard before the not-found branch ever runs — the
	// admin's token is bound to f.projectID, not the random uuid.
	r = doRaw(t, "GET", f.ts.URL+"/api/projects/"+uuid.New().String(), f.authAdmin, nil)
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("get random project (scope mismatch): got %d, want 403", r.StatusCode)
	}

	// 401 unauth on get.
	r = doRaw(t, "GET", f.ts.URL+"/api/projects/"+f.projectID, "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("get no-auth: %d", r.StatusCode)
	}
}

func TestApiProjects_CreateAdminOnly(t *testing.T) {
	f := setupHandlersFixture(t)

	// 401 no auth.
	r := doRaw(t, "POST", f.ts.URL+"/api/projects", "",
		[]byte(`{"name":"x"}`))
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("no auth: %d", r.StatusCode)
	}

	// 403 non-admin.
	r = doRaw(t, "POST", f.ts.URL+"/api/projects", f.authOutsider,
		[]byte(`{"name":"x"}`))
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin: %d", r.StatusCode)
	}

	// 400 empty name.
	r = doRaw(t, "POST", f.ts.URL+"/api/projects", f.authAdmin,
		[]byte(`{"name":""}`))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name: %d", r.StatusCode)
	}

	// 400 invalid JSON.
	r = doRaw(t, "POST", f.ts.URL+"/api/projects", f.authAdmin, []byte("nope"))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad json: %d", r.StatusCode)
	}

	// 201 happy path.
	var p map[string]any
	body, _ := json.Marshal(map[string]any{"name": "FromTest", "description": "y"})
	r = doRaw(t, "POST", f.ts.URL+"/api/projects", f.authAdmin, body)
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", r.StatusCode, r.Body)
	}
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p["name"] != "FromTest" {
		t.Errorf("name mismatch: %+v", p)
	}
}

func TestApiProjects_UpdateAndDelete(t *testing.T) {
	f := setupHandlersFixture(t)

	// PATCH name + default_view.
	body, _ := json.Marshal(map[string]any{
		"name": "Renamed", "default_view": "board/gantt",
	})
	r := doRaw(t, "PATCH", f.ts.URL+"/api/projects/"+f.projectID, f.authAdmin, body)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", r.StatusCode, r.Body)
	}

	// 403 non-admin update.
	r = doRaw(t, "PATCH", f.ts.URL+"/api/projects/"+f.projectID, f.authOutsider, body)
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin update: %d", r.StatusCode)
	}

	// 400 invalid id.
	r = doRaw(t, "PATCH", f.ts.URL+"/api/projects/not-a-uuid", f.authAdmin, body)
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad id: %d", r.StatusCode)
	}

	// PATCH default_view (own handler).
	r = doRaw(t, "PATCH", f.ts.URL+"/api/projects/"+f.projectID+"/default_view",
		f.authAdmin, []byte(`{"default_view":"board/kanban"}`))
	if r.StatusCode != http.StatusOK {
		t.Errorf("default_view: %d %s", r.StatusCode, r.Body)
	}

	// PATCH mcp page size.
	r = doRaw(t, "PATCH", f.ts.URL+"/api/projects/"+f.projectID+"/mcp",
		f.authAdmin, []byte(`{"mcp_page_size":25}`))
	if r.StatusCode != http.StatusOK {
		t.Errorf("mcp page size: %d %s", r.StatusCode, r.Body)
	}

	// Delete: 403 non-admin first, then 200 admin.
	r = doRaw(t, "DELETE", f.ts.URL+"/api/projects/"+f.projectID, f.authOutsider, nil)
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin delete: %d", r.StatusCode)
	}
	r = doRaw(t, "DELETE", f.ts.URL+"/api/projects/"+f.projectID, f.authAdmin, nil)
	if r.StatusCode >= 300 {
		t.Errorf("delete: %d %s", r.StatusCode, r.Body)
	}
}

// ---- /api/users ----

func TestApiUsers_AuthAndList(t *testing.T) {
	f := setupHandlersFixture(t)
	r := doRaw(t, "GET", f.ts.URL+"/api/users", "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("no auth: %d", r.StatusCode)
	}
	var out struct {
		Users []map[string]any `json:"users"`
	}
	doJSON(t, "GET", f.ts.URL+"/api/users", f.authAdmin, nil, &out)
	if len(out.Users) < 2 {
		t.Errorf("expected ≥2 users (admin + outsider), got %d", len(out.Users))
	}
}

// ---- /api/tokens edge cases ----

func TestApiTokens_RevokeMissing(t *testing.T) {
	f := setupHandlersFixture(t)
	// Token revoke uses session-cookie auth — issue a session for the
	// admin who owns the fixture project so the membership check
	// passes before we get to the "does this token exist?" branch.
	ctx := t.Context()
	pool := testutil.NewPool(t)
	u, _, _ := identity.UpsertFromGithub(ctx, pool, 13503, "rev", "Rev", "")
	p, _ := identity.CreateProject(ctx, pool, "Rev", "", "", "", u.ID)
	key := []byte("test-session-key")
	sess, _ := identity.NewSession(ctx, pool, u.ID, "t", "127.0.0.1")
	cookie := &http.Cookie{
		Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key),
	}
	srv := NewServer(Deps{
		Pool: pool, Resolver: identity.NewResolver(pool, key, false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Revoke a never-existed token. Behaviour: handler returns 404
	// (the token row doesn't exist for this project).
	url := ts.URL + "/api/projects/" + p.ID.String() + "/tokens/" + uuid.New().String()
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound &&
		resp.StatusCode != http.StatusNoContent &&
		resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusBadRequest {
		t.Errorf("revoke missing: got %d, want 404/204/200/400", resp.StatusCode)
	}
	_ = f
}

// ---- /skill, /skill.zip ----

func TestApiSkill_Serve(t *testing.T) {
	f := setupHandlersFixture(t)

	// /skill returns the root skill.md (text/markdown).
	r := doRaw(t, "GET", f.ts.URL+"/skill", "", nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("/skill: %d", r.StatusCode)
	}
	// The bundle ships a markdown header.
	if !strings.Contains(string(r.Body), "Nottario") && !strings.Contains(string(r.Body), "nottario") {
		t.Errorf("/skill body doesn't look like the skill bundle: %.80s", r.Body)
	}

	// Missing skill file → 404.
	r = doRaw(t, "GET", f.ts.URL+"/skill/no-such.md", "", nil)
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("missing skill: %d", r.StatusCode)
	}

	// /skill.zip returns a valid zip.
	resp, err := http.Get(f.ts.URL + "/skill.zip")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("zip status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip parse: %v", err)
	}
	if len(zr.File) == 0 {
		t.Errorf("zip has no entries")
	}
}

// ---- /api/docs handlers (beyond versioning) ----

func TestApiDocs_ListSearchHistoryDelete(t *testing.T) {
	// Seed a project doc directly via the docs package so we don't
	// have to reproduce the write-protocol of the API here.
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, _ := identity.UpsertFromGithub(ctx, pool, 13504, "dh", "DH", "")
	key := []byte("test-session-key")
	p, _ := identity.CreateProject(ctx, pool, "DH", "", "", "", u.ID)
	zero := 0
	path := "projects/" + p.ID.String() + "/notes/alpha.md"
	doc, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD: "# alpha\n\nfirst", Message: "init",
		ExpectedVersion: &zero,
	}, docs.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	// Second version.
	v1 := 1
	_, _ = docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD: "# alpha\n\nsecond", Message: "update",
		ExpectedVersion: &v1,
	}, docs.Authorship{UserID: &u.ID})

	// New session + server bound to THIS pool so we hit the seeded doc.
	sess, _ := identity.NewSession(ctx, pool, u.ID, "t", "127.0.0.1")
	cookie := &http.Cookie{
		Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key),
	}
	srv := NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	withCookie := func(method, urlStr string, body []byte) *rawResp {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, _ := http.NewRequest(method, urlStr, reqBody)
		req.AddCookie(cookie)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, urlStr, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return &rawResp{StatusCode: resp.StatusCode, Body: b}
	}

	// List.
	q := url.Values{"scope": {"project"}, "project_id": {p.ID.String()}}
	r := withCookie("GET", ts.URL+"/api/docs?"+q.Encode(), nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("list: %d %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(string(r.Body), "alpha.md") {
		t.Errorf("list missing alpha.md: %s", r.Body)
	}

	// Read.
	q = url.Values{"scope": {"project"}, "project_id": {p.ID.String()}, "path": {path}}
	r = withCookie("GET", ts.URL+"/api/docs/read?"+q.Encode(), nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("read: %d %s", r.StatusCode, r.Body)
	}

	// Search.
	q = url.Values{
		"scope": {"project"}, "project_id": {p.ID.String()}, "q": {"second"},
	}
	r = withCookie("GET", ts.URL+"/api/docs/search?"+q.Encode(), nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("search: %d %s", r.StatusCode, r.Body)
	}

	// History.
	q = url.Values{"scope": {"project"}, "project_id": {p.ID.String()}, "path": {path}}
	r = withCookie("GET", ts.URL+"/api/docs/history?"+q.Encode(), nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("history: %d %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(string(r.Body), "init") {
		t.Errorf("history missing init message: %s", r.Body)
	}

	// Read v1 specifically.
	q = url.Values{
		"scope": {"project"}, "project_id": {p.ID.String()}, "path": {path}, "version": {"1"},
	}
	r = withCookie("GET", ts.URL+"/api/docs/read-version?"+q.Encode(), nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("read-version: %d %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(string(r.Body), "first") {
		t.Errorf("v1 body should contain 'first': %s", r.Body)
	}

	// Read missing path.
	q = url.Values{
		"scope": {"project"}, "project_id": {p.ID.String()}, "path": {"missing.md"},
	}
	r = withCookie("GET", ts.URL+"/api/docs/read?"+q.Encode(), nil)
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("missing read: %d", r.StatusCode)
	}

	// Delete.
	body, _ := json.Marshal(map[string]any{
		"scope": "project", "project_id": p.ID.String(), "path": path,
	})
	r = withCookie("POST", ts.URL+"/api/docs/delete", body)
	if r.StatusCode >= 300 {
		t.Errorf("delete: %d %s", r.StatusCode, r.Body)
	}
	// After delete, the doc is gone.
	q = url.Values{"scope": {"project"}, "project_id": {p.ID.String()}, "path": {path}}
	r = withCookie("GET", ts.URL+"/api/docs/read?"+q.Encode(), nil)
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("read after delete: %d", r.StatusCode)
	}
	_ = doc
}
