// Package arch_test covers the repository surface end-to-end against
// a real Postgres database, provisioned per-test by internal/testutil.
//
// Coverage focus (mirrors the public API in nodes.go, edges.go,
// kinds.go, links.go):
//   - Kinds: default seed, custom upsert, deletion guarded by in-use.
//   - Nodes: validation, parent resolution, cycle checks (upsert +
//     move), cascade vs no-cascade delete, slug-based lookup.
//   - Edges: kind/from/to validation, list + filter (in/out/kind),
//     remove and ErrEdgeNotFound on missing id.
//   - Links: doc + task variants, listing, unlinking.
//
// The suite intentionally avoids reaching into dbq.* — every check
// runs through the high-level arch.* surface that the HTTP handlers
// and MCP tools also consume.
package arch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

type testProjectCtx struct {
	pool      *pgxpool.Pool
	projectID uuid.UUID
	userID    uuid.UUID
}

func seedProject(t *testing.T) (context.Context, *testProjectCtx, func()) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	u, _, err := identity.UpsertFromGithub(ctx, pool, 4001, "archer", "Archer", "")
	if err != nil {
		cancel()
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "ArchProj", "", "", "", u.ID)
	if err != nil {
		cancel()
		t.Fatalf("CreateProject: %v", err)
	}
	return ctx, &testProjectCtx{pool: pool, projectID: p.ID, userID: u.ID}, cancel
}

// Tests --------------------------------------------------------------

func TestKinds_DefaultsAndCustom(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	// ListKinds auto-seeds the catalogue.
	kinds, err := arch.ListKinds(ctx, tc.pool, tc.projectID)
	if err != nil {
		t.Fatalf("ListKinds: %v", err)
	}
	if len(kinds) != len(arch.DefaultKinds) {
		t.Fatalf("default kinds: got %d, want %d", len(kinds), len(arch.DefaultKinds))
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		seen[k.Key] = true
		if !k.IsDefault {
			t.Errorf("kind %q expected IsDefault=true", k.Key)
		}
	}
	for _, want := range arch.DefaultKinds {
		if !seen[want.Key] {
			t.Errorf("default kind missing: %q", want.Key)
		}
	}

	// EnsureDefaultKinds is idempotent — running it again does not
	// duplicate rows.
	if err := arch.EnsureDefaultKinds(ctx, tc.pool, tc.projectID); err != nil {
		t.Fatalf("EnsureDefaultKinds (second call): %v", err)
	}
	kinds2, _ := arch.ListKinds(ctx, tc.pool, tc.projectID)
	if len(kinds2) != len(arch.DefaultKinds) {
		t.Fatalf("kinds duplicated after re-ensure: %d", len(kinds2))
	}

	// UpsertKind: insert custom, then update its label.
	custom, err := arch.UpsertKind(ctx, tc.pool, tc.projectID, arch.Kind{
		Key: "worker", Label: "Worker", Color: "#abcdef", Description: "background job",
	})
	if err != nil {
		t.Fatalf("UpsertKind insert: %v", err)
	}
	if custom.Label != "Worker" || custom.IsDefault {
		t.Fatalf("custom kind unexpected: label=%q isDefault=%v", custom.Label, custom.IsDefault)
	}

	updated, err := arch.UpsertKind(ctx, tc.pool, tc.projectID, arch.Kind{
		Key: "worker", Label: "Background Worker", Color: "#abcdef",
	})
	if err != nil {
		t.Fatalf("UpsertKind update: %v", err)
	}
	if updated.Label != "Background Worker" {
		t.Fatalf("update did not stick: %q", updated.Label)
	}

	// Required fields.
	if _, err := arch.UpsertKind(ctx, tc.pool, tc.projectID, arch.Kind{Key: ""}); err == nil {
		t.Error("UpsertKind with empty key should fail")
	}
	if _, err := arch.UpsertKind(ctx, tc.pool, tc.projectID, arch.Kind{Key: "ok"}); err == nil {
		t.Error("UpsertKind with empty label should fail")
	}

	// Delete: unused kind goes; in-use kind refuses.
	if err := arch.DeleteKind(ctx, tc.pool, tc.projectID, "worker"); err != nil {
		t.Fatalf("DeleteKind unused: %v", err)
	}
	// Missing key.
	if err := arch.DeleteKind(ctx, tc.pool, tc.projectID, "nope"); err == nil {
		t.Error("DeleteKind on missing key should fail")
	}
	// In use: create a node with kind="service", then try to delete.
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "svc-a", Kind: "service", Name: "Svc A",
	}); err != nil {
		t.Fatalf("UpsertNode for in-use kind test: %v", err)
	}
	if err := arch.DeleteKind(ctx, tc.pool, tc.projectID, "service"); !errors.Is(err, arch.ErrKindInUse) {
		t.Fatalf("DeleteKind in-use: got %v, want ErrKindInUse", err)
	}
}

