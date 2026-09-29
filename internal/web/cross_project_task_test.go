package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// victimState is everything a cross-project attack could change about
// a task: if any of it moves, the attack got through.
func victimState(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	task, err := tasks.Get(t.Context(), pool, id)
	if err != nil {
		return "gone"
	}
	comments, _ := tasks.ListComments(t.Context(), pool, id)
	commits, _ := tasks.ListCommits(t.Context(), pool, id)
	deps, _ := tasks.ListDependenciesOf(t.Context(), pool, id)
	var b strings.Builder
	b.WriteString(string(task.State) + "|" + task.Title + "|" + task.DescriptionMD)
	if task.AssigneeUserID != nil {
		b.WriteString("|a=" + task.AssigneeUserID.String())
	}
	b.WriteString("|c=" + strconv.Itoa(len(comments)) + "|k=" + strconv.Itoa(len(commits)) + "|d=" + strconv.Itoa(len(deps)))
	return b.String()
}

func readServerGo() (string, error) {
	b, err := os.ReadFile("server.go")
	return string(b), err
}

type crossFixture struct {
	pool       *pgxpool.Pool
	ts         *httptest.Server
	auth       string
	mine       identity.Project
	ownTask    *tasks.Task
	victim     *tasks.Task
	victimDeps *tasks.Task
	strangerID uuid.UUID
	meID       uuid.UUID
}

// The caller owns one project and belongs to nothing else; the victim
// task lives in a project the caller has never been part of.
func setupCross(t *testing.T) *crossFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	if _, _, err := identity.UpsertFromGithub(ctx, pool, 14501, "cross-admin", "Admin", ""); err != nil {
		t.Fatalf("UpsertFromGithub admin: %v", err)
	}
	me, _, _ := identity.UpsertFromGithub(ctx, pool, 14502, "cross-me", "Me", "")
	stranger, _, _ := identity.UpsertFromGithub(ctx, pool, 14503, "cross-stranger", "Stranger", "")
	mine, _ := identity.CreateProject(ctx, pool, "Cross Mine", "", "", "", me.ID)
	theirs, _ := identity.CreateProject(ctx, pool, "Cross Theirs", "", "", "", stranger.ID)
	own, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: mine.ID, Type: tasks.TypeFeature, Title: "mine"},
		tasks.Authorship{UserID: &me.ID})
	if err != nil {
		t.Fatalf("own task: %v", err)
	}
	sby := tasks.Authorship{UserID: &stranger.ID}
	victim, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: theirs.ID, Type: tasks.TypeTask, Title: "victim", DescriptionMD: "untouched"}, sby)
	if err != nil {
		t.Fatalf("victim: %v", err)
	}
	pre, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: theirs.ID, Type: tasks.TypeTask, Title: "victim's precondition"}, sby)
	if err := tasks.AddDependency(ctx, pool, victim.ID, pre.ID); err != nil {
		t.Fatalf("victim dependency: %v", err)
	}
	if err := tasks.LinkCommit(ctx, pool, victim.ID, "them/repo", "abc1234", "theirs", sby); err != nil {
		t.Fatalf("victim commit: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, me.ID, mine.ID, "cross", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	return &crossFixture{pool: pool, ts: ts, auth: "Bearer " + token, mine: *mine, ownTask: own, victim: victim, victimDeps: pre, strangerID: stranger.ID, meID: me.ID}
}

var taskRouteDecl = regexp.MustCompile(`mux\.Handle\("([A-Z]+) (/api/projects/\{id\}/tasks/\{task_id\}[^"]*)"`)

// Every web route that acts on a task in the path must refuse a task
// that is not in the project in the path — here, the caller's own
// project next to someone else's task.
func TestCrossProject_EveryTaskRouteRefusesAForeignTask(t *testing.T) {
	f := setupCross(t)
	src, err := readServerGo()
	if err != nil {
		t.Fatal(err)
	}
	routes := taskRouteDecl.FindAllStringSubmatch(src, -1)
	if len(routes) < 8 {
		t.Fatalf("found only %d task routes in server.go", len(routes))
	}
	before := victimState(t, f.pool, f.victim.ID)

	for _, m := range routes {
		method, path := m[1], m[2]
		url := f.ts.URL + strings.NewReplacer(
			"{id}", f.mine.ID.String(),
			"{task_id}", f.victim.ID.String(),
			"{dep_id}", f.victimDeps.ID.String(),
			"{comment_id}", uuid.NewString(),
		).Replace(path)
		body := `{"state":"done","title":"hijacked","description":"hijacked","body":"hijacked",` +
			`"repo":"them/repo","sha":"abc1234","depends_on_id":"` + f.victimDeps.ID.String() + `"}`
		r := doRaw(t, method, url, f.auth, []byte(body))
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s with a foreign task: %d, want 404\n%s", method, path, r.StatusCode, r.Body)
		}
	}
	if after := victimState(t, f.pool, f.victim.ID); after != before {
		t.Errorf("the foreign task changed:\n before %s\n after  %s", before, after)
	}

	// Ids carried in the body, not the path.
	r := doRaw(t, "POST", f.ts.URL+"/api/projects/"+f.mine.ID.String()+"/tasks", f.auth,
		mustJSON(map[string]any{"title": "child", "type": "task", "parent_task_id": f.victim.ID.String()}))
	if r.StatusCode >= 200 && r.StatusCode < 300 {
		t.Errorf("created a task under another project's feature: %s", r.Body)
	}
	r = doRaw(t, "POST", f.ts.URL+"/api/projects/"+f.mine.ID.String()+"/tasks/"+f.ownTask.ID.String()+"/dependencies", f.auth,
		mustJSON(map[string]any{"depends_on_id": f.victim.ID.String()}))
	if r.StatusCode >= 200 && r.StatusCode < 300 {
		t.Errorf("made an own task depend on a foreign one: %s", r.Body)
	}
}

