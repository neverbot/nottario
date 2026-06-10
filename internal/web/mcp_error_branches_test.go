package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// Targeted error-branch tests across the MCP families. The
// integration tests in mcp_*_integration_test.go cover happy paths;
// these exist to lift internal/mcp coverage above the 80% bar by
// exercising validation/precondition branches that the happy path
// can't reach. Reuses `newMCPFixture` / `callExpectErr`.

// ---- tasks ----

func TestMCP_Tasks_ErrorBranches(t *testing.T) {
	f := newMCPFixture(t, 14000, "errtasks")

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list bad project_id", "nottario.tasks.list",
			map[string]any{"project_id": "not-a-uuid"}},
		{"list bad cursor", "nottario.tasks.list",
			map[string]any{"project_id": f.projectID, "cursor": "!!!"}},
		{"get bad project_id", "nottario.tasks.get",
			map[string]any{"project_id": "x", "task_id": "y"}},
		{"get bad task_id", "nottario.tasks.get",
			map[string]any{"project_id": f.projectID, "task_id": "z"}},
		{"create missing project_id", "nottario.tasks.create",
			map[string]any{"title": "x"}},
		{"update missing task_id", "nottario.tasks.update",
			map[string]any{"project_id": f.projectID, "task_id": "no", "title": "x"}},
		{"set_state bad state", "nottario.tasks.set_state",
			map[string]any{"project_id": f.projectID, "task_id": "no", "state": "wat"}},
		{"claim bad task", "nottario.tasks.claim",
			map[string]any{"project_id": f.projectID, "task_id": "no"}},
		{"claim_next bad project_id", "nottario.tasks.claim_next",
			map[string]any{"project_id": "no"}},
		{"next bad project_id", "nottario.tasks.next",
			map[string]any{"project_id": "no"}},
		{"add_dependency bad uuid", "nottario.tasks.add_dependency",
			map[string]any{"project_id": f.projectID, "task_id": "no", "depends_on_id": "no"}},
		{"remove_dependency bad uuid", "nottario.tasks.remove_dependency",
			map[string]any{"project_id": f.projectID, "task_id": "no", "depends_on_id": "no"}},
		{"link_commit bad uuid", "nottario.tasks.link_commit",
			map[string]any{"project_id": "x", "task_id": "y", "repo": "a/b", "sha": "abc"}},
		{"add_comment bad uuid", "nottario.tasks.add_comment",
			map[string]any{"project_id": "x", "task_id": "y", "body": "hi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := f.callExpectErr(t, tc.tool, tc.args)
			if msg == "" {
				t.Errorf("%s: expected an error message, got empty", tc.tool)
			}
			_ = strings.ToLower(msg) // keep import in case future cases assert
		})
	}
}

// Cursor decodes successfully when round-tripped. Hits the
// pagination happy path through DecodeCursor + EncodeCursor.
func TestMCP_Tasks_ListPaginatesWithCursor(t *testing.T) {
	f := newMCPFixture(t, 14001, "paginate")
	// Create a few tasks so the page actually paginates.
	for i := 0; i < 4; i++ {
		f.callJSON(t, "nottario.tasks.create", map[string]any{
			"project_id": f.projectID,
			"title":      "t",
		}, nil)
	}
	var page1 struct {
		Tasks      []map[string]any `json:"tasks"`
		NextCursor string           `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id": f.projectID,
		"limit":      2,
	}, &page1)
	if len(page1.Tasks) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("first page: %+v", page1)
	}
	var page2 struct {
		Tasks []map[string]any `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id": f.projectID,
		"limit":      2,
		"cursor":     page1.NextCursor,
	}, &page2)
	if len(page2.Tasks) == 0 {
		t.Fatalf("second page empty: %+v", page2)
	}
}

// ---- arch ----

