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
func Handler(d Deps) http.Handler {
	server := buildServer(d)
	streamable := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return server
	}, nil)

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
	return server
}
