package web

import (
	"strings"
	"testing"
)

// Covers the `nottario.tasks.*` MCP family end-to-end through the
// streamable-HTTP transport. We don't re-test the underlying repo
// invariants (the tasks integration tests already do) — the focus is
// the tool wiring and JSON shape on the wire.
func TestMCP_Tasks_CreateGetUpdateState(t *testing.T) {
	f := newMCPFixture(t, 13350, "tasks")

	// Create.
	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID,
		"title":      "first",
	}, &task)
	id, _ := task["ID"].(string)
	if id == "" || task["State"] != "todo" {
		t.Fatalf("create: %+v", task)
	}

	// Get.
	var got map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID,
		"task_id":    id,
	}, &got)
	if g, ok := got["task"]; ok {
		if m, ok := g.(map[string]any); ok && m["ID"] != id {
			t.Errorf("get task ID mismatch: %+v", m)
		}
	}

	// Update title.
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID,
		"task_id":    id,
		"title":      "first-updated",
	}, nil)

	// set_state doing.
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID,
		"task_id":    id,
		"state":      "doing",
	}, nil)

	// list with state filter.
	var list struct {
		Tasks []map[string]any `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id": f.projectID,
		"state":      "doing",
	}, &list)
	if len(list.Tasks) != 1 || list.Tasks[0]["ID"] != id {
		t.Errorf("list doing: %+v", list.Tasks)
	}
}

func TestMCP_Tasks_DependenciesAndClaim(t *testing.T) {
	f := newMCPFixture(t, 13351, "tasks-deps")

	mk := func(title string) string {
		t.Helper()
		var tk map[string]any
		f.callJSON(t, "nottario.tasks.create", map[string]any{
			"project_id": f.projectID, "title": title,
		}, &tk)
		return tk["ID"].(string)
	}
	a := mk("A")
	b := mk("B")
	c := mk("C")

	// b depends on a; c depends on b.
	f.callJSON(t, "nottario.tasks.add_dependency", map[string]any{
		"project_id": f.projectID, "task_id": b, "depends_on_id": a,
	}, nil)
	f.callJSON(t, "nottario.tasks.add_dependency", map[string]any{
		"project_id": f.projectID, "task_id": c, "depends_on_id": b,
	}, nil)

	// Cycle: a -> c would close the loop.
	msg := f.callExpectErr(t, "nottario.tasks.add_dependency", map[string]any{
		"project_id": f.projectID, "task_id": a, "depends_on_id": c,
	})
	if !strings.Contains(strings.ToLower(msg), "cycle") && !strings.Contains(strings.ToLower(msg), "depend") {
		t.Errorf("expected cycle error, got: %s", msg)
	}

	// next preview returns A (no deps).
	var nxt map[string]any
	f.callJSON(t, "nottario.tasks.next", map[string]any{
		"project_id": f.projectID,
	}, &nxt)
	if tk, ok := nxt["task"].(map[string]any); !ok || tk["ID"] != a {
		t.Errorf("next preview expected A: %+v", nxt)
	}

	// claim_next atomically picks A and transitions to doing.
	var claimed map[string]any
	f.callJSON(t, "nottario.tasks.claim_next", map[string]any{
		"project_id": f.projectID,
	}, &claimed)
	if ct, ok := claimed["task"].(map[string]any); !ok || ct["ID"] != a {
		t.Errorf("claim_next expected A: %+v", claimed)
	}

	// remove_dependency.
	f.callJSON(t, "nottario.tasks.remove_dependency", map[string]any{
		"project_id": f.projectID, "task_id": c, "depends_on_id": b,
	}, nil)
}

func TestMCP_Tasks_LinkCommitAndComment(t *testing.T) {
	f := newMCPFixture(t, 13352, "tasks-meta")

	var tk map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "meta",
	}, &tk)
	id := tk["ID"].(string)

	f.callJSON(t, "nottario.tasks.link_commit", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"repo": "neverbot/nottario", "sha": "abc1234",
		"message": "wip",
	}, nil)

	f.callJSON(t, "nottario.tasks.add_comment", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"body": "thinking out loud",
	}, nil)

	// get should now include the commit + comment.
	var got map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID, "task_id": id,
	}, &got)
	commits, _ := got["commits"].([]any)
	comments, _ := got["comments"].([]any)
	if len(commits) == 0 || len(comments) == 0 {
		t.Errorf("expected commits and comments after link/add: commits=%d comments=%d", len(commits), len(comments))
	}
}
