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
	id, _ := task["id"].(string)
	if id == "" || task["state"] != "todo" {
		t.Fatalf("create: %+v", task)
	}

	// Get.
	var got map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID,
		"task_id":    id,
	}, &got)
	if g, ok := got["task"]; ok {
		if m, ok := g.(map[string]any); ok && m["id"] != id {
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
	if len(list.Tasks) != 1 || list.Tasks[0]["id"] != id {
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
		return tk["id"].(string)
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
	if tk, ok := nxt["task"].(map[string]any); !ok || tk["id"] != a {
		t.Errorf("next preview expected A: %+v", nxt)
	}

	// claim_next atomically picks A and transitions to doing.
	var claimed map[string]any
	f.callJSON(t, "nottario.tasks.claim_next", map[string]any{
		"project_id": f.projectID,
	}, &claimed)
	if ct, ok := claimed["task"].(map[string]any); !ok || ct["id"] != a {
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
	id := tk["id"].(string)

	f.callJSON(t, "nottario.tasks.link_commit", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"repo": "neverbot/nottario", "sha": "abc1234",
		"message": "wip",
	}, nil)

	f.callJSON(t, "nottario.tasks.add_comment", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"body": "thinking out loud",
	}, nil)

	// get with explicit include flags returns commits + comments.
	var got map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id":       f.projectID,
		"task_id":          id,
		"include_commits":  true,
		"include_comments": true,
	}, &got)
	commits, _ := got["commits"].([]any)
	comments, _ := got["comments"].([]any)
	if len(commits) == 0 || len(comments) == 0 {
		t.Errorf("expected commits and comments after link/add: commits=%d comments=%d", len(commits), len(comments))
	}

	// Without the include_* flags, the same call omits them entirely
	// (not just empty arrays) so agents don't pay tokens for fields
	// they didn't ask for.
	var lean map[string]any
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID, "task_id": id,
	}, &lean)
	if _, ok := lean["commits"]; ok {
		t.Errorf("default tasks.get must omit 'commits' key, got %v", lean["commits"])
	}
	if _, ok := lean["comments"]; ok {
		t.Errorf("default tasks.get must omit 'comments' key, got %v", lean["comments"])
	}
	if _, ok := lean["depends_on"]; ok {
		t.Errorf("default tasks.get must omit 'depends_on' key, got %v", lean["depends_on"])
	}
}

// TestMCP_Tasks_SlimResponses verifies that the high-frequency
// mutations return the slim shape by default and full shape on
// verbose=true. The slim shape is the canonical "id, type, title,
// state, priority, parent_task_id, target_role_id, assignee_user_id,
// updated_at" tuple — explicitly no description, no created_by, no
// actual_*. Adding fields to slimTask is a deliberate (and tested)
// change; removing fields IS a breaking change to clients.
func TestMCP_Tasks_SlimResponses(t *testing.T) {
	f := newMCPFixture(t, 13360, "slim-responses")

	// tasks.create slim by default — description not echoed back.
	var slim map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id":  f.projectID,
		"title":       "slim test",
		"description": "this 2 KB description should NOT come back when we omit verbose",
	}, &slim)
	id := slim["id"].(string)
	if _, ok := slim["description"]; ok {
		t.Errorf("default tasks.create must omit 'description', got %q", slim["description"])
	}
	if _, ok := slim["created_by_user_id"]; ok {
		t.Errorf("default tasks.create must omit 'created_by_user_id'")
	}
	if _, ok := slim["actual_start"]; ok {
		t.Errorf("default tasks.create must omit 'actual_start'")
	}
	for _, k := range []string{"id", "type", "title", "state", "priority", "updated_at"} {
		if _, ok := slim[k]; !ok {
			t.Errorf("slim tasks.create missing required key %q: %+v", k, slim)
		}
	}

	// tasks.create verbose returns the description.
	var verbose map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id":  f.projectID,
		"title":       "verbose test",
		"description": "echo this back please",
		"verbose":     true,
	}, &verbose)
	if verbose["description"] != "echo this back please" {
		t.Errorf("verbose tasks.create must echo description, got %v", verbose["description"])
	}

	// tasks.set_state slim by default.
	var st map[string]any
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": id, "state": "doing",
	}, &st)
	if _, ok := st["description"]; ok {
		t.Errorf("default tasks.set_state must omit 'description'")
	}
	if st["state"] != "doing" {
		t.Errorf("tasks.set_state state=%v want doing", st["state"])
	}

	// tasks.add_comment slim — body NOT echoed.
	var cm map[string]any
	f.callJSON(t, "nottario.tasks.add_comment", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"body": "a very long markdown comment that should NOT be echoed back",
	}, &cm)
	if _, ok := cm["body"]; ok {
		t.Errorf("default tasks.add_comment must omit 'body', got %q", cm["body"])
	}
	for _, k := range []string{"id", "task_id", "created_at", "updated_at"} {
		if _, ok := cm[k]; !ok {
			t.Errorf("slim tasks.add_comment missing required key %q: %+v", k, cm)
		}
	}

	// tasks.add_comment verbose echoes the body.
	var cmv map[string]any
	f.callJSON(t, "nottario.tasks.add_comment", map[string]any{
		"project_id": f.projectID, "task_id": id,
		"body":    "echo me",
		"verbose": true,
	}, &cmv)
	if cmv["body"] != "echo me" {
		t.Errorf("verbose tasks.add_comment must echo body, got %v", cmv["body"])
	}

	// tasks.list slim per row.
	var list map[string]any
	f.callJSON(t, "nottario.tasks.list", map[string]any{
		"project_id": f.projectID,
	}, &list)
	rows, _ := list["tasks"].([]any)
	if len(rows) == 0 {
		t.Fatalf("tasks.list returned no rows")
	}
	for i, r := range rows {
		row, _ := r.(map[string]any)
		if _, ok := row["description"]; ok {
			t.Errorf("default tasks.list row %d must omit 'description'", i)
		}
	}
}
