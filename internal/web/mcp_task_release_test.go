package web

import (
	"strings"
	"testing"
)

// An agent that gives work back must be able to drop the assignee:
// some MCP clients lose empty strings, so "none" unsets, and set_state
// to todo can release the task in the same call.
func TestMCP_TaskUnassign(t *testing.T) {
	f := newMCPFixture(t, 14502, "task-release")

	var created map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "give back", "target_role_id": "backend", "claim": true,
	}, &created)
	if created["assignee"] != "me" || created["state"] != "doing" {
		t.Fatalf("create: %v", created)
	}

	var upd map[string]any
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "assignee_user_id": "none", "target_role_id": "None",
	}, &upd)
	if upd["assignee"] != nil || upd["role"] != nil {
		t.Fatalf("update with none: %v", upd)
	}

	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "assignee_user_id": "me",
	}, &upd)
	if msg := f.callExpectErr(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "state": "done", "release": true,
	}); !strings.Contains(msg, "todo") {
		t.Errorf("release outside todo: %q", msg)
	}

	var back map[string]any
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "state": "todo", "release": true,
	}, &back)
	if back["state"] != "todo" || back["assignee"] != nil {
		t.Fatalf("set_state todo with release: %v", back)
	}
}
