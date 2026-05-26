package mcp

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/version"
)

// Deps wires the dependencies the MCP server needs.
type Deps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
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
func Handler(d Deps) http.Handler {
	server := buildServer(d)
	streamable := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return server
	}, &sdk.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := resolveCaller(r, d.Resolver)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nottario"`)
			http.Error(w, `{"error":"missing or invalid Authorization token"}`, http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r.WithContext(withCaller(r.Context(), c)))
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
func buildServer(d Deps) *sdk.Server {
	server := sdk.NewServer(
		&sdk.Implementation{Name: "nottario", Version: version.Version},
		nil,
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
