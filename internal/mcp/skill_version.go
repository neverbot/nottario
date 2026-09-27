package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/skill"
)

// defaultBundleVersionTTL bounds how stale the advertised bundle
// version can be after an admin edits a skill override. Short enough
// that the next agent to connect hears about it; long enough that a
// busy instance is not re-hashing the bundle on every MCP request.
const defaultBundleVersionTTL = 30 * time.Second

// bundleVersions caches the current skill bundle version. Computing it
// reads every override from the database, and every MCP request asks
// for it, so it is recomputed at most once per TTL.
type bundleVersions struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu      sync.Mutex
	value   string
	fetched time.Time
}

func newBundleVersions(pool *pgxpool.Pool, ttl time.Duration) *bundleVersions {
	if ttl <= 0 {
		ttl = defaultBundleVersionTTL
	}
	return &bundleVersions{pool: pool, ttl: ttl}
}

// current returns the cached version, refreshing it when stale. On a
// refresh failure it keeps serving the last known value: a transient
// database error should not stop agents from connecting.
func (b *bundleVersions) current(ctx context.Context) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.value != "" && time.Since(b.fetched) < b.ttl {
		return b.value
	}
	v, err := skill.BundleVersion(ctx, b.pool)
	if err != nil {
		return b.value
	}
	b.value, b.fetched = v, time.Now()
	return v
}

// skillInstructions is what every agent is told when it connects.
// It lives in the server, not in the skill, on purpose: an agent
// running an outdated bundle reads outdated instructions, and a bundle
// that predates this check would never learn to run it. The server is
// the one party that always knows the current version.
func skillInstructions(bundleVersion string) string {
	return fmt.Sprintf(`Nottario coordinates tasks, documents and architecture for this team; the operating rules for agents ship as a skill bundle.

Skill bundle version: %s

Check it before relying on your installed skills: hash the SHA256SUMS file in the directory where the nottario skill is installed (sha256sum <dir>/SHA256SUMS, or shasum -a 256) and compare "sha256:<hex>" with the version above. If it differs, or the file does not exist, call nottario.skill.install to reinstall, then tell the human to restart this client: skills are loaded when a session starts, so the new ones only apply after a restart. If they match, carry on; there is nothing to do.`, bundleVersion)
}

// versionedServers keeps one MCP server per bundle version. The
// instructions are fixed when a server is built, so a new version
// needs a new server; everything else is identical, so the previous
// one is simply replaced rather than rebuilt on every request.
type versionedServers struct {
	build func(instructions string) *sdk.Server

	mu      sync.Mutex
	version string
	server  *sdk.Server
}

func (v *versionedServers) forVersion(version string) *sdk.Server {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.server == nil || v.version != version {
		v.server = v.build(skillInstructions(version))
		v.version = version
	}
	return v.server
}
