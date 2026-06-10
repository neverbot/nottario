package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
	"github.com/neverbot/nottario/internal/web"
)

// helper: build a server with the markdown route + a logged-in user
// cookie. Reused across the four cases below.
func newMarkdownServer(t *testing.T) (*httptest.Server, *http.Cookie, *identity.Caller) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := context.Background()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 6001, "md", "MD", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	resolver := identity.NewResolver(pool, key, false)
	sess, err := identity.NewSession(ctx, pool, u.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookie := &http.Cookie{
		Name:  identity.SessionCookieName,
		Value: identity.EncodeCookie(sess.ID, key),
	}
	c, _ := resolver.ResolveSession(&http.Request{Header: http.Header{"Cookie": []string{cookie.String()}}})
	srv := httptest.NewServer(web.RenderMarkdownHandler(web.MarkdownDeps{
		Pool: pool, Resolver: resolver,
	}))
	t.Cleanup(srv.Close)
	return srv, cookie, &c
}

func TestAPIMarkdown_RejectsUnauthenticated(t *testing.T) {
	srv, _, _ := newMarkdownServer(t)
	body := bytes.NewBufferString(`{"content":"hi"}`)
	resp, err := http.Post(srv.URL, "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestAPIMarkdown_RejectsInvalidJSON(t *testing.T) {
	srv, cookie, _ := newMarkdownServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("not-json"))
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAPIMarkdown_RejectsInvalidProjectID(t *testing.T) {
	srv, cookie, _ := newMarkdownServer(t)
	body := bytes.NewBufferString(`{"project_id":"not-a-uuid","content":"hi"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, body)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAPIMarkdown_RendersAuthenticated(t *testing.T) {
	srv, cookie, _ := newMarkdownServer(t)
	body := bytes.NewBufferString(`{"content":"# Hi\n\nA *line*."}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, body)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	var payload struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(payload.HTML, "<h1") || !strings.Contains(payload.HTML, "<em>line</em>") {
		t.Errorf("unexpected html: %s", payload.HTML)
	}
}
