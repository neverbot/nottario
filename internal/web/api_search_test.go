package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
	"github.com/neverbot/nottario/internal/web"
)

// newSearchServer wires the /api/search handler over a fresh DB and
// returns a logged-in session cookie for the project's owner. A second
// "outsider" cookie is returned for the non-member case.
func newSearchServer(t *testing.T) (*httptest.Server, *http.Cookie, *http.Cookie, uuid.UUID) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := context.Background()
	owner, _, err := identity.UpsertFromGithub(ctx, pool, 9100, "owner", "Owner", "")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	outsider, _, err := identity.UpsertFromGithub(ctx, pool, 9101, "outsider", "Outsider", "")
	if err != nil {
		t.Fatalf("outsider: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "S", "", "", "", owner.ID, nil)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// Seed a task so search has something to match.
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "alpaca pipeline",
		TargetRoleID: &roles[0].ID,
	}, tasks.Authorship{UserID: &owner.ID}); err != nil {
		t.Fatalf("task: %v", err)
	}

	key := []byte("0123456789abcdef0123456789abcdef")
	resolver := identity.NewResolver(pool, key, false)

	ownerSess, err := identity.NewSession(ctx, pool, owner.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("owner session: %v", err)
	}
	outsiderSess, err := identity.NewSession(ctx, pool, outsider.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("outsider session: %v", err)
	}

	srv := httptest.NewServer(web.SearchHandler(web.SearchDeps{
		Pool: pool, Resolver: resolver,
	}))
	t.Cleanup(srv.Close)

	return srv,
		&http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(ownerSess.ID, key)},
		&http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(outsiderSess.ID, key)},
		p.ID
}

func TestAPISearch_Unauthenticated(t *testing.T) {
	srv, _, _, pid := newSearchServer(t)
	resp, err := http.Get(srv.URL + "?q=x&project_id=" + pid.String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestAPISearch_MissingProjectID(t *testing.T) {
	srv, cookie, _, _ := newSearchServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?q=x", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAPISearch_InvalidProjectID(t *testing.T) {
	srv, cookie, _, _ := newSearchServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?q=x&project_id=nope", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAPISearch_NonMemberGets404(t *testing.T) {
	srv, _, outsider, pid := newSearchServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?q=x&project_id="+pid.String(), nil)
	req.AddCookie(outsider)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestAPISearch_HappyPath(t *testing.T) {
	srv, cookie, _, pid := newSearchServer(t)
	q := url.Values{"q": {"alpaca"}, "project_id": {pid.String()}}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?"+q.Encode(), nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body struct {
		Hits []struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Hits) == 0 {
		t.Error("expected at least one hit")
	}
}

func TestAPISearch_EmptyQueryRejected(t *testing.T) {
	srv, cookie, _, pid := newSearchServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?q=&project_id="+pid.String(), nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}
