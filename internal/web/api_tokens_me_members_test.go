package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// Covers three small REST surfaces in one suite to share the seed
// scaffolding: /api/me (whoami), /api/tokens (issue/list/revoke),
// and /api/projects/{id}/members (list/add/remove + role gates).
// Reuses doRaw/doJSON from api_tasks_integration_test.go.

// webFixture exposes both session-cookie and Bearer-token auth. The
// token endpoints (/api/tokens, /api/me) reject Bearer auth on
// purpose (session-only), so cookie variants are needed for those.
type webFixture struct {
	ts             *httptest.Server
	cookieOwner    *http.Cookie
	cookieOutsider *http.Cookie
	authOwner      string
	authOutsider   string
	ownerID        string
	outsiderID     string
	projectID      string
	roleID         string
}

func setupWebFixture(t *testing.T) *webFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()

	owner, _, err := identity.UpsertFromGithub(ctx, pool, 13201, "owner-tme", "Owner", "")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "TME", "", "", "", owner.ID, nil)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	ownerToken, _, err := identity.IssueToken(ctx, pool, owner.ID, p.ID, "owner-token", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	if len(roles) == 0 {
		t.Fatal("no roles seeded")
	}
	outsider, _, _ := identity.UpsertFromGithub(ctx, pool, 13202, "outsider-tme", "Out", "")
	outProj, _ := identity.CreateProject(ctx, pool, "TME-Out", "", "", "", outsider.ID, nil)
	outsiderToken, _, _ := identity.IssueToken(ctx, pool, outsider.ID, outProj.ID, "out-token", nil)

	key := []byte("test-session-key")
	ownerSess, _ := identity.NewSession(ctx, pool, owner.ID, "test", "127.0.0.1")
	outsiderSess, _ := identity.NewSession(ctx, pool, outsider.ID, "test", "127.0.0.1")

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, key, false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &webFixture{
		ts: ts,
		cookieOwner: &http.Cookie{
			Name: identity.SessionCookieName, Value: identity.EncodeCookie(ownerSess.ID, key),
		},
		cookieOutsider: &http.Cookie{
			Name: identity.SessionCookieName, Value: identity.EncodeCookie(outsiderSess.ID, key),
		},
		authOwner:    "Bearer " + ownerToken,
		authOutsider: "Bearer " + outsiderToken,
		ownerID:      owner.ID.String(),
		outsiderID:   outsider.ID.String(),
		projectID:    p.ID.String(),
		roleID:       roles[0].ID.String(),
	}
}

// doWithCookie wraps doRaw for session-cookie-authenticated requests.
// Tokens + Me endpoints require it.
func doWithCookie(t *testing.T, method, url string, cookie *http.Cookie, body []byte) *rawResp {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, reqBody)
	req.AddCookie(cookie)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return &rawResp{StatusCode: resp.StatusCode, Body: b}
}

// ---- /api/me ----

func TestApiMe_Unauthenticated(t *testing.T) {
	f := setupWebFixture(t)
	r := doRaw(t, "GET", f.ts.URL+"/api/me", "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", r.StatusCode)
	}
}

func TestApiMe_Authenticated(t *testing.T) {
	f := setupWebFixture(t)
	r := doWithCookie(t, "GET", f.ts.URL+"/api/me", f.cookieOwner, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", r.StatusCode, r.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != f.ownerID {
		t.Errorf("id mismatch: %+v", out)
	}
	if out["github_login"] != "owner-tme" {
		t.Errorf("github_login mismatch: %+v", out)
	}
}

// ---- /api/tokens ----

