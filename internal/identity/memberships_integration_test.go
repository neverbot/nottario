package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestDeleteRole_KeepsMembership is the regression guard for feature
// 663b7219. Before the split, deleting a role cascaded to memberships
// and kicked members with only that role out of the project. After
// the split, deleting a role just clears the role assignment; the
// member row survives.
func TestDeleteRole_KeepsMembership(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 3001, "creator", "Creator", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "SplitRegression", "regression", "go", "service", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// The creator is auto-joined with every default role.
	initialRoles, err := identity.UserRoleIDs(ctx, pool, u.ID, p.ID)
	if err != nil {
		t.Fatalf("UserRoleIDs before: %v", err)
	}
	if got, want := len(initialRoles), len(identity.DefaultRoleCatalogue); got != want {
		t.Fatalf("expected %d seeded roles, got %d", want, got)
	}

	// Delete every role in the project.
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	for _, r := range roles {
		if err := identity.DeleteRole(ctx, pool, r.ID); err != nil {
			t.Fatalf("DeleteRole %s: %v", r.Key, err)
		}
	}

	// The creator must still be a member of the project — visible in
	// the members list, listed via ListProjects, no role assignments.
	afterRoles, err := identity.UserRoleIDs(ctx, pool, u.ID, p.ID)
	if err != nil {
		t.Fatalf("UserRoleIDs after: %v", err)
	}
	if len(afterRoles) != 0 {
		t.Fatalf("expected 0 role assignments after wiping the catalogue, got %d", len(afterRoles))
	}

	members, err := identity.ListMembers(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	// One row, RoleID nil (LEFT JOIN placeholder for a role-less member).
	if len(members) != 1 {
		t.Fatalf("expected 1 member row, got %d: %+v", len(members), members)
	}
	if members[0].UserID != u.ID {
		t.Fatalf("expected creator as member, got %s", members[0].UserID)
	}
	if members[0].RoleID != nil {
		t.Fatalf("expected RoleID nil for role-less member, got %s", members[0].RoleID)
	}

	visible, err := identity.ListProjects(ctx, pool, u.ID, false /*isAdmin*/)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	var seen bool
	for _, pr := range visible {
		if pr.ID == p.ID {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("creator should still see the project after every role was deleted")
	}
}

// TestRemoveMember_CascadesRoleAssignments ensures that removing a
// member from a project cleanly drops all their role rows via the
// composite FK on membership_roles.
func TestRemoveMember_CascadesRoleAssignments(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	owner, _, err := identity.UpsertFromGithub(ctx, pool, 3101, "owner", "Owner", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub owner: %v", err)
	}
	other, _, err := identity.UpsertFromGithub(ctx, pool, 3102, "other", "Other", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub other: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "CascadeTest", "cascade", "go", "service", owner.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v (got %d)", err, len(roles))
	}
	if err := identity.AddMembership(ctx, pool, other.ID, p.ID, roles[0].ID); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	if err := identity.AddMembership(ctx, pool, other.ID, p.ID, roles[1].ID); err != nil {
		t.Fatalf("AddMembership 2: %v", err)
	}
	if got, err := identity.UserRoleIDs(ctx, pool, other.ID, p.ID); err != nil || len(got) != 2 {
		t.Fatalf("UserRoleIDs before: %v (got %d)", err, len(got))
	}

	if err := identity.RemoveMember(ctx, pool, other.ID, p.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	after, err := identity.UserRoleIDs(ctx, pool, other.ID, p.ID)
	if err != nil {
		t.Fatalf("UserRoleIDs after: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected role assignments cleared, got %d", len(after))
	}

	// The member row itself is gone too — no LEFT JOIN placeholder.
	members, err := identity.ListMembers(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range members {
		if m.UserID == other.ID {
			t.Fatalf("removed member should not appear in ListMembers, got %+v", m)
		}
	}
}

// TestUnassignLastRole_KeepsMember: dropping a member's last role
// leaves them in the project (they just have no role assignments).
func TestUnassignLastRole_KeepsMember(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	owner, _, err := identity.UpsertFromGithub(ctx, pool, 3201, "owner", "Owner", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "LastRole", "last-role", "go", "service", owner.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	for _, r := range roles {
		if err := identity.RemoveMembership(ctx, pool, owner.ID, p.ID, r.ID); err != nil {
			t.Fatalf("RemoveMembership %s: %v", r.Key, err)
		}
	}
	after, err := identity.UserRoleIDs(ctx, pool, owner.ID, p.ID)
	if err != nil {
		t.Fatalf("UserRoleIDs: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected 0 role assignments, got %d", len(after))
	}
	members, err := identity.ListMembers(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	var seen bool
	for _, m := range members {
		if m.UserID == owner.ID {
			seen = true
			if m.RoleID != nil {
				t.Fatalf("expected role-less member, got RoleID=%s", m.RoleID)
			}
		}
	}
	if !seen {
		t.Fatalf("owner should still appear in ListMembers after every role was unassigned")
	}
}
