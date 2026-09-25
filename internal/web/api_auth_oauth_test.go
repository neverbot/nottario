package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// githubDouble stands in for github.com and api.github.com: it hands
// out an access token and describes one user. membershipState drives
// the org gate ("" = the endpoint answers 404, i.e. not a member).
type githubDouble struct {
	*httptest.Server
	membershipState string
	tokenCalls      int
}

func newGithubDouble(t *testing.T, login string, userID int64) *githubDouble {
	t.Helper()
	d := &githubDouble{}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		d.tokenCalls++
		if err := r.ParseForm(); err != nil || r.Form.Get("code") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") == "expired-code" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gho_test","token_type":"bearer"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": userID, "login": login, "name": "Test " + login, "avatar_url": "https://example.test/a.png",
		})
	})
	mux.HandleFunc("/user/memberships/orgs/", func(w http.ResponseWriter, r *http.Request) {
		if d.membershipState == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"` + d.membershipState + `"}`))
	})
	d.Server = httptest.NewServer(mux)
	t.Cleanup(d.Close)
	return d
}

func authFixture(t *testing.T, org string) (*pgxpool.Pool, *httptest.Server, *githubDouble) {
	t.Helper()
	pool := testutil.NewPool(t)
	gh := newGithubDouble(t, "octocat", 4242)
	key := []byte("test-session-key")
	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, key, false),
		OAuthConfig: identity.OAuthConfig{
			ClientID: "cid", ClientSecret: "secret",
			SessionKey: key, RequiredOrg: org,
			AuthURL:  gh.URL + "/login/oauth/authorize",
			TokenURL: gh.URL + "/login/oauth/access_token",
			APIBase:  gh.URL,
		},
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return pool, ts, gh
}

// noRedirect keeps the 302s visible instead of following them.
func noRedirect(ts *httptest.Server) *http.Client {
	c := ts.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func cookieNamed(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// The whole sign-in dance, end to end: /auth/github/start hands out a
// state cookie and sends the browser to GitHub; the callback exchanges
// the code, creates the user, and opens a session the rest of the API
// accepts; logout destroys that session for good.
func TestAuth_GithubSignInFlow(t *testing.T) {
	pool, ts, gh := authFixture(t, "")
	client := noRedirect(ts)

	start, err := client.Get(ts.URL + "/auth/github/start")
	if err != nil {
		t.Fatal(err)
	}
	defer start.Body.Close()
	if start.StatusCode != http.StatusFound {
		t.Fatalf("start: %d, want 302", start.StatusCode)
	}
	loc, err := url.Parse(start.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(start.Header.Get("Location"), gh.URL) {
		t.Errorf("start redirected to %q, want the configured authorize URL", loc)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatal("no state in the authorize URL")
	}
	stateCookie := cookieNamed(start, "nottario_oauth_state")
	if stateCookie == nil {
		t.Fatal("no state cookie")
	}
	if !stateCookie.HttpOnly {
		t.Error("the state cookie must be HttpOnly")
	}
	if !strings.HasPrefix(stateCookie.Value, state+".") {
		t.Error("the state cookie does not carry the state sent to GitHub")
	}

	callback := func(query string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+"/auth/github/callback?"+query, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	done := callback("state="+state+"&code=good-code", stateCookie)
	defer done.Body.Close()
	if done.StatusCode != http.StatusFound || done.Header.Get("Location") != "/" {
		t.Fatalf("callback: %d → %q, want 302 → /", done.StatusCode, done.Header.Get("Location"))
	}
	session := cookieNamed(done, identity.SessionCookieName)
	if session == nil || session.Value == "" {
		t.Fatal("callback did not set a session cookie")
	}
	if gh.tokenCalls != 1 {
		t.Errorf("token endpoint called %d times, want 1", gh.tokenCalls)
	}

	// The user really exists, and the session really authenticates.
	me := func(c *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
		if c != nil {
			req.AddCookie(c)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	r := me(session)
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("/api/me with the new session: %d, want 200", r.StatusCode)
	}
	var who map[string]any
	if err := json.NewDecoder(r.Body).Decode(&who); err != nil {
		t.Fatal(err)
	}
	if who["github_login"] != "octocat" {
		t.Errorf("/api/me returned %v, want the GitHub login", who["github_login"])
	}
	// First user of an empty instance is the admin.
	if who["is_admin"] != true {
		t.Errorf("first user should be admin, got %v", who["is_admin"])
	}
	var users int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE github_login = 'octocat'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Errorf("users rows = %d, want 1", users)
	}

	// Logout destroys the session server-side, not just the cookie.
	req, _ := http.NewRequest("POST", ts.URL+"/auth/logout", nil)
	req.AddCookie(session)
	out, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	if out.StatusCode != http.StatusFound {
		t.Errorf("logout: %d, want 302", out.StatusCode)
	}
	if cleared := cookieNamed(out, identity.SessionCookieName); cleared == nil || cleared.MaxAge >= 0 && cleared.Value != "" {
		t.Errorf("logout did not clear the session cookie: %+v", cleared)
	}
	after := me(session)
	defer after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("the old session still works after logout: %d, want 401", after.StatusCode)
	}
}

// The state cookie is the CSRF defence of the whole login. Every way
// of getting it wrong must end without a session.
func TestAuth_CallbackRejectsBadState(t *testing.T) {
	_, ts, _ := authFixture(t, "")
	client := noRedirect(ts)

	start, err := client.Get(ts.URL + "/auth/github/start")
	if err != nil {
		t.Fatal(err)
	}
	defer start.Body.Close()
	loc, _ := url.Parse(start.Header.Get("Location"))
	state := loc.Query().Get("state")
	good := cookieNamed(start, "nottario_oauth_state")

	forged := &http.Cookie{Name: "nottario_oauth_state", Value: state + ".00", Path: "/"}
	other := &http.Cookie{Name: "nottario_oauth_state", Value: "someoneelse." + strings.TrimPrefix(good.Value, state+"."), Path: "/"}

	cases := []struct {
		name   string
		query  string
		cookie *http.Cookie
	}{
		{"no state cookie at all", "state=" + state + "&code=good-code", nil},
		{"state does not match the cookie", "state=other-state&code=good-code", good},
		{"cookie signature forged", "state=" + state + "&code=good-code", forged},
		{"cookie carries a different state", "state=someoneelse&code=good-code", other},
		{"no code", "state=" + state, good},
		{"no state", "code=good-code", good},
		{"code rejected by github", "state=" + state + "&code=expired-code", good},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.URL+"/auth/github/callback?"+tc.query, nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status %d, want 400", resp.StatusCode)
			}
			if c := cookieNamed(resp, identity.SessionCookieName); c != nil && c.Value != "" {
				t.Error("a rejected callback handed out a session cookie")
			}
		})
	}
}

// With GITHUB_OAUTH_ORG set, only active members get in. A pending
// invitation is not membership.
func TestAuth_OrgGate(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		wantSession bool
	}{
		{"active member signs in", "active", true},
		{"pending invitation is refused", "pending", false},
		{"not a member is refused", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ts, gh := authFixture(t, "acme")
			gh.membershipState = tc.state
			client := noRedirect(ts)

			start, err := client.Get(ts.URL + "/auth/github/start")
			if err != nil {
				t.Fatal(err)
			}
			defer start.Body.Close()
			loc, _ := url.Parse(start.Header.Get("Location"))
			if !strings.Contains(loc.Query().Get("scope"), "read:org") {
				t.Errorf("org gate on, but scope is %q: read:org is needed to check membership", loc.Query().Get("scope"))
			}
			state := loc.Query().Get("state")

			req, _ := http.NewRequest("GET", ts.URL+"/auth/github/callback?state="+state+"&code=good-code", nil)
			req.AddCookie(cookieNamed(start, "nottario_oauth_state"))
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			session := cookieNamed(resp, identity.SessionCookieName)
			got := session != nil && session.Value != ""
			if got != tc.wantSession {
				t.Errorf("session issued = %v, want %v (status %d, location %q)",
					got, tc.wantSession, resp.StatusCode, resp.Header.Get("Location"))
			}
			if !tc.wantSession && !strings.HasPrefix(resp.Header.Get("Location"), "/login?") {
				t.Errorf("refused login should land on /login with a reason, got %q", resp.Header.Get("Location"))
			}
		})
	}
}
