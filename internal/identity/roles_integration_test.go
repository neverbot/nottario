package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestDeleteRole_TokenDefaultSetNull covers the regression where a
// role could not be deleted if any api_token had it as its
// default_role_id. The FK is now ON DELETE SET NULL; the token stays
// but its default is cleared.
func TestDeleteRole_TokenDefaultSetNull(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 2001, "owner", "Owner", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "Regr", "regression", "go", "service", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	var frontendID string
	for _, r := range roles {
		if r.Key == "frontend" {
			frontendID = r.ID.String()
		}
	}
	if frontendID == "" {
		t.Fatalf("seeded frontend role missing: %+v", roles)
	}

	// Issue a token pinned to the frontend role — this is what
	// blocked the delete before the FK was switched to SET NULL.
	frontendUUID := roles[0].ID
	for _, r := range roles {
		if r.Key == "frontend" {
			frontendUUID = r.ID
			break
		}
	}
	_, tok, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "pinned", &frontendUUID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if tok.DefaultRoleID == nil || *tok.DefaultRoleID != frontendUUID {
		t.Fatalf("token default not set to frontend: %+v", tok)
	}

	if err := identity.DeleteRole(ctx, pool, frontendUUID); err != nil {
		t.Fatalf("DeleteRole should succeed even with a token referencing it, got: %v", err)
	}

	// Re-fetch the token via a raw query — ListTokens redacts other
	// users' rows, but we own this one.
	var defRoleID *string
	if err := pool.QueryRow(ctx,
		`SELECT default_role_id::text FROM api_tokens WHERE id = $1`, tok.ID,
	).Scan(&defRoleID); err != nil {
		t.Fatalf("scan default_role_id: %v", err)
	}
	if defRoleID != nil {
		t.Fatalf("expected default_role_id NULL after delete, got %q", *defRoleID)
	}
}