func TestMCP_Arch_ErrorBranches(t *testing.T) {
	f := newMCPFixture(t, 14010, "errarch")

	cases := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"upsert_node bad project_id", "nottario.arch.upsert_node",
			map[string]any{"project_id": "x", "slug": "a", "name": "A"}, "uuid"},
		{"upsert_node missing slug", "nottario.arch.upsert_node",
			map[string]any{"project_id": f.projectID, "name": "A"}, "slug"},
		{"upsert_node missing name", "nottario.arch.upsert_node",
			map[string]any{"project_id": f.projectID, "slug": "a"}, "name"},
		{"upsert_node bad parent", "nottario.arch.upsert_node",
			map[string]any{"project_id": f.projectID, "slug": "child", "name": "Child", "parent_slug": "nope"}, ""},
		{"upsert_kind missing key", "nottario.arch.upsert_kind",
			map[string]any{"project_id": f.projectID, "label": "X"}, "key"},
		{"upsert_edge missing from", "nottario.arch.upsert_edge",
			map[string]any{"project_id": f.projectID, "to_slug": "b", "kind": "uses"}, ""},
		{"get_node unknown slug", "nottario.arch.get_node",
			map[string]any{"project_id": f.projectID, "slug": "ghost"}, ""},
		{"remove_node missing", "nottario.arch.remove_node",
			map[string]any{"project_id": f.projectID, "slug": "ghost"}, ""},
		{"remove_kind unknown", "nottario.arch.remove_kind",
			map[string]any{"project_id": f.projectID, "key": "ghost"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.callExpectErr(t, tc.tool, tc.args)
		})
	}
}

// ---- docs ----

func TestMCP_Docs_ErrorBranches(t *testing.T) {
	f := newMCPFixture(t, 14020, "errdocs")

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"write empty path", "nottario.docs.write",
			map[string]any{"scope": "project", "project_id": f.projectID, "path": "", "content": "x"}},
		{"write project scope without project_id", "nottario.docs.write",
			map[string]any{"scope": "project", "path": "p.md", "content": "x"}},
		{"read missing path", "nottario.docs.read",
			map[string]any{"scope": "project", "project_id": f.projectID}},
		{"read project scope without project_id", "nottario.docs.read",
			map[string]any{"scope": "project", "path": "p.md"}},
		{"delete unknown path", "nottario.docs.delete",
			map[string]any{"scope": "project", "project_id": f.projectID, "path": "ghost.md"}},
		{"list project scope without project_id", "nottario.docs.list",
			map[string]any{"scope": "project"}},
		{"history project scope without project_id", "nottario.docs.history",
			map[string]any{"scope": "project", "path": "p.md"}},
		{"read_version missing version", "nottario.docs.read_version",
			map[string]any{"scope": "project", "project_id": f.projectID, "path": "p.md"}},
		{"write invalid scope", "nottario.docs.write",
			map[string]any{"scope": "wat", "path": "p.md", "content": "x"}},
		{"read invalid scope", "nottario.docs.read",
			map[string]any{"scope": "wat", "path": "p.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.callExpectErr(t, tc.tool, tc.args)
		})
	}
}

// ---- projects ----

func TestMCP_Projects_ErrorBranches(t *testing.T) {
	f := newMCPFixture(t, 14030, "errproj")

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get bad uuid", "nottario.projects.get",
			map[string]any{"project_id": "no"}},
		{"list_roles bad uuid", "nottario.projects.list_roles",
			map[string]any{"project_id": "no"}},
		{"list_priorities bad uuid", "nottario.projects.list_priorities",
			map[string]any{"project_id": "no"}},
		{"reorder_roles bad uuid", "nottario.projects.reorder_roles",
			map[string]any{"project_id": "no", "role_ids": []string{}}},
		{"reorder_roles incomplete list", "nottario.projects.reorder_roles",
			map[string]any{"project_id": f.projectID, "role_ids": []string{"00000000-0000-0000-0000-000000000000"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.callExpectErr(t, tc.tool, tc.args)
		})
	}
}

// ---- skill ----

func TestMCP_Skill_Unknown(t *testing.T) {
	f := newMCPFixture(t, 14040, "errskill")
	f.callExpectErr(t, "nottario.skill.read", map[string]any{"path": "no-such-file.md"})
}

// ---- search ----

func TestMCP_Search_BadProjectID(t *testing.T) {
	f := newMCPFixture(t, 14050, "errsearch")
	f.callExpectErr(t, "nottario.search", map[string]any{"project_id": "no", "query": "hi"})
}

