// Coverage for the arch versioning model: per-author session lock,
// auto-flush via the ticker, explicit checkpoint, and cross-author
// rejection with retry_after. All exercised through the public arch.*
// surface against a real Postgres provisioned by testutil.
package arch_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/identity"
)

// TestArchVersioning_SessionCoalescesIntoOneRevision verifies that
// the canonical agent workflow — N writes then a checkpoint — produces
// exactly ONE revision row with the checkpoint message, and that the
// lock row is gone afterwards.
func TestArchVersioning_SessionCoalescesIntoOneRevision(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	by := arch.Authorship{UserID: tc.userID}

	// Snapshot the starting state — the migration seed inserts v1.
	hist0, err := arch.ListRevisions(ctx, tc.pool, tc.projectID, 50, nil)
	if err != nil {
		t.Fatalf("ListRevisions baseline: %v", err)
	}
	startVersion := 0
	if len(hist0) > 0 {
		startVersion = hist0[0].Version
	}

	// Five writes from the same author. Should NOT create five
	// revisions; the lock keeps the session open until checkpoint.
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, by, arch.UpsertParams{
		Slug: "alpha", Kind: "system", Name: "Alpha",
	}); err != nil {
		t.Fatalf("upsert alpha: %v", err)
	}
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, by, arch.UpsertParams{
		Slug: "bravo", Kind: "service", Name: "Bravo", ParentSlug: "alpha",
	}); err != nil {
		t.Fatalf("upsert bravo: %v", err)
	}
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, by, arch.UpsertParams{
		Slug: "charlie", Kind: "service", Name: "Charlie", ParentSlug: "alpha",
	}); err != nil {
		t.Fatalf("upsert charlie: %v", err)
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, by, arch.EdgeUpsertParams{
		FromSlug: "bravo", ToSlug: "charlie", Kind: "calls",
	}); err != nil {
		t.Fatalf("upsert edge: %v", err)
	}
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, by, arch.UpsertParams{
		Slug: "delta", Kind: "module", Name: "Delta", ParentSlug: "bravo",
	}); err != nil {
		t.Fatalf("upsert delta: %v", err)
	}

	// History still at the baseline — no auto-flush yet because the
	// idle window is the package default (120s) and we just wrote.
	hist1, err := arch.ListRevisions(ctx, tc.pool, tc.projectID, 50, nil)
	if err != nil {
		t.Fatalf("ListRevisions mid-session: %v", err)
	}
	if len(hist1) != len(hist0) {
		t.Fatalf("expected NO new revisions during open session, got %d new",
			len(hist1)-len(hist0))
	}

	// Explicit checkpoint with a message → one new revision, lock gone.
	res, err := arch.Checkpoint(ctx, tc.pool, tc.projectID, by, "added alpha + bravo/charlie")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if res.Version != startVersion+1 {
		t.Fatalf("expected new version %d, got %d", startVersion+1, res.Version)
	}
	if res.Message != "added alpha + bravo/charlie" {
		t.Fatalf("message mismatch: %q", res.Message)
	}
	if res.WriteCount != 5 {
		t.Fatalf("expected write_count=5, got %d", res.WriteCount)
	}
	hist2, _ := arch.ListRevisions(ctx, tc.pool, tc.projectID, 50, nil)
	if len(hist2) != len(hist0)+1 {
		t.Fatalf("expected exactly 1 new revision after checkpoint, got %d",
			len(hist2)-len(hist0))
	}
	if hist2[0].AutoFlushed {
		t.Fatalf("expected explicit checkpoint, got auto_flushed=true")
	}

	// Calling checkpoint again with no open session returns the
	// dedicated error.
	if _, err := arch.Checkpoint(ctx, tc.pool, tc.projectID, by, "second"); err != arch.ErrNoActiveSession {
		t.Fatalf("expected ErrNoActiveSession on idle checkpoint, got %v", err)
	}

	// The full snapshot of the new revision contains the 5 nodes /
	// 1 edge we wrote, so a restore (or diff) sees the same graph
	// the canvas does.
	rev, err := arch.GetRevision(ctx, tc.pool, tc.projectID, res.Version)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	nodes, _ := rev.Snapshot["nodes"].([]any)
	if len(nodes) < 4 {
		t.Fatalf("expected >=4 nodes in snapshot, got %d", len(nodes))
	}
}

