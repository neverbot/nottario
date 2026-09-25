package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// /me lists the tokens a person has issued across every project. It
// is the only place they can audit their own agents, so it must show
// all of them, none of anybody else's, and never the secret itself —
// the secret is shown once, at creation, and stored hashed.
func TestApiMe_TokensListing(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")

	me, _, err := identity.UpsertFromGithub(ctx, pool, 14001, "tokens-me", "Me", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	other, _, err := identity.UpsertFromGithub(ctx, pool, 14002, "tokens-other", "Other", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub other: %v", err)
	}
	alpha, _ := identity.CreateProject(ctx, pool, "Alpha", "", "", "", me.ID)
	beta, _ := identity.CreateProject(ctx, pool, "Beta", "", "", "", me.ID)
	gamma, _ := identity.CreateProject(ctx, pool, "Gamma", "", "", "", other.ID)

	secretA, _, err := identity.IssueToken(ctx, pool, me.ID, alpha.ID, "alpha agent", nil)
	if err != nil {
		t.Fatalf("IssueToken alpha: %v", err)
	}
	if _, _, err := identity.IssueToken(ctx, pool, me.ID, beta.ID, "beta agent", nil); err != nil {
		t.Fatalf("IssueToken beta: %v", err)
	}
	if _, _, err := identity.IssueToken(ctx, pool, other.ID, gamma.ID, "not mine", nil); err != nil {
		t.Fatalf("IssueToken gamma: %v", err)
	}

	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)

	sess, _ := identity.NewSession(ctx, pool, me.ID, "test", "127.0.0.1")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}

	req, _ := http.NewRequest("GET", ts.URL+"/api/me/tokens", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body struct {
		Tokens []map[string]any `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tokens) != 2 {
		t.Fatalf("got %d tokens, want the caller's two: %v", len(body.Tokens), body.Tokens)
	}
	names := map[string]bool{}
	for _, tk := range body.Tokens {
		names[tk["name"].(string)] = true
	}
	if !names["alpha agent"] || !names["beta agent"] {
		t.Errorf("listing misses a project: %v", names)
	}
	if names["not mine"] {
		t.Error("the listing includes another user's token")
	}

	raw, _ := json.Marshal(body.Tokens)
	if strings.Contains(string(raw), secretA) {
		t.Error("the token secret is exposed by the listing")
	}
	// A 12-character prefix is shown on purpose, so a person can tell
	// their agents apart. Anything longer would be the secret itself.
	for _, tk := range body.Tokens {
		prefix, _ := tk["prefix"].(string)
		if prefix == "" || !strings.HasPrefix(prefix, identity.TokenPrefix) {
			t.Errorf("token row has no usable prefix: %v", tk)
		}
		if len(prefix) > 12 {
			t.Errorf("prefix %q is long enough to be the secret", prefix)
		}
		if strings.Contains(secretA, prefix) && len(prefix) >= len(secretA) {
			t.Error("the whole secret is being shown as a prefix")
		}
	}

	anon, err := http.Get(ts.URL + "/api/me/tokens")
	if err != nil {
		t.Fatal(err)
	}
	defer anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous listing: %d, want 401", anon.StatusCode)
	}
}

// The markdown endpoint renders what people type in the UI, so it
// must render real markdown and must not hand back active content.
func TestApiMarkdown_RenderThroughTheRouter(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14003, "md-router", "MD", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)
	sess, _ := identity.NewSession(ctx, pool, u.ID, "test", "127.0.0.1")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}

	post := func(cookie *http.Cookie, body []byte) *rawResp {
		t.Helper()
		req, _ := http.NewRequest("POST", ts.URL+"/api/markdown/render", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return &rawResp{StatusCode: resp.StatusCode, Body: raw}
	}

	r := post(cookie, mustJSON(map[string]any{
		"content": "# Title\n\nSome **bold** text.\n\n<script>alert(1)</script>\n",
	}))
	if r.StatusCode != http.StatusOK {
		t.Fatalf("render: %d %s", r.StatusCode, r.Body)
	}
	var rendered struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(r.Body, &rendered); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
	html := rendered.HTML
	if !strings.Contains(html, "<h1") || !strings.Contains(html, "<strong>") {
		t.Errorf("markdown was not rendered: %s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("script tag survived sanitisation: %s", html)
	}

	if anon := post(nil, mustJSON(map[string]any{"content": "hi"})); anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous render: %d, want 401", anon.StatusCode)
	}
}