func TestNodes_Validation(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	// Empty slug
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Kind: "service", Name: "X",
	}); !errors.Is(err, arch.ErrSlugRequired) {
		t.Errorf("empty slug: got %v, want ErrSlugRequired", err)
	}
	// Bad slug
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "Bad Slug", Kind: "service", Name: "X",
	}); err == nil {
		t.Error("bad slug should fail validation")
	}
	// Empty name
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "x", Kind: "service", Name: "  ",
	}); !errors.Is(err, arch.ErrNameRequired) {
		t.Errorf("empty name: got %v, want ErrNameRequired", err)
	}
	// Unknown kind
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "x", Kind: "made-up", Name: "X",
	}); !errors.Is(err, arch.ErrInvalidKind) {
		t.Errorf("bad kind: got %v, want ErrInvalidKind", err)
	}
	// Missing parent
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "x", Kind: "service", Name: "X", ParentSlug: "ghost",
	}); !errors.Is(err, arch.ErrParentMissing) {
		t.Errorf("missing parent: got %v, want ErrParentMissing", err)
	}
}

func TestNodes_InsertUpdateGetList(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	root, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "sys", Kind: "system", Name: "System",
		Metadata: map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("insert root: %v", err)
	}
	if root.ParentID != nil {
		t.Errorf("root.ParentID should be nil, got %v", root.ParentID)
	}
	if root.Metadata["env"] != "prod" {
		t.Errorf("metadata round-trip: %v", root.Metadata)
	}

	child, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "sys.api", Kind: "service", Name: "API",
		ParentSlug: "sys",
	})
	if err != nil {
		t.Fatalf("insert child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != root.ID {
		t.Errorf("child parent id mismatch: %v", child.ParentID)
	}

	// Update: same slug → update path (label change picked up).
	updated, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "sys.api", Kind: "service", Name: "API v2", ParentSlug: "sys",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ID != child.ID || updated.Name != "API v2" {
		t.Errorf("update: id changed or name not applied: %+v", updated)
	}

	// Get by slug.
	got, err := arch.GetNode(ctx, tc.pool, tc.projectID, "sys.api")
	if err != nil || got.ID != child.ID {
		t.Fatalf("GetNode: %v / id mismatch", err)
	}
	if _, err := arch.GetNode(ctx, tc.pool, tc.projectID, "missing"); !errors.Is(err, arch.ErrNodeNotFound) {
		t.Errorf("GetNode missing: got %v, want ErrNodeNotFound", err)
	}

	// ListNodes rootOnly + by parent.
	roots, err := arch.ListNodes(ctx, tc.pool, tc.projectID, "", true)
	if err != nil {
		t.Fatalf("ListNodes roots: %v", err)
	}
	if len(roots) != 1 || roots[0].Slug != "sys" {
		t.Errorf("roots: %+v", roots)
	}
	kids, err := arch.ListNodes(ctx, tc.pool, tc.projectID, "sys", false)
	if err != nil {
		t.Fatalf("ListNodes by parent: %v", err)
	}
	if len(kids) != 1 || kids[0].Slug != "sys.api" {
		t.Errorf("kids: %+v", kids)
	}
}

