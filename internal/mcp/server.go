package mcp

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/version"
)

// Deps wires the dependencies the MCP server needs.
type Deps struct {
	Pool       *pgxpool.Pool
	Resolver   *identity.Resolver
	SessionKey []byte // for signing short-lived /skill.zip download URLs
	// BundleVersionTTL bounds how long the advertised skill bundle
	// version may lag an override edit. Zero means the default.
	BundleVersionTTL time.Duration

	bundles *bundleVersions
}

// Handler returns an http.Handler that authenticates the incoming
// MCP request and dispatches it to a streamable-http MCP server.
//
// The MCP server itself is built once and shared across requests; the
// per-request Caller is propagated to tool handlers via the request
// context.
//
// Stateless: true makes the SDK skip Mcp-Session-Id validation and
// treat every request as a fresh session with default initialization
// parameters. This matters because container rebuilds (very frequent
// during development of Nottario itself) would otherwise invalidate
// the client's session ID and force a manual /mcp reconnect. The
// MCP tools are all stateless request/response (no per-session
// active project, no server->client requests, no cross-request
// streaming state — the Caller is re-resolved from the Bearer token
// per request and project access is checked from the DB each time),
// so we lose nothing the tools currently use. Server->client
// notifications inside a single request's lifetime still work per
// the SDK's documentation.
//
// The server carries instructions naming the current skill bundle
// version, so every agent learns at connection time whether its
// installed skills are out of date. One server is kept per version and
// replaced when the version changes (see versionedServers).
func Handler(d Deps) http.Handler {
	d.bundles = newBundleVersions(d.Pool, d.BundleVersionTTL)
	servers := &versionedServers{build: func(instructions string) *sdk.Server {
		return buildServer(d, instructions)
	}}
	streamable := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return servers.forVersion(d.bundles.current(r.Context()))
	}, &sdk.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := resolveCaller(r, d.Resolver)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nottario"`)
			http.Error(w, `{"error":"missing or invalid Authorization token"}`, http.StatusUnauthorized)
			return
		}
		ctx := withCaller(r.Context(), c)
		ctx = withExternalBaseURL(ctx, externalBaseURLFromRequest(r))
		streamable.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveCaller accepts a Bearer token. Browser cookies are not honoured
// here on purpose: this endpoint is for agents.
func resolveCaller(r *http.Request, rv *identity.Resolver) (identity.Caller, bool) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return identity.Caller{}, false
	}
	return rv.ResolveToken(r)
}

// buildServer constructs the MCP server, registering every tool.
func buildServer(d Deps, instructions string) *sdk.Server {
	server := sdk.NewServer(
		&sdk.Implementation{Name: "nottario", Version: version.Version},
		&sdk.ServerOptions{Instructions: instructions},
	)
	registerWhoami(server, d)
	registerProjects(server, d)
	registerTasks(server, d)
	registerDocs(server, d)
	registerArch(server, d)
	registerSearch(server, d)
	registerSkill(server, d)
	registerCycles(server, d)
	return server
}