// TestMCP_Arch_LinksAndMove exercises the link_doc / unlink_doc /
// link_task / unlink_task / move_node tools which the family test
// doesn't currently touch.
func TestMCP_Arch_LinksAndMove(t *testing.T) {
	f := newMCPFixture(t, 14060, "archlinks")

	// Seed: parent + child + a doc to link.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "system", "name": "System", "kind": "system",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "child", "name": "Child", "kind": "service",
		"parent_slug": "system",
	}, nil)
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"scope": "project", "project_id": f.projectID,
		"path": "projects/" + f.projectID + "/context/notes.md", "content": "hi",
	}, nil)

	// Link doc → unlink doc.
	f.callJSON(t, "nottario.arch.link_doc", map[string]any{
		"project_id": f.projectID, "slug": "child",
		"doc_path": "projects/" + f.projectID + "/context/notes.md",
	}, nil)
	f.callJSON(t, "nottario.arch.unlink_doc", map[string]any{
		"project_id": f.projectID, "slug": "child",
		"doc_path": "projects/" + f.projectID + "/context/notes.md",
	}, nil)

	// Link task → unlink task.
	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "linked",
	}, &task)
	taskID, _ := task["id"].(string)
	f.callJSON(t, "nottario.arch.link_task", map[string]any{
		"project_id": f.projectID, "slug": "child", "task_id": taskID,
	}, nil)
	f.callJSON(t, "nottario.arch.unlink_task", map[string]any{
		"project_id": f.projectID, "slug": "child", "task_id": taskID,
	}, nil)

	// list_edges (read-only). We don't expect any yet.
	f.callJSON(t, "nottario.arch.list_edges", map[string]any{
		"project_id": f.projectID,
	}, &struct{}{})

	// Add a sibling so we can move the child under it.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "sibling", "name": "Sibling", "kind": "service",
		"parent_slug": "system",
	}, nil)
	f.callJSON(t, "nottario.arch.move_node", map[string]any{
		"project_id": f.projectID, "slug": "child", "new_parent_slug": "sibling",
	}, nil)
}

// TestMCP_Tasks_NextWithFilters exercises claim_next / next branches
// the happy-path family test misses (role filter + already-doing
// state).
func TestMCP_Tasks_NextWithFilters(t *testing.T) {
	f := newMCPFixture(t, 14070, "nextfilt")

	// list_roles → pick one to filter by.
	var roles struct {
		Roles []map[string]any `json:"roles"`
	}
	f.callJSON(t, "nottario.projects.list_roles", map[string]any{
		"project_id": f.projectID,
	}, &roles)
	if len(roles.Roles) == 0 {
		t.Fatal("expected default roles")
	}
	roleID, _ := roles.Roles[0]["id"].(string)

	// Create a task scoped to that role.
	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id":     f.projectID,
		"title":          "for-role",
		"target_role_id": roleID,
	}, &task)

	// next with role_id filter → finds it.
	var next map[string]any
	f.callJSON(t, "nottario.tasks.next", map[string]any{
		"project_id": f.projectID, "role_id": roleID,
	}, &next)
	if t1, ok := next["task"].(map[string]any); !ok || t1["id"] != task["id"] {
		t.Errorf("expected the role-targeted task back, got %+v", next)
	}

	// claim_next with role_id → claims it.
	var claimed map[string]any
	f.callJSON(t, "nottario.tasks.claim_next", map[string]any{
		"project_id": f.projectID, "role_id": roleID,
	}, &claimed)
	tk, _ := claimed["task"].(map[string]any)
	if tk == nil || tk["state"] != "doing" {
		t.Errorf("claim_next did not move to doing: %+v", claimed)
	}
}

