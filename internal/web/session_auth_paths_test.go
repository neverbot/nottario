package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// sessionFixture gives a human (cookie, no token) and a second human
// who is not a member of the project. Almost every other test in this
// package authenticates with a Bearer token, so the cookie branch of
// each handler — the one every person actually uses — is the one that
// goes unexercised.
type sessionFixture struct {
	ts        *httptest.Server
	pool      *pgxpool.Pool
	admin     *http.Cookie
	outsider  *http.Cookie
	adminID   uuid.UUID
	key       []byte
	projectID string
}

func setupSessions(t *testing.T) *sessionFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")

	admin, _, err := identity.UpsertFromGithub(ctx, pool, 13901, "session-admin", "Admin", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub admin: %v", err)
	}
	outsider, _, err := identity.UpsertFromGithub(ctx, pool, 13902, "session-outsider", "Outsider", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub outsider: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "Session Project", "", "", "", admin.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	cookieFor := func(u uuid.UUID) *http.Cookie {
		t.Helper()
		sess, err := identity.NewSession(ctx, pool, u, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		return &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)
	return &sessionFixture{
		ts: ts, pool: pool, key: key, adminID: admin.ID,
		admin: cookieFor(admin.ID), outsider: cookieFor(outsider.ID),
		projectID: p.ID.String(),
	}
}

func (f *sessionFixture) do(t *testing.T, method, path string, cookie *http.Cookie, body []byte) *rawResp {
	t.Helper()
	var r *http.Request
	if body != nil {
		r, _ = http.NewRequest(method, f.ts.URL+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r, _ = http.NewRequest(method, f.ts.URL+path, nil)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return &rawResp{StatusCode: resp.StatusCode, Body: raw}
}

// The settings surfaces a person uses: project taxonomy edits, all
// through a session cookie, and all closed to someone outside the
// project.
func TestSessionAuth_ProjectAdminSurfaces(t *testing.T) {
	f := setupSessions(t)
	base := "/api/projects/" + f.projectID

	cases := []struct {
		name, method, path string
		body               []byte
	}{
		{"read the project", "GET", base, nil},
		{"list roles", "GET", base + "/roles", nil},
		{"list priorities", "GET", base + "/priorities", nil},
		{"list cycles", "GET", base + "/cycles", nil},
		{"list members", "GET", base + "/members", nil},
		{"list tasks", "GET", base + "/tasks", nil},
		{"rename the project", "PATCH", base, mustJSON(map[string]any{"name": "Renamed By A Human"})},
		{"add a role", "POST", base + "/roles", mustJSON(map[string]any{"key": "ops", "label": "Ops", "color": "#123456"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := f.do(t, tc.method, tc.path, f.admin, tc.body)
			if r.StatusCode < 200 || r.StatusCode >= 300 {
				t.Errorf("as the owner: %d %s", r.StatusCode, r.Body)
			}
			out := f.do(t, tc.method, tc.path, f.outsider, tc.body)
			if out.StatusCode >= 200 && out.StatusCode < 300 {
				t.Errorf("someone outside the project got through: %d %s", out.StatusCode, out.Body)
			}
			anon := f.do(t, tc.method, tc.path, nil, tc.body)
			if anon.StatusCode != http.StatusUnauthorized {
				t.Errorf("anonymous: %d, want 401", anon.StatusCode)
			}
		})
	}

	// The rename actually happened, rather than answering 2xx and
	// doing nothing.
	r := f.do(t, "GET", base, f.admin, nil)
	var project map[string]any
	if err := json.Unmarshal(r.Body, &project); err != nil {
		t.Fatal(err)
	}
	if project["name"] != "Renamed By A Human" {
		t.Errorf("project name = %v after the PATCH", project["name"])
	}
}

// A session that was deleted server-side must stop working even
// though the browser still holds the cookie.
func TestSessionAuth_DeletedSessionStopsWorking(t *testing.T) {
	f := setupSessions(t)
	if r := f.do(t, "GET", "/api/me", f.admin, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("fresh session: %d", r.StatusCode)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM sessions`); err != nil {
		t.Fatalf("delete sessions: %v", err)
	}
	if r := f.do(t, "GET", "/api/me", f.admin, nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("deleted session still authenticates: %d", r.StatusCode)
	}
}

// A cookie signed with the wrong key is a forgery: knowing a session
// id is not enough without the server's key.
func TestSessionAuth_ForgedCookieRejected(t *testing.T) {
	f := setupSessions(t)
	sess, err := identity.NewSession(t.Context(), f.pool, f.adminID, "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	real := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, f.key)}
	forged := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, []byte("not-the-session-key"))}

	if r := f.do(t, "GET", "/api/me", real, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("the genuine cookie does not work: %d %s", r.StatusCode, r.Body)
	}
	if r := f.do(t, "GET", "/api/me", forged, nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("a cookie signed with the wrong key was accepted: %d %s", r.StatusCode, r.Body)
	}
}
