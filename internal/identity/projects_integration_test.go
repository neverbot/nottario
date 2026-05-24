package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestCreateProject_SeedsRolesAndPriorities(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, created, err := identity.UpsertFromGithub(ctx, pool, 1001, "creator", "Creator", "")
	if err != nil || !created {
		t.Fatalf("UpsertFromGithub: %v created=%v", err, created)
	}

	p, err := identity.CreateProject(ctx, pool, "Demo", "A demo", "go", "service", u.ID, []string{"https://example.com/repo"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Slug == "" || p.Name != "Demo" {
		t.Fatalf("unexpected project: %+v", p)
	}
	if len(p.Repos) != 1 || p.Repos[0] != "https://example.com/repo" {
		t.Fatalf("expected attached repo, got %+v", p.Repos)
	}

	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	want := map[string]bool{"backend": false, "frontend": false, "qa": false, "design": false}
	for _, r := range roles {
		want[r.Key] = true
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("expected seeded role %q, missing; got %+v", k, roles)
		}
	}

	prios, err := identity.ListPriorities(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListPriorities: %v", err)
	}
	if len(prios) == 0 {
		t.Fatalf("expected seeded priorities, got none")
	}
	medium, err := identity.ResolvePriorityKey(ctx, pool, p.ID, "medium")
	if err != nil {
		t.Fatalf("ResolvePriorityKey(medium): %v", err)
	}
	if medium < 0 || medium > 100 {
		t.Fatalf("medium priority out of range: %d", medium)
	}

	creatorRoles, err := identity.UserRoleIDs(ctx, pool, u.ID, p.ID)
	if err != nil {
		t.Fatalf("UserRoleIDs: %v", err)
	}
	if len(creatorRoles) != len(identity.DefaultRoleCatalogue) {
		t.Fatalf("expected creator auto-joined with %d roles, got %d", len(identity.DefaultRoleCatalogue), len(creatorRoles))
	}
}