// TestMCP_Arch_EdgesAndUpdates covers the edges CRUD (upsert/list/
// remove) and the node-update branch of upsert_node — the family
// test only creates new nodes.
func TestMCP_Arch_EdgesAndUpdates(t *testing.T) {
	f := newMCPFixture(t, 14095, "archedges")
	// Two nodes + one edge between them.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "a", "name": "A", "kind": "system",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "b", "name": "B", "kind": "service",
	}, nil)
	// Update node A (re-upsert with new description).
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "a", "name": "A2",
		"kind": "system", "description": "updated",
	}, nil)
	// list_nodes with root_only filter.
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID, "root_only": true,
	}, &struct{}{})
	// list_nodes with parent_slug filter.
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID, "parent_slug": "a",
	}, &struct{}{})

	// Edge A → B.
	var edge map[string]any
	f.callJSON(t, "nottario.arch.upsert_edge", map[string]any{
		"project_id": f.projectID, "from_slug": "a", "to_slug": "b", "kind": "calls",
	}, &edge)
	eid, _ := edge["id"].(string)
	// list_edges with direction filter.
	f.callJSON(t, "nottario.arch.list_edges", map[string]any{
		"project_id": f.projectID, "node_slug": "a", "direction": "out",
	}, &struct{}{})
	// get_node now finds the edge.
	f.callJSON(t, "nottario.arch.get_node", map[string]any{
		"project_id": f.projectID, "slug": "a",
	}, &struct{}{})
	// remove_edge.
	if eid != "" {
		f.callJSON(t, "nottario.arch.remove_edge", map[string]any{
			"project_id": f.projectID, "edge_id": eid,
		}, nil)
	}
	// remove_node — fail without cascade if children, then with cascade.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "c", "name": "C", "kind": "service", "parent_slug": "a",
	}, nil)
	f.callExpectErr(t, "nottario.arch.remove_node", map[string]any{
		"project_id": f.projectID, "slug": "a",
	})
	f.callJSON(t, "nottario.arch.remove_node", map[string]any{
		"project_id": f.projectID, "slug": "a", "cascade": true,
	}, nil)
}

// TestMCP_Tasks_ClaimConflict claims an already-done task to hit
// the ClaimConflictError branch in nottario.tasks.claim.
func TestMCP_Tasks_ClaimConflict(t *testing.T) {
	f := newMCPFixture(t, 14090, "claimcfl")

	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "ship-it",
	}, &task)
	id, _ := task["id"].(string)
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": id, "state": "doing",
	}, nil)
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": id, "state": "done",
	}, nil)

	// Now claim it — surfaces a ClaimConflictError, returned as a
	// structured jsonResult (NOT an IsError result).
	var out map[string]any
	f.callJSON(t, "nottario.tasks.claim", map[string]any{
		"project_id": f.projectID, "task_id": id,
	}, &out)
	if out["error"] == nil || out["reason"] == nil {
		t.Errorf("expected claim conflict shape, got %+v", out)
	}
}

