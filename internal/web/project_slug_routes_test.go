package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// The SPA puts whatever is in the URL into its API calls, and its own
// project links carry the slug. Every project route has to accept it,
// not just GET /api/projects/{id}.
func TestApiProjects_RoutesAcceptSlug(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 13601, "slug-user", "Slug", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	proj, err := identity.CreateProject(ctx, pool, "Slug Project", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: proj.ID, Type: tasks.TypeTask, Title: "visible through the slug",
	}, tasks.Authorship{UserID: &u.ID}); err != nil {
		t.Fatalf("Create task: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, proj.ID, "slug-token", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	ts := httptest.NewServer(NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token

	for _, path := range []string{"", "/tasks", "/roles", "/members", "/priorities", "/cycles", "/arch/nodes"} {
		r := doRaw(t, "GET", ts.URL+"/api/projects/"+proj.Slug+path, auth, nil)
		if r.StatusCode != http.StatusOK {
			t.Errorf("GET /api/projects/{slug}%s = %d %s, want 200", path, r.StatusCode, r.Body)
		}
	}

	// And the slug really resolves to this project, rather than
	// returning something empty that only looks like success.
	r := doRaw(t, "GET", ts.URL+"/api/projects/"+proj.Slug+"/tasks", auth, nil)
	if !strings.Contains(string(r.Body), "visible through the slug") {
		t.Errorf("tasks by slug did not return the project's task: %s", r.Body)
	}

	// A uuid still works, unchanged.
	r = doRaw(t, "GET", ts.URL+"/api/projects/"+proj.ID.String()+"/tasks", auth, nil)
	if r.StatusCode != http.StatusOK {
		t.Errorf("GET by uuid = %d, want 200", r.StatusCode)
	}

	// An unknown slug is left to the handler, which reports it as an
	// invalid id rather than confirming which slugs exist.
	r = doRaw(t, "GET", ts.URL+"/api/projects/no-such-project/tasks", auth, nil)
	if r.StatusCode == http.StatusOK {
		t.Errorf("unknown slug returned 200: %s", r.Body)
	}
}

// A token is scoped to one project. Naming another project by slug
// must not slip past the scope guard, which only checks uuids.
func TestApiProjects_SlugDoesNotBypassTokenScope(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 13602, "two-proj", "Two", "")
	mine, err := identity.CreateProject(ctx, pool, "Mine", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject mine: %v", err)
	}
	theirs, err := identity.CreateProject(ctx, pool, "Theirs", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject theirs: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, mine.ID, "scoped", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	ts := httptest.NewServer(NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token

	// Same project, by slug: allowed.
	if r := doRaw(t, "GET", ts.URL+"/api/projects/"+mine.Slug+"/tasks", auth, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("own project by slug = %d %s, want 200", r.StatusCode, r.Body)
	}
	// The other project, by slug: refused, exactly as by uuid. /roles,
	// /priorities, /cycles and /members run no scope check of their own
	// — the guard is all that stands between a token and another
	// project's taxonomy, which is why the slug has to be resolved
	// before it runs.
	for _, path := range []string{"/tasks", "/roles", "/priorities", "/cycles", "/members"} {
		bySlug := doRaw(t, "GET", ts.URL+"/api/projects/"+theirs.Slug+path, auth, nil)
		byID := doRaw(t, "GET", ts.URL+"/api/projects/"+theirs.ID.String()+path, auth, nil)
		if bySlug.StatusCode == http.StatusOK {
			t.Errorf("a slug let a scoped token read %s of another project: %s", path, bySlug.Body)
		}
		if bySlug.StatusCode != byID.StatusCode {
			t.Errorf("%s: slug and uuid disagree for a foreign project: %d vs %d", path, bySlug.StatusCode, byID.StatusCode)
		}
	}
}