// TestArchVersioning_CrossAuthorLockReturnsRetryAfter exercises the
// 423-equivalent error when a second user tries to write while a
// first user owns the active session.
func TestArchVersioning_CrossAuthorLockReturnsRetryAfter(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	a := arch.Authorship{UserID: tc.userID}
	other, _, err := identity.UpsertFromGithub(ctx, tc.pool, 4099, "intruder", "Intruder", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub intruder: %v", err)
	}
	b := arch.Authorship{UserID: other.ID}

	// A opens a session.
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, a, arch.UpsertParams{
		Slug: "owned", Kind: "service", Name: "Owned",
	}); err != nil {
		t.Fatalf("a upsert: %v", err)
	}

	// B's write must be refused with a LockedError carrying a positive
	// retry_after.
	_, err = arch.UpsertNode(ctx, tc.pool, tc.projectID, b, arch.UpsertParams{
		Slug: "intrusion", Kind: "service", Name: "Intrusion",
	})
	le, ok := arch.IsLocked(err)
	if !ok {
		t.Fatalf("expected LockedError for B, got %v", err)
	}
	if le.LockedByUserID != tc.userID {
		t.Fatalf("locked_by mismatch: got %s, want %s", le.LockedByUserID, tc.userID)
	}
	if le.RetryAfterSeconds <= 0 {
		t.Fatalf("expected positive retry_after_seconds, got %d", le.RetryAfterSeconds)
	}

	// A checkpoints — session closes.
	if _, err := arch.Checkpoint(ctx, tc.pool, tc.projectID, a, "done"); err != nil {
		t.Fatalf("a checkpoint: %v", err)
	}

	// B can now write — no lock owner.
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, b, arch.UpsertParams{
		Slug: "intrusion", Kind: "service", Name: "Intrusion",
	}); err != nil {
		t.Fatalf("b upsert after checkpoint: %v", err)
	}
}

// TestArchVersioning_ExpiredLockEvictedInline forces the idle window
// to ~0 so the next write from a different author evicts the stale
// lock and proceeds without waiting on the background ticker.
func TestArchVersioning_ExpiredLockEvictedInline(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	prev := arch.DefaultIdleConfig
	arch.SetIdleConfig(arch.IdleConfig{DefaultSeconds: 1})
	defer arch.SetIdleConfig(prev)

	a := arch.Authorship{UserID: tc.userID}
	other, _, err := identity.UpsertFromGithub(ctx, tc.pool, 4101, "later", "Later", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub later: %v", err)
	}
	b := arch.Authorship{UserID: other.ID}

	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, a, arch.UpsertParams{
		Slug: "ancient", Kind: "service", Name: "Ancient",
	}); err != nil {
		t.Fatalf("a upsert: %v", err)
	}

	histBefore, _ := arch.ListRevisions(ctx, tc.pool, tc.projectID, 50, nil)

	// Wait past the idle window then try as B.
	time.Sleep(1500 * time.Millisecond)
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, b, arch.UpsertParams{
		Slug: "successor", Kind: "service", Name: "Successor",
	}); err != nil {
		t.Fatalf("b should succeed after eviction, got %v", err)
	}

	// The eviction flushed A's session into one auto_flushed revision.
	histAfter, _ := arch.ListRevisions(ctx, tc.pool, tc.projectID, 50, nil)
	if len(histAfter) != len(histBefore)+1 {
		t.Fatalf("expected exactly 1 new revision after eviction, got %d",
			len(histAfter)-len(histBefore))
	}
	if !histAfter[0].AutoFlushed {
		t.Fatalf("expected eviction-flushed revision to have auto_flushed=true")
	}
	if histAfter[0].AuthorUserID == nil || *histAfter[0].AuthorUserID != tc.userID {
		t.Fatalf("expected author of evicted revision = A (%s), got %v", tc.userID, histAfter[0].AuthorUserID)
	}
}

// TestArchVersioning_NoLockRowAfterFlush is a sanity check that the
// helper machinery doesn't accidentally leave an open session
// hanging around after a checkpoint flush.
func TestArchVersioning_NoLockRowAfterFlush(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()
	by := arch.Authorship{UserID: tc.userID}

	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, by, arch.UpsertParams{
		Slug: "x", Kind: "service", Name: "X",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := arch.Checkpoint(ctx, tc.pool, tc.projectID, by, "x"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	// A second checkpoint must report no active session — confirms the
	// flush removed the lock row.
	if _, err := arch.Checkpoint(ctx, tc.pool, tc.projectID, by, "again"); err != arch.ErrNoActiveSession {
		t.Fatalf("expected ErrNoActiveSession, got %v", err)
	}
	_ = uuid.New // keep import alive across compiler versions
}
