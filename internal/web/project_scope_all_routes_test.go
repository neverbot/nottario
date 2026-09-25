package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// routeDecl matches the route table in server.go. The test reads the
// source rather than a hand-kept list so a route added tomorrow is
// covered without anyone remembering to add it here.
var routeDecl = regexp.MustCompile(`mux\.Handle\("([A-Z]+) (/api/projects/[^"]*)"`)

func projectRoutes(t *testing.T) [][2]string {
	t.Helper()
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	matches := routeDecl.FindAllStringSubmatch(string(src), -1)
	if len(matches) < 20 {
		t.Fatalf("found %d project routes in server.go, expected the full table — has the routing style changed?", len(matches))
	}
	out := make([][2]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// Every project route, without exception, must refuse a token issued
// for a different project — whether the caller names the project by
// uuid or by slug. One token = one project is the whole promise of the
// API token, and several handlers (roles, priorities, cycles, members)
// enforce it only through the router wrapper, so an unwrapped route
// would be an open door with nothing to flag it.
func TestApiProjects_EveryRouteRefusesAForeignToken(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 13701, "scope-http", "Scope", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	// The user belongs to both projects: the only thing standing
	// between the token and project B is the scope check itself.
	mine, err := identity.CreateProject(ctx, pool, "Mine HTTP", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject mine: %v", err)
	}
	theirs, err := identity.CreateProject(ctx, pool, "Theirs HTTP", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject theirs: %v", err)
	}
	victim, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: theirs.ID, Type: tasks.TypeTask, Title: "not yours",
	}, tasks.Authorship{UserID: &u.ID})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, theirs.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v (%d)", err, len(roles))
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, mine.ID, "scoped-http", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	ts := httptest.NewServer(NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	}))
	t.Cleanup(ts.Close)
	auth := "Bearer " + token

	fill := func(path, project string) string {
		r := strings.NewReplacer(
			"{id}", project,
			"{project_id}", project,
			"{task_id}", victim.ID.String(),
			"{user_id}", u.ID.String(),
			"{role_id}", roles[0].ID.String(),
			"{token_id}", uuid.NewString(),
			"{edge_id}", uuid.NewString(),
			"{dep_id}", uuid.NewString(),
			"{comment_id}", uuid.NewString(),
			"{slug}", "some-node",
			"{key}", "backend",
			"{version}", "1",
		)
		return r.Replace(path)
	}

	for _, route := range projectRoutes(t) {
		method, path := route[0], route[1]
		for _, naming := range []struct{ label, project string }{
			{"uuid", theirs.ID.String()},
			{"slug", theirs.Slug},
		} {
			url := ts.URL + fill(path, naming.project)
			r := doRaw(t, method, url, auth, []byte(`{}`))
			switch r.StatusCode {
			case http.StatusForbidden, http.StatusNotFound:
				// 403 names the scope violation, 404 hides the project
				// from someone who should not know it exists. Both are
				// the access check answering.
			default:
				// Anything else means the request died for another
				// reason — a rejected body, a missing field — and this
				// route would pass the test even with the check gone.
				t.Errorf("%s %s by %s: %d, want 403 or 404 from the access check\n%s",
					method, path, naming.label, r.StatusCode, r.Body)
			}
		}
	}

	// The same routes must still work for the project the token owns,
	// or the test above would pass on a server that refuses everything.
	if r := doRaw(t, "GET", ts.URL+"/api/projects/"+mine.Slug+"/tasks", auth, nil); r.StatusCode != http.StatusOK {
		t.Errorf("own project by slug = %d %s, want 200", r.StatusCode, r.Body)
	}
	if r := doRaw(t, "GET", ts.URL+"/api/projects/"+mine.ID.String()+"/roles", auth, nil); r.StatusCode != http.StatusOK {
		t.Errorf("own project roles by uuid = %d %s, want 200", r.StatusCode, r.Body)
	}
}
