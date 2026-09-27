package mcp

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/skill"
	"github.com/neverbot/nottario/internal/testutil"
)

// An admin can change a skill override without redeploying, and the
// version agents are told about has to follow within the TTL, not
// stay frozen at whatever it was when the binary started.
func TestBundleVersions_FollowsOverrideEdits(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	versions := newBundleVersions(pool, 10*time.Millisecond)

	before := versions.current(ctx)
	if !strings.HasPrefix(before, "sha256:") {
		t.Fatalf("version = %q", before)
	}
	if again := versions.current(ctx); again != before {
		t.Errorf("an unchanged bundle reported two versions: %s then %s", before, again)
	}

	if _, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeGlobal, Kind: docs.KindSkill,
		Path: "global/skills/extra.md", ContentMD: "# Extra\n\nA rule added by an admin.\n",
	}, docs.Authorship{}); err != nil {
		t.Fatalf("write override: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	after := versions.current(ctx)
	if after == before {
		t.Error("an override edit did not change the advertised version")
	}
	if want, _ := skill.BundleVersion(ctx, pool); after != want {
		t.Errorf("advertised %s, the bundle is actually %s", after, want)
	}
}

// Within the TTL the cached value is served: the whole point is not
// to re-hash the bundle on every MCP request.
func TestBundleVersions_CachesWithinTTL(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	versions := newBundleVersions(pool, time.Hour)

	before := versions.current(ctx)
	if _, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeGlobal, Kind: docs.KindSkill,
		Path: "global/skills/extra.md", ContentMD: "# Extra\n",
	}, docs.Authorship{}); err != nil {
		t.Fatalf("write override: %v", err)
	}
	if got := versions.current(ctx); got != before {
		t.Errorf("the cache was bypassed inside its TTL: %s then %s", before, got)
	}
}

// A new version needs a new server, because instructions are fixed at
// construction; the same version must reuse the one already built.
func TestVersionedServers_RebuildOnlyOnChange(t *testing.T) {
	builds := 0
	var last string
	servers := &versionedServers{build: func(instructions string) *sdk.Server {
		builds++
		last = instructions
		return newTestServer()
	}}

	a := servers.forVersion("sha256:aaa")
	if b := servers.forVersion("sha256:aaa"); b != a || builds != 1 {
		t.Errorf("same version rebuilt the server (%d builds)", builds)
	}
	if !strings.Contains(last, "sha256:aaa") {
		t.Errorf("instructions do not carry the version: %s", last)
	}
	if c := servers.forVersion("sha256:bbb"); c == a || builds != 2 {
		t.Errorf("a new version did not get a new server (%d builds)", builds)
	}
	if !strings.Contains(last, "sha256:bbb") {
		t.Errorf("the rebuilt server still announces the old version: %s", last)
	}
}

func newTestServer() *sdk.Server {
	return sdk.NewServer(&sdk.Implementation{Name: "test", Version: "0"}, nil)
}
