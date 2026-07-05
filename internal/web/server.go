package web

import (
	"io/fs"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/identity"
	mcpserver "github.com/neverbot/nottario/internal/mcp"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/selfupdate"
)

// Deps wires the http server with its collaborators.
type Deps struct {
	Pool        *pgxpool.Pool
	Resolver    *identity.Resolver
	OAuthConfig identity.OAuthConfig
	Hub         *realtime.Hub
	// SelfUpdateState is nil when SELF_UPDATE_CHECK_ENABLED=false;
	// the /api/version/status endpoint reports check_enabled=false
	// in that case. SelfUpdateUpstream is echoed back regardless so
	// operators can confirm the pointer.
	SelfUpdateState    *selfupdate.State
	SelfUpdateUpstream string
	// Notifier is nil-safe; a nil Notifier means notifications are
	// off and every OnXxx call is a no-op. NotificationsEnabled
	// is echoed in the /api/notifications/unread_count response so
	// the frontend can hide the bell when the feature is disabled.
	Notifier             *notifications.Notifier
	NotificationsEnabled bool
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
	mux.Handle("GET /api/version/status", VersionStatusHandler(VersionStatusDeps{
		Resolver: d.Resolver,
		State:    d.SelfUpdateState,
		Upstream: d.SelfUpdateUpstream,
	}))
	// SPA catch-all: any GET that does not match a more specific route
	// is served the embedded index.html so the client-side router can
	// resolve the path (handles direct page loads and refreshes for
	// /projects/<id>/board, /projects/<id>/settings, etc.). Unknown /api/* and /auth/*
	// paths short-circuit to 404 inside the handler.
	mux.Handle("GET /", IndexHandler())

	auth := AuthDeps{Pool: d.Pool, Resolver: d.Resolver, OAuthConfig: d.OAuthConfig}
	mux.Handle("GET /auth/github/start", GithubStartHandler(auth))
	mux.Handle("GET /auth/github/callback", GithubCallbackHandler(auth))
	mux.Handle("POST /auth/logout", LogoutHandler(auth))
	mux.Handle("GET /auth/logout", LogoutHandler(auth)) // convenience for the UI
	mux.Handle("GET /api/me", MeHandler(auth))
	mux.Handle("GET /api/me/tokens", MeTokensHandler(auth))
	mux.Handle("GET /api/users", ListUsersHandler(UsersDeps{Pool: d.Pool, Resolver: d.Resolver}))

	notif := NotificationsDeps{Pool: d.Pool, Resolver: d.Resolver, Enabled: d.NotificationsEnabled}
	mux.Handle("GET /api/notifications", ListNotificationsHandler(notif))
	mux.Handle("GET /api/notifications/unread_count", UnreadCountHandler(notif))
	mux.Handle("POST /api/notifications/read", MarkReadHandler(notif))
	mux.Handle("POST /api/notifications/read_all", MarkAllReadHandler(notif))
	mux.Handle("GET /api/me/notification_preferences", GetPreferencesHandler(notif))
	mux.Handle("PATCH /api/me/notification_preferences", PatchPreferencesHandler(notif))

	proj := ProjectDeps{Pool: d.Pool, Resolver: d.Resolver}
	guard := func(h http.Handler) http.Handler { return withProjectScopeGuard(d.Resolver, h) }
	mux.Handle("GET /api/projects", ListProjectsHandler(proj))
	mux.Handle("POST /api/projects", CreateProjectHandler(proj))
	mux.Handle("GET /api/projects/{id}", guard(GetProjectHandler(proj)))
	mux.Handle("PATCH /api/projects/{id}", guard(UpdateProjectHandler(proj)))
	mux.Handle("PATCH /api/projects/{id}/mcp", guard(UpdateProjectMCPHandler(proj)))
	mux.Handle("PATCH /api/projects/{id}/default_view", guard(UpdateProjectDefaultViewHandler(proj)))
	mux.Handle("PATCH /api/projects/{id}/owner", guard(SetOwnerHandler(proj)))
	mux.Handle("DELETE /api/projects/{id}", guard(DeleteProjectHandler(proj)))

	mux.Handle("GET /api/projects/{id}/cycles", guard(ListCyclesHandler(proj)))
	mux.Handle("GET /api/projects/{id}/cycles/current", guard(GetCurrentCycleHandler(proj)))
	mux.Handle("POST /api/projects/{id}/cycles/end", guard(EndCycleHandler(proj)))

	mux.Handle("GET /api/projects/{id}/roles", guard(ListRolesHandler(proj)))
	mux.Handle("POST /api/projects/{id}/roles", guard(CreateRoleHandler(proj)))
	mux.Handle("PATCH /api/projects/{id}/roles/{role_id}", guard(UpdateRoleHandler(proj)))
	mux.Handle("DELETE /api/projects/{id}/roles/{role_id}", guard(DeleteRoleHandler(proj)))
	mux.Handle("POST /api/projects/{id}/roles/reorder", guard(ReorderRolesHandler(proj)))

	mux.Handle("GET /api/projects/{id}/priorities", guard(ListPrioritiesHandler(proj)))
	mux.Handle("POST /api/projects/{id}/priorities", guard(UpsertPriorityHandler(proj)))
	mux.Handle("DELETE /api/projects/{id}/priorities/{key}", guard(RemovePriorityHandler(proj)))

	mux.Handle("GET /api/projects/{id}/members", guard(ListMembersHandler(proj)))
	mux.Handle("POST /api/projects/{id}/members", guard(AddMemberHandler(proj)))
	mux.Handle("DELETE /api/projects/{id}/members/{user_id}/{role_id}", guard(RemoveMemberHandler(proj)))

	tok := TokenDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects/{project_id}/tokens", guard(ListProjectTokensHandler(tok)))
	mux.Handle("POST /api/projects/{project_id}/tokens", guard(IssueProjectTokenHandler(tok)))
	mux.Handle("DELETE /api/projects/{project_id}/tokens/{token_id}", guard(RevokeProjectTokenHandler(tok)))

	// Skill bundle served unauthenticated so agents can preview the
	// catalogue before authenticating. Per-organisation overrides
	// resolved against the documents table inside the handler.
	mux.Handle("GET /skill", SkillHandler(d.Pool))
	mux.Handle("GET /skill/", SkillHandler(d.Pool))
	var sessionKey []byte
	if d.Resolver != nil {
		sessionKey = d.Resolver.SessionKey
	}
	mux.Handle("GET /skill.zip", SkillZipHandler(d.Pool, sessionKey))

	// Real-time event stream for the web UI (and any future SSE client).
	if d.Hub != nil {
		mux.Handle("GET /events", realtime.SSEHandler(d.Hub, d.Pool, d.Resolver))
	}

	// MCP endpoint — Streamable HTTP transport with Bearer-token auth.
	// Methods are enumerated explicitly so this route does not conflict
	// with the SPA catch-all on GET /.
	mcpHandler := mcpserver.Handler(mcpserver.Deps{Pool: d.Pool, Resolver: d.Resolver, SessionKey: sessionKey})
	for _, method := range []string{"GET", "POST", "DELETE", "OPTIONS"} {
		mux.Handle(method+" /mcp", mcpHandler)
		mux.Handle(method+" /mcp/", mcpHandler)
	}

	archDeps := ArchDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/projects/{id}/arch/kinds", guard(ListKindsHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/kinds", guard(UpsertKindHandler(archDeps)))
	mux.Handle("DELETE /api/projects/{id}/arch/kinds/{key}", guard(DeleteKindHandler(archDeps)))
	mux.Handle("GET /api/projects/{id}/arch/nodes", guard(ListNodesHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/nodes", guard(UpsertNodeHandler(archDeps)))
	mux.Handle("GET /api/projects/{id}/arch/nodes/{slug}", guard(GetNodeHandler(archDeps)))
	mux.Handle("DELETE /api/projects/{id}/arch/nodes/{slug}", guard(RemoveNodeHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/move", guard(MoveNodeHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/links", guard(LinkNodeHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/nodes/{slug}/unlinks", guard(UnlinkNodeHandler(archDeps)))
	mux.Handle("GET /api/projects/{id}/arch/edges", guard(ListEdgesHandler(archDeps)))
	mux.Handle("POST /api/projects/{id}/arch/edges", guard(UpsertEdgeHandler(archDeps)))
	mux.Handle("DELETE /api/projects/{id}/arch/edges/{edge_id}", guard(RemoveEdgeHandler(archDeps)))
	mux.Handle("GET /api/projects/{id}/arch/history", guard(ListArchHistoryHandler(archDeps)))
	mux.Handle("GET /api/projects/{id}/arch/revisions/{version}", guard(GetArchRevisionHandler(archDeps)))

	docsDeps := DocsDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/docs", ListDocsHandler(docsDeps))
	mux.Handle("GET /api/docs/read", ReadDocHandler(docsDeps))
	mux.Handle("POST /api/docs/write", WriteDocHandler(docsDeps))
	mux.Handle("POST /api/docs/delete", DeleteDocHandler(docsDeps))
	mux.Handle("GET /api/docs/search", SearchDocsHandler(docsDeps))
	mux.Handle("GET /api/docs/history", HistoryDocHandler(docsDeps))
	mux.Handle("GET /api/docs/read-version", ReadDocVersionHandler(docsDeps))

	mdDeps := MarkdownDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("POST /api/markdown/render", RenderMarkdownHandler(mdDeps))

	searchDeps := SearchDeps{Pool: d.Pool, Resolver: d.Resolver}
	mux.Handle("GET /api/search", SearchHandler(searchDeps))

	tasks := TaskDeps{Pool: d.Pool, Resolver: d.Resolver, Notifier: d.Notifier}
	mux.Handle("GET /api/projects/{id}/tasks", guard(ListTasksHandler(tasks)))
	mux.Handle("POST /api/projects/{id}/tasks", guard(CreateTaskHandler(tasks)))
	mux.Handle("GET /api/projects/{id}/tasks/next", guard(NextTaskHandler(tasks)))
	mux.Handle("GET /api/projects/{id}/tasks/dependencies", guard(ListDependenciesHandler(tasks)))
	mux.Handle("GET /api/projects/{id}/tasks/inconsistencies", guard(ListInconsistenciesHandler(tasks)))
	mux.Handle("GET /api/projects/{id}/tasks/{task_id}", guard(GetTaskHandler(tasks)))
	mux.Handle("PATCH /api/projects/{id}/tasks/{task_id}", guard(UpdateTaskHandler(tasks)))
	mux.Handle("DELETE /api/projects/{id}/tasks/{task_id}", guard(DeleteTaskHandler(tasks)))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/state", guard(SetTaskStateHandler(tasks)))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/dependencies", guard(AddDependencyHandler(tasks)))
	mux.Handle("DELETE /api/projects/{id}/tasks/{task_id}/dependencies/{dep_id}", guard(RemoveDependencyHandler(tasks)))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/commits", guard(LinkCommitHandler(tasks)))
	mux.Handle("POST /api/projects/{id}/tasks/{task_id}/comments", guard(AddCommentHandler(tasks)))
	mux.Handle("PATCH /api/projects/{id}/tasks/{task_id}/text", guard(EditTaskTextHandler(tasks)))
	mux.Handle("PATCH /api/projects/{id}/tasks/{task_id}/comments/{comment_id}", guard(EditCommentHandler(tasks)))
	mux.Handle("DELETE /api/projects/{id}/tasks/{task_id}/comments/{comment_id}", guard(DeleteCommentHandler(tasks)))

	return mux
}
