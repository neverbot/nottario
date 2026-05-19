package web

import (
	"io/fs"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
	mcpserver "github.com/neverbot/nottario/internal/mcp"
)

// Deps wires the http server with its collaborators.
type Deps struct {
	Pool        *pgxpool.Pool
	Resolver    *identity.Resolver
	OAuthConfig identity.OAuthConfig
}

// NewServer returns an http.Handler wiring all M1 routes.
func NewServer(d Deps) http.Handler {
	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("nottario: cannot derive static sub-fs: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.Handle("GET /healthz", HealthzHandler())
	mux.Handle("GET /version", VersionHandler())
	mux.Handle("GET /{$}", IndexHandler())

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

	mux.Handle("GET /api/projects/{id}/members", ListMembersHandler(proj))
	mux.Handle("POST /api/projects/{id}/members", AddMemberHandler(proj))
	mux.Handle("DELETE /api/projects/{id}/members/{user_id}/{role_id}", RemoveMemberHandler(proj))

	tok := TokenDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/tokens", ListTokensHandler(tok))
	mux.Handle("POST /api/tokens", IssueTokenHandler(tok))
	mux.Handle("DELETE /api/tokens/{id}", RevokeTokenHandler(tok))

	// Skill bundle served unauthenticated so agents can preview the
	// catalogue before authenticating.
	mux.Handle("GET /skill", SkillHandler())
	mux.Handle("GET /skill/", SkillHandler())

	// MCP endpoint — Streamable HTTP transport with Bearer-token auth.
	mcpHandler := mcpserver.Handler(mcpserver.Deps{Pool: d.Pool, Resolver: d.Resolver})
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	tasks := TaskDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects/{id}/tasks", ListTasksHandler(tasks))
	mux.Handle("POST /api/projects/{id}/tasks", CreateTaskHandler(tasks))
	mux.Handle("GET /api/projects/{id}/tasks/next", NextTaskHandler(tasks))
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
