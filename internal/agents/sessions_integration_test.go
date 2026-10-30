package agents_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestTouch(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u1, _, err := identity.UpsertFromGithub(ctx, pool, 9701, "sess-a", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	u2, _, err := identity.UpsertFromGithub(ctx, pool, 9702, "sess-b", "B", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "SessProj", "", "", "", u1.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref := agents.Ref{ProjectID: p.ID, UserID: u1.ID, Key: "s-1", Label: "first", Client: "claude-code"}

	a, err := agents.Touch(ctx, pool, ref)
	if err != nil {
		t.Fatal(err)
	}
	if a.Label != "first" || a.Client != "claude-code" || a.Key != "s-1" {
		t.Fatalf("created session = %+v", a)
	}

	time.Sleep(10 * time.Millisecond)
	ref.Label, ref.Client = "", ""
	b, err := agents.Touch(ctx, pool, ref)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != a.ID || b.Label != "first" || b.Client != "claude-code" {
		t.Fatalf("empty label must keep the old one: %+v", b)
	}
	if !b.LastSeenAt.After(a.LastSeenAt) {
		t.Fatalf("last_seen_at did not move: %v -> %v", a.LastSeenAt, b.LastSeenAt)
	}

	ref.Label = "renamed"
	c, err := agents.Touch(ctx, pool, ref)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != a.ID || c.Label != "renamed" {
		t.Fatalf("new label must replace: %+v", c)
	}

	ref.Key = "  "
	if _, err := agents.Touch(ctx, pool, ref); err == nil {
		t.Fatal("blank key must error")
	}
	ref.Key = strings.Repeat("x", agents.MaxKeyLen+1)
	if _, err := agents.Touch(ctx, pool, ref); err == nil {
		t.Fatal("key over 128 characters must error")
	}
	ref.Key = strings.Repeat("x", agents.MaxKeyLen)
	if _, err := agents.Touch(ctx, pool, ref); err != nil {
		t.Fatalf("key of exactly 128 characters: %v", err)
	}
	// The limit counts characters, not bytes.
	ref.Key = strings.Repeat("ñ", agents.MaxKeyLen)
	if _, err := agents.Touch(ctx, pool, ref); err != nil {
		t.Fatalf("128 multibyte characters: %v", err)
	}
	ref.Key = strings.Repeat("ñ", agents.MaxKeyLen+1)
	if _, err := agents.Touch(ctx, pool, ref); err == nil {
		t.Fatal("129 multibyte characters must error")
	}

	// Same key under another user is a different session.
	ref.Key, ref.UserID = "s-1", u2.ID
	d, err := agents.Touch(ctx, pool, ref)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == a.ID {
		t.Fatal("other user must get its own session")
	}

	got, err := agents.ByIDs(ctx, pool, []uuid.UUID{a.ID, d.ID, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[a.ID].Label != "renamed" {
		t.Fatalf("ByIDs = %+v", got)
	}
}