// TestNodes_HTMLEntitiesDecodedAtBoundary ensures that the rare agent
// that hands us an HTML-entity-encoded name or description (a
// `Pages &amp; Router` style payload that the Lit renderer would
// otherwise re-escape and surface literally to users) gets the
// entities decoded at the UpsertNode boundary. The DB row holds
// plain UTF-8 from then on.
func TestNodes_HTMLEntitiesDecodedAtBoundary(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	row, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "viewer.pages", Kind: "module",
		Name:          "Pages &amp; Router",
		DescriptionMD: "Routes &lt;/&gt; &amp; render &quot;leaf&quot; pages.",
	})
	if err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if row.Name != "Pages & Router" {
		t.Errorf("name not decoded: %q", row.Name)
	}
	if row.DescriptionMD != "Routes </> & render \"leaf\" pages." {
		t.Errorf("description not decoded: %q", row.DescriptionMD)
	}
}

func TestNodes_MoveAndCycles(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	mk := func(slug, parent string) {
		t.Helper()
		if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
			Slug: slug, Kind: "service", Name: slug, ParentSlug: parent,
		}); err != nil {
			t.Fatalf("upsert %s: %v", slug, err)
		}
	}
	// sys → a → b → c. Standalone "other" root.
	mk("sys", "")
	mk("a", "sys")
	mk("b", "a")
	mk("c", "b")
	mk("other", "")

	// Self-cycle via UpsertNode update.
	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "a", Kind: "service", Name: "a", ParentSlug: "a",
	}); !errors.Is(err, arch.ErrCycle) {
		t.Errorf("self-cycle on upsert: got %v, want ErrCycle", err)
	}

	// Descendant cycle: move a under c (its grand-grand-child).
	if _, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "a", "c"); !errors.Is(err, arch.ErrCycle) {
		t.Errorf("descendant cycle: got %v, want ErrCycle", err)
	}

	// Self move.
	if _, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "a", "a"); !errors.Is(err, arch.ErrCycle) {
		t.Errorf("self move: got %v, want ErrCycle", err)
	}

	// Valid: move c under "other".
	moved, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "c", "other")
	if err != nil {
		t.Fatalf("MoveNode valid: %v", err)
	}
	if moved.ParentID == nil {
		t.Fatalf("moved.ParentID nil")
	}

	// Promote to root: MoveNode with empty parent.
	rooted, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "c", "")
	if err != nil {
		t.Fatalf("MoveNode to root: %v", err)
	}
	if rooted.ParentID != nil {
		t.Errorf("promoted node should have nil ParentID, got %v", rooted.ParentID)
	}

	// Move missing node + missing parent.
	if _, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "ghost", ""); !errors.Is(err, arch.ErrNodeNotFound) {
		t.Errorf("move ghost: got %v, want ErrNodeNotFound", err)
	}
	if _, err := arch.MoveNode(ctx, tc.pool, tc.projectID, "a", "ghost-parent"); !errors.Is(err, arch.ErrParentMissing) {
		t.Errorf("move to ghost parent: got %v, want ErrParentMissing", err)
	}
}

func TestNodes_RemoveCascade(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	mk := func(slug, parent string) {
		t.Helper()
		if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
			Slug: slug, Kind: "service", Name: slug, ParentSlug: parent,
		}); err != nil {
			t.Fatalf("upsert %s: %v", slug, err)
		}
	}
	mk("sys", "")
	mk("sys.a", "sys")
	mk("sys.b", "sys")

	// Without cascade, removing a parent with children fails.
	if err := arch.RemoveNode(ctx, tc.pool, tc.projectID, "sys", false); err == nil {
		t.Error("RemoveNode without cascade on parent should fail")
	}

	// With cascade, the whole subtree disappears.
	if err := arch.RemoveNode(ctx, tc.pool, tc.projectID, "sys", true); err != nil {
		t.Fatalf("RemoveNode cascade: %v", err)
	}
	left, err := arch.ListNodes(ctx, tc.pool, tc.projectID, "", false)
	if err != nil {
		t.Fatalf("ListNodes after cascade: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("expected empty tree after cascade, got %d", len(left))
	}

	// Removing a missing node surfaces ErrNodeNotFound.
	if err := arch.RemoveNode(ctx, tc.pool, tc.projectID, "ghost", false); !errors.Is(err, arch.ErrNodeNotFound) {
		t.Errorf("Remove missing: got %v, want ErrNodeNotFound", err)
	}
}