// TestMCP_Tasks_CreateWithPriorityAndAssignee covers the
// priority_key resolution + assignee_user_id assignment branches of
// tasks.create.
func TestMCP_Tasks_CreateWithPriorityAndAssignee(t *testing.T) {
	f := newMCPFixture(t, 14091, "createprio")

	// Unknown priority_key fails cleanly.
	f.callExpectErr(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "x", "priority_key": "blink",
	})

	// Known key + assignee + target role + explicit priority + type.
	var roles struct {
		Roles []map[string]any `json:"roles"`
	}
	f.callJSON(t, "nottario.projects.list_roles", map[string]any{"project_id": f.projectID}, &roles)
	roleID, _ := roles.Roles[0]["id"].(string)

	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id":       f.projectID,
		"title":            "prio",
		"description":      "with body",
		"type":             "bug",
		"priority_key":     "high",
		"assignee_user_id": f.userID,
		"target_role_id":   roleID,
	}, &task)
	if task["assignee_user_id"] != f.userID {
		t.Errorf("expected assignee on create: %+v", task)
	}

	// list with every optional filter so registerTasks's optUUID
	// branches all fire.
	var list struct {
		Tasks []map[string]any `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id":       f.projectID,
		"assignee_user_id": f.userID,
		"target_role_id":   roleID,
		"type":             "bug",
		"state":            "todo",
	}, &list)
	if len(list.Tasks) == 0 {
		t.Errorf("expected the filtered task back: %+v", list)
	}

	// list filtered by parent_task_id — create a feature parent first.
	var feature map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "parent", "type": "feature",
	}, &feature)
	parentID, _ := feature["id"].(string)
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "child", "parent_task_id": parentID,
	}, nil)
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id":     f.projectID,
		"parent_task_id": parentID,
	}, &list)

	// update with priority_key (separate code path than create).
	id, _ := task["id"].(string)
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id":   f.projectID,
		"task_id":      id,
		"priority_key": "low",
	}, nil)
	// update with explicit priority + assignee clear.
	empty := ""
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id":       f.projectID,
		"task_id":          id,
		"priority":         77,
		"assignee_user_id": empty,
	}, nil)
}

// TestMCP_Tasks_NonMemberRejected hits the `not a project member`
// branch of requireProjectAccess: an outsider (non-admin, no
// membership) gets a tool error when they touch another user's
// project. The first user upserted always becomes admin, so we
// register the project owner first and the outsider second.
func TestMCP_Tasks_NonMemberRejected(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	owner, _, err := identity.UpsertFromGithub(ctx, pool, 14093, "owner-cov", "Owner", "")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	outsider, _, err := identity.UpsertFromGithub(ctx, pool, 14094, "out-cov", "Out", "")
	if err != nil {
		t.Fatalf("outsider: %v", err)
	}
	if outsider.IsAdmin {
		t.Fatalf("test assumes only the first user is admin; outsider is admin")
	}
	p, err := identity.CreateProject(ctx, pool, "Owned", "", "", "", owner.ID, nil)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	outProj, err := identity.CreateProject(ctx, pool, "OutProj", "", "", "", outsider.ID, nil)
	if err != nil {
		t.Fatalf("outsider project: %v", err)
	}
	tok, _, err := identity.IssueToken(ctx, pool, outsider.ID, outProj.ID, "outsider", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	srv := NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	httpClient := &http.Client{Transport: bearerTransport{rt: http.DefaultTransport, token: tok}}
	client := sdk.NewClient(&sdk.Implementation{Name: "outsider", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             ts.URL + "/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      "nottario.tasks.list",
		Arguments: map[string]any{"project_id": p.ID.String()},
	})
	if err != nil {
		// Some setups surface auth errors at the protocol layer.
		return
	}
	if !res.IsError {
		t.Fatalf("expected an error for non-member access, got: %+v", res)
	}
	tc, _ := res.Content[0].(*sdk.TextContent)
	// Either rejection path is acceptable: scope guard fires first
	// when the token is project-scoped ("token scoped to project X");
	// the membership check fires next when scope passes ("not a
	// project member"). Both prove the non-member cannot reach data.
	if tc == nil {
		t.Fatalf("expected error text, got nil content")
	}
	low := strings.ToLower(tc.Text)
	if !strings.Contains(low, "member") && !strings.Contains(low, "scoped") {
		t.Errorf("expected rejection (member or scoped), got: %q", tc.Text)
	}
}

// TestMCP_Arch_KindsCRUD exercises list_kinds / upsert_kind /
// remove_kind which the family test doesn't currently touch.
func TestMCP_Arch_KindsCRUD(t *testing.T) {
	f := newMCPFixture(t, 14092, "archkinds")
	// Touch arch once so the default kind catalogue is seeded.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "root", "name": "Root", "kind": "system",
	}, nil)
	f.callJSON(t, "nottario.arch.list_kinds", map[string]any{
		"project_id": f.projectID,
	}, &struct{}{})
	f.callJSON(t, "nottario.arch.upsert_kind", map[string]any{
		"project_id": f.projectID, "key": "queue", "label": "Queue", "color": "#cf222e",
	}, nil)
	f.callJSON(t, "nottario.arch.remove_kind", map[string]any{
		"project_id": f.projectID, "key": "queue",
	}, nil)
}

// TestMCP_Docs_GlobalScope exercises the global-scope branch of
// resolveDocScope which the project-scoped tests don't cover.
func TestMCP_Docs_GlobalScope(t *testing.T) {
	f := newMCPFixture(t, 14080, "docsglob")

	// write global doc → list global → read global.
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"scope": "global", "path": "global/x.md", "content": "g",
	}, nil)
	var list struct {
		Documents []map[string]any `json:"documents"`
	}
	f.callJSON(t, "nottario.docs.list", map[string]any{
		"scope": "global",
	}, &list)
	if len(list.Documents) == 0 {
		t.Errorf("expected at least one global doc, got none")
	}
	var got map[string]any
	f.callJSON(t, "nottario.docs.read", map[string]any{
		"scope": "global", "path": "global/x.md",
	}, &got)
}
