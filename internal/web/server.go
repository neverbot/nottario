package web

import (
	"io/fs"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
	mcpserver "github.com/neverbot/nottario/internal/mcp"
	"github.com/neverbot/nottario/internal/realtime"
)

// Deps wires the http server with its collaborators.
type Deps struct {
	Pool        *pgxpool.Pool
	Resolver    *identity.Resolver
	OAuthConfig identity.OAuthConfig
	Hub         *realtime.Hub
}

// NewServer returns an http.Handler wiring all M1 routes.
func NewServer(d Deps) http.Handler {
	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("nottario: cannot derive static sub-fs: " + err.Error())
	}
	// Wrap the static file server so every response forces the browser
	// to revalidate before reusing a cached copy. Without this header
	// browsers happily serve stale JS/CSS even after a binary rebuild,
	// which surfaced repeatedly during dogfooding as "I rebuilt but the
	// page hasn't changed". The assets ship inside the binary anyway,
	// so the bandwidth cost of revalidation is trivial.
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticSub)))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		staticHandler.ServeHTTP(w, r)
	}))

	mux.Handle("GET /healthz", HealthzHandler())
	mux.Handle("GET /version", VersionHandler())
	// SPA catch-all: any GET that does not match a more specific route
	// is served the embedded index.html so the client-side router can
	// resolve the path (handles direct page loads and refreshes for
	// /projects/<id>/board, /tokens, etc.). Unknown /api/* and /auth/*
	// paths short-circuit to 404 inside the handler.
	mux.Handle("GET /", IndexHandler())

	auth := AuthDeps{Pool: d.Pool, Resolver: d.Resolver, OAuthConfig: d.OAuthConfig}
	mux.Handle("GET /auth/github/start", GithubStartHandler(auth))
	mux.Handle("GET /auth/github/callback", GithubCallbackHandler(auth))
	mux.Handle("POST /auth/logout", LogoutHandler(auth))
	mux.Handle("GET /auth/logout", LogoutHandler(auth)) // convenience for the UI
	mux.Handle("GET /api/me", MeHandler(auth))

	proj := ProjectDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects", ListProjectsHandler(proj))
	mux.Handle("POST /api/projects", CreateProjectHandler(proj))
	mux.Handle("GET /api/projects/{id}", GetProjectHandler(proj))
	mux.Handle("PATCH /api/projects/{id}", UpdateProjectHandler(proj))
	mux.Handle("DELETE /api/projects/{id}", DeleteProjectHandler(proj))

	mux.Handle("GET /api/projects/{id}/roles", ListRolesHandler(proj))
	mux.Handle("POST /api/projects/{id}/roles", CreateRoleHandler(proj))
	mux.Handle("PATCH /api/projects/{id}/roles/{role_id}", UpdateRoleHandler(proj))
	mux.Handle("DELETE /api/projects/{id}/roles/{role_id}", DeleteRoleHandler(proj))

	mux.Handle("GET /api/projects/{id}/priorities", ListPrioritiesHandler(proj))
	mux.Handle("POST /api/projects/{id}/priorities", UpsertPriorityHandler(proj))
	mux.Handle("DELETE /api/projects/{id}/priorities/{key}", RemovePriorityHandler(proj))

	mux.Handle("GET /api/projects/{id}/members", ListMembersHandler(proj))
	mux.Handle("POST /api/projects/{id}/members", AddMemberHandler(proj))
	mux.Handle("DELETE /api/projects/{id}/members/{user_id}/{role_id}", RemoveMemberHandler(proj))

	tok := TokenDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/tokens", ListTokensHandler(tok))
	mux.Handle("POST /api/tokens", IssueTokenHandler(tok))
	mux.Handle("DELETE /api/tokens/{id}", RevokeTokenHandler(tok))

	// Skill bundle served unauthenticated so agents can preview the
	// catalogue before authenticating. Per-organisation overrides
	// resolved against the documents table inside the handler.
	mux.Handle("GET /skill", SkillHandler(d.Pool))
	mux.Handle("GET /skill/", SkillHandler(d.Pool))
	mux.Handle("GET /skill.zip", SkillZipHandler(d.Pool))

	// Real-time event stream for the web UI (and any future SSE client).
	if d.Hub != nil {
		mux.Handle("GET /events", realtime.SSEHandler(d.Hub, d.Pool, d.Resolver))
	}

	// MCP endpoint — Streamable HTTP transport with Bearer-token auth.
	// Methods are enumerated explicitly so this route does not conflict
	// with the SPA catch-all on GET /.
	mcpHandler := mcpserver.Handler(mcpserver.Deps{Pool: d.Pool, Resolver: d.Resolver})
	for _, method := range []string{"GET", "POST", "DELETE", "OPTIONS"} {
		mux.Handle(method+" /mcp", mcpHandler)
		mux.Handle(method+" /mcp/", mcpHandler)
	}

	archDeps := ArchDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects/{id}/arch/kinds", ListKindsHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/kinds", UpsertKindHandler(archDeps))
	mux.Handle("DELETE /api/projects/{id}/arch/kinds/{key}", DeleteKindHandler(archDeps))
	mux.Handle("GET /api/projects/{id}/arch/nodes", ListNodesHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/nodes", UpsertNodeHandler(archDeps))
	mux.Handle("GET /api/projects/{id}/arch/nodes/{slug}", GetNodeHandler(archDeps))
	mux.Handle("DELETE /api/projects/{id}/arch/nodes/{slug}", RemoveNodeHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/move", MoveNodeHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/links", LinkNodeHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/unlinks", UnlinkNodeHandler(archDeps))
	mux.Handle("GET /api/projects/{id}/arch/edges", ListEdgesHandler(archDeps))
	mux.Handle("POST /api/projects/{id}/arch/edges", UpsertEdgeHandler(archDeps))
	mux.Handle("DELETE /api/projects/{id}/arch/edges/{edge_id}", RemoveEdgeHandler(archDeps))

	docsDeps := DocsDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/docs", ListDocsHandler(docsDeps))
	mux.Handle("GET /api/docs/read", ReadDocHandler(docsDeps))
	mux.Handle("POST /api/docs/write", WriteDocHandler(docsDeps))
	mux.Handle("POST /api/docs/delete", DeleteDocHandler(docsDeps))
	mux.Handle("GET /api/docs/search", SearchDocsHandler(docsDeps))
	mux.Handle("GET /api/docs/history", HistoryDocHandler(docsDeps))

	searchDeps := SearchDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/search", SearchHandler(searchDeps))

	tasks := TaskDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects/{id}/tasks", ListTasksHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks", CreateTaskHandler(tasks))
	mux.Handle("GET /api/projects/{id}/tasks/next", NextTaskHandler(tasks))
	mux.Handle("GET /api/projects/{id}/tasks/dependencies", ListDependenciesHandler(tasks))
	mux.Handle("GET /api/projects/{id}/tasks/{task_id}", GetTaskHandler(tasks))
	mux.Handle("PATCH /api/projects/{id}/tasks/{task_id}", UpdateTaskHandler(tasks))
	mux.Handle("DELETE /api/projects/{id}/tasks/{task_id}", DeleteTaskHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/state", SetTaskStateHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/dependencies", AddDependencyHandler(tasks))
	mux.Handle("DELETE /api/projects/{id}/tasks/{task_id}/dependencies/{dep_id}", RemoveDependencyHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/commits", LinkCommitHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/comments", AddCommentHandler(tasks))

	return mux
}