func TestEdges_FullCycle(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	mk := func(slug string) {
		t.Helper()
		if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
			Slug: slug, Kind: "service", Name: slug,
		}); err != nil {
			t.Fatalf("upsert %s: %v", slug, err)
		}
	}
	mk("a")
	mk("b")
	mk("c")

	// Validation: empty slugs / empty kind / self-loop.
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "", ToSlug: "b", Kind: "calls",
	}); err == nil {
		t.Error("empty from should fail")
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "a", ToSlug: "a", Kind: "calls",
	}); err == nil {
		t.Error("self-loop should fail")
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "a", ToSlug: "b", Kind: "",
	}); err == nil {
		t.Error("empty kind should fail")
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "ghost", ToSlug: "a", Kind: "calls",
	}); err == nil {
		t.Error("missing from slug should fail")
	}

	// Insert two outgoing from "a" and one outgoing from "b".
	eAB, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "a", ToSlug: "b", Kind: "calls", Label: "AB",
	})
	if err != nil {
		t.Fatalf("upsert a→b: %v", err)
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "a", ToSlug: "c", Kind: "uses", Label: "AC",
	}); err != nil {
		t.Fatalf("upsert a→c: %v", err)
	}
	if _, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "b", ToSlug: "c", Kind: "uses", Label: "BC",
	}); err != nil {
		t.Fatalf("upsert b→c: %v", err)
	}

	// Upsert key is (from, to, kind): replaying with the same kind
	// updates rather than duplicates.
	again, err := arch.UpsertEdge(ctx, tc.pool, tc.projectID, arch.EdgeUpsertParams{
		FromSlug: "a", ToSlug: "b", Kind: "calls", Label: "AB-v2",
	})
	if err != nil {
		t.Fatalf("upsert a→b again: %v", err)
	}
	if again.ID != eAB.ID || again.Label != "AB-v2" {
		t.Errorf("upsert dedup: id changed or label not updated: %+v", again)
	}

	// ListEdges unfiltered.
	all, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{})
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListEdges total: got %d, want 3", len(all))
	}

	// Filter by node + direction.
	outA, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{
		NodeSlug: "a", Direction: "out",
	})
	if err != nil {
		t.Fatalf("ListEdges out a: %v", err)
	}
	if len(outA) != 2 {
		t.Errorf("a outgoing: got %d, want 2", len(outA))
	}
	inC, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{
		NodeSlug: "c", Direction: "in",
	})
	if err != nil {
		t.Fatalf("ListEdges in c: %v", err)
	}
	if len(inC) != 2 {
		t.Errorf("c incoming: got %d, want 2", len(inC))
	}
	bothB, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{
		NodeSlug: "b", Direction: "",
	})
	if err != nil {
		t.Fatalf("ListEdges both b: %v", err)
	}
	if len(bothB) != 2 {
		t.Errorf("b both: got %d, want 2", len(bothB))
	}

	// Filter by kind.
	uses, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{Kind: "uses"})
	if err != nil {
		t.Fatalf("ListEdges kind=uses: %v", err)
	}
	if len(uses) != 2 {
		t.Errorf("uses edges: got %d, want 2", len(uses))
	}

	// Remove by id.
	if err := arch.RemoveEdge(ctx, tc.pool, tc.projectID, eAB.ID); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
	if err := arch.RemoveEdge(ctx, tc.pool, tc.projectID, eAB.ID); !errors.Is(err, arch.ErrEdgeNotFound) {
		t.Errorf("RemoveEdge twice: got %v, want ErrEdgeNotFound", err)
	}
	// Filter by an unknown node slug surfaces as an error.
	if _, err := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{NodeSlug: "ghost"}); err == nil {
		t.Error("ListEdges with unknown node slug should fail")
	}

	// Cascade: removing a node deletes its edges.
	if err := arch.RemoveNode(ctx, tc.pool, tc.projectID, "c", true); err != nil {
		t.Fatalf("RemoveNode c: %v", err)
	}
	remaining, _ := arch.ListEdges(ctx, tc.pool, tc.projectID, arch.EdgeFilter{})
	for _, e := range remaining {
		if e.ToSlug == "c" || e.FromSlug == "c" {
			t.Errorf("edge to/from c survived node delete: %+v", e)
		}
	}
}