// The same for every MCP tool that takes a task: the caller's project
// plus someone else's task must be refused, and change nothing.
func TestCrossProject_EveryMCPTaskToolRefusesAForeignTask(t *testing.T) {
	f := newMCPFixture(t, 14510, "cross-mcp")
	pool := f.pool.(*pgxpool.Pool)
	ctx := t.Context()
	stranger, _, _ := identity.UpsertFromGithub(ctx, pool, 14511, "cross-mcp-stranger", "Stranger", "")
	theirs, _ := identity.CreateProject(ctx, pool, "Cross MCP Theirs", "", "", "", stranger.ID)
	sby := tasks.Authorship{UserID: &stranger.ID}
	victim, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: theirs.ID, Type: tasks.TypeTask, Title: "victim"}, sby)
	pre, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: theirs.ID, Type: tasks.TypeTask, Title: "pre"}, sby)
	_ = tasks.AddDependency(ctx, pool, victim.ID, pre.ID)
	_ = tasks.LinkCommit(ctx, pool, victim.ID, "them/repo", "abc1234", "theirs", sby)
	before := victimState(t, pool, victim.ID)

	vid := victim.ID.String()
	calls := []struct {
		tool string
		args map[string]any
	}{
		{"nottario.tasks.get", map[string]any{"task_id": vid}},
		{"nottario.tasks.update", map[string]any{"task_id": vid, "title": "hijacked"}},
		{"nottario.tasks.set_state", map[string]any{"task_id": vid, "state": "done"}},
		{"nottario.tasks.close", map[string]any{"task_id": vid, "comment": "hijacked"}},
		{"nottario.tasks.claim", map[string]any{"task_id": vid}},
		{"nottario.tasks.add_comment", map[string]any{"task_id": vid, "body": "hijacked"}},
		{"nottario.tasks.link_commit", map[string]any{"task_id": vid, "repo": "x/y", "sha": "deadbeef"}},
		{"nottario.tasks.unlink_commit", map[string]any{"task_id": vid, "repo": "them/repo", "sha": "abc1234"}},
		{"nottario.tasks.add_dependency", map[string]any{"task_id": vid, "depends_on_id": pre.ID.String()}},
		{"nottario.tasks.remove_dependency", map[string]any{"task_id": vid, "depends_on_id": pre.ID.String()}},
	}
	for _, c := range calls {
		c.args["project_id"] = f.projectID
		if msg := f.callExpectErr(t, c.tool, c.args); msg == "" {
			t.Errorf("%s accepted another project's task", c.tool)
		}
	}
	if after := victimState(t, pool, victim.ID); after != before {
		t.Errorf("the foreign task changed:\n before %s\n after  %s", before, after)
	}

	// Ids that arrive as a second argument.
	var own map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "own"}, &own)
	if msg := f.callExpectErr(t, "nottario.tasks.add_dependency", map[string]any{
		"project_id": f.projectID, "task_id": own["id"], "depends_on_id": vid,
	}); msg == "" {
		t.Error("made an own task depend on a foreign one")
	}
	if msg := f.callExpectErr(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "child", "parent_task_id": vid,
	}); msg == "" {
		t.Error("created a task under another project's feature")
	}
}

// An architecture node links tasks of its own project only.
func TestCrossProject_ArchLinkRefusesAForeignTask(t *testing.T) {
	f := setupCross(t)
	ctx := t.Context()
	by := arch.Authorship{UserID: f.meID}
	if _, err := arch.UpsertKind(ctx, f.pool, f.mine.ID, by, arch.Kind{Key: "svc", Label: "Svc"}); err != nil {
		t.Fatalf("arch setup: %v", err)
	}
	if _, err := arch.UpsertNode(ctx, f.pool, f.mine.ID, by, arch.UpsertParams{Slug: "api", Kind: "svc", Name: "API"}); err != nil {
		t.Fatalf("arch setup: %v", err)
	}
	if err := arch.LinkTask(ctx, f.pool, f.mine.ID, by, f.victim.ID, "api"); err == nil {
		t.Error("linked a node to another project's task")
	}
	if err := arch.LinkTask(ctx, f.pool, f.mine.ID, by, f.ownTask.ID, "api"); err != nil {
		t.Errorf("linking an own task failed: %v", err)
	}
}