func TestApiTokens_Lifecycle(t *testing.T) {
	f := setupWebFixture(t)
	base := f.ts.URL + "/api/projects/" + f.projectID + "/tokens"

	// Issue a token via REST (session cookie auth).
	body, _ := json.Marshal(map[string]any{"name": "via-rest"})
	r := doWithCookie(t, "POST", base, f.cookieOwner, body)
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		t.Fatalf("issue: %d %s", r.StatusCode, r.Body)
	}
	var issued map[string]any
	if err := json.Unmarshal(r.Body, &issued); err != nil {
		t.Fatalf("decode: %v", err)
	}
	plaintext, _ := issued["plaintext"].(string)
	tok, _ := issued["token"].(map[string]any)
	id, _ := tok["ID"].(string)
	if plaintext == "" || !strings.HasPrefix(plaintext, "ntr_") {
		t.Errorf("plaintext missing or unprefixed: %+v", issued)
	}
	if id == "" {
		t.Errorf("issued id missing: %+v", issued)
	}

	// List tokens.
	r = doWithCookie(t, "GET", base, f.cookieOwner, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", r.StatusCode, r.Body)
	}
	var list struct {
		Tokens []map[string]any `json:"tokens"`
	}
	_ = json.Unmarshal(r.Body, &list)
	found := false
	for _, tk := range list.Tokens {
		if tk["ID"] == id {
			found = true
			if _, hasSecret := tk["plaintext"]; hasSecret {
				t.Errorf("list leaked plaintext: %+v", tk)
			}
		}
	}
	if !found {
		t.Errorf("newly-issued token not in list: %+v", list.Tokens)
	}

	// Revoke.
	r = doWithCookie(t, "DELETE", base+"/"+id, f.cookieOwner, nil)
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		t.Errorf("revoke status %d: %s", r.StatusCode, r.Body)
	}

	// After revoke, the token is still listed (revocation is soft,
	// for audit) but `RevokedAt` is set.
	r = doWithCookie(t, "GET", base, f.cookieOwner, nil)
	_ = json.Unmarshal(r.Body, &list)
	var revoked map[string]any
	for _, tk := range list.Tokens {
		if tk["ID"] == id {
			revoked = tk
			break
		}
	}
	if revoked == nil {
		t.Fatalf("revoked token disappeared from list: %+v", list.Tokens)
	}
	if revoked["RevokedAt"] == nil {
		t.Errorf("expected RevokedAt to be set: %+v", revoked)
	}
}

func TestApiTokens_Unauthenticated(t *testing.T) {
	f := setupWebFixture(t)
	// No cookie at all → 401.
	r := doRaw(t, "GET", f.ts.URL+"/api/projects/"+f.projectID+"/tokens", "", nil)
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", r.StatusCode)
	}
}

// ---- /api/projects/{id}/members ----

func TestApiMembers_ListSeedsCreatorWithDefaultRoles(t *testing.T) {
	f := setupWebFixture(t)
	// CreateProject auto-joins the creator with every default role
	// (b18ba5f). The Members list reflects that: one row per
	// (creator, default-role) tuple, four rows total.
	var out struct {
		Members []map[string]any `json:"members"`
	}
	doJSON(t, "GET", f.ts.URL+"/api/projects/"+f.projectID+"/members", f.authOwner, nil, &out)
	if len(out.Members) != 4 {
		t.Errorf("expected 4 default-role memberships for the creator, got %d", len(out.Members))
	}
}

func TestApiMembers_AddRequiresAdmin(t *testing.T) {
	f := setupWebFixture(t)
	// Outsider (not instance admin, not project member) cannot add
	// members — the handler is admin-only.
	body, _ := json.Marshal(map[string]any{
		"user_id": f.outsiderID, "role_id": f.roleID,
	})
	r := doRaw(t, "POST", f.ts.URL+"/api/projects/"+f.projectID+"/members", f.authOutsider, body)
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", r.StatusCode)
	}
}

func TestApiMembers_AddAndRemove(t *testing.T) {
	f := setupWebFixture(t)

	// Owner (instance admin — first GitHub user) adds the outsider
	// with one role. The handler is admin-only.
	addBody, _ := json.Marshal(map[string]any{
		"user_id": f.outsiderID,
		"role_id": f.roleID,
	})
	r := doRaw(t, "POST", f.ts.URL+"/api/projects/"+f.projectID+"/members", f.authOwner, addBody)
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		t.Fatalf("add: %d %s", r.StatusCode, r.Body)
	}

	// Outsider is now a member, can list.
	var listed struct {
		Members []map[string]any `json:"members"`
	}
	doJSON(t, "GET", f.ts.URL+"/api/projects/"+f.projectID+"/members", f.authOutsider, nil, &listed)
	if len(listed.Members) == 0 {
		t.Error("outsider should see members after being added")
	}

	// Remove the role from the outsider.
	r = doRaw(t, "DELETE",
		f.ts.URL+"/api/projects/"+f.projectID+"/members/"+f.outsiderID+"/"+f.roleID,
		f.authOwner, nil)
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		t.Errorf("remove: %d %s", r.StatusCode, r.Body)
	}
}