func TestLinks_DocAndTask(t *testing.T) {
	ctx, tc, cancel := seedProject(t)
	defer cancel()

	if _, err := arch.UpsertNode(ctx, tc.pool, tc.projectID, arch.UpsertParams{
		Slug: "svc", Kind: "service", Name: "Svc",
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	// Doc link round-trip.
	if err := arch.LinkDoc(ctx, tc.pool, tc.projectID, "svc", "projects/x/readme.md"); err != nil {
		t.Fatalf("LinkDoc: %v", err)
	}
	// LinkDoc on missing slug must error.
	if err := arch.LinkDoc(ctx, tc.pool, tc.projectID, "ghost", "x.md"); err == nil {
		t.Error("LinkDoc to ghost node should fail")
	}
	// Empty doc path is rejected.
	if err := arch.LinkDoc(ctx, tc.pool, tc.projectID, "svc", ""); err == nil {
		t.Error("LinkDoc with empty path should fail")
	}

	// Task link round-trip.
	taskID := uuid.New()
	if err := arch.LinkTask(ctx, tc.pool, tc.projectID, tc.projectID, taskID, "svc"); err != nil {
		t.Fatalf("LinkTask: %v", err)
	}

	// ListLinks shows both.
	links, err := arch.ListLinks(ctx, tc.pool, tc.projectID, "svc")
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	hasDoc, hasTask := false, false
	for _, l := range links {
		switch l.LinkType {
		case "doc":
			if l.TargetID == "projects/x/readme.md" {
				hasDoc = true
			}
		case "task":
			if l.TargetID == taskID.String() {
				hasTask = true
			}
		}
	}
	if !hasDoc || !hasTask {
		t.Errorf("missing links: doc=%v task=%v", hasDoc, hasTask)
	}

	// Unlink both, list goes empty.
	if err := arch.UnlinkDoc(ctx, tc.pool, tc.projectID, "svc", "projects/x/readme.md"); err != nil {
		t.Fatalf("UnlinkDoc: %v", err)
	}
	if err := arch.UnlinkTask(ctx, tc.pool, tc.projectID, "svc", taskID); err != nil {
		t.Fatalf("UnlinkTask: %v", err)
	}
	left, err := arch.ListLinks(ctx, tc.pool, tc.projectID, "svc")
	if err != nil {
		t.Fatalf("ListLinks after unlink: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("expected zero links after unlink, got %d", len(left))
	}

	// Cascade: deleting the node removes the links table rows (FK
	// cascade in the schema). Re-link, delete node, verify.
	if err := arch.LinkDoc(ctx, tc.pool, tc.projectID, "svc", "y.md"); err != nil {
		t.Fatalf("relink: %v", err)
	}
	if err := arch.RemoveNode(ctx, tc.pool, tc.projectID, "svc", false); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	if _, err := arch.ListLinks(ctx, tc.pool, tc.projectID, "svc"); err == nil {
		t.Error("ListLinks on removed node should fail (node lookup)")
	}
}
