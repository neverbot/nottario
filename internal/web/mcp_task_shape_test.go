package web

import (
	"strings"
	"testing"
)

// Agents name roles by key and themselves as "me", and get tasks back
// in the same terms: no uuid to look up, times to the second.
func TestMCP_TaskShape_RoleKeysAndMe(t *testing.T) {
	f := newMCPFixture(t, 14501, "task-shape")

	var created map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "keyed", "target_role_id": "backend", "assignee_user_id": "me",
		"description": "the body",
	}, &created)
	if created["role"] != "backend" || created["assignee"] != "me" {
		t.Fatalf("create: %v", created)
	}
	for _, k := range []string{"target_role_id", "assignee_user_id"} {
		if _, ok := created[k]; ok {
			t.Errorf("slim task still carries %s: %v", k, created)
		}
	}
	if ts, _ := created["updated_at"].(string); !strings.HasSuffix(ts, "Z") || strings.Contains(ts, ".") {
		t.Errorf("updated_at = %q, want UTC to the second", ts)
	}

	var page struct {
		Tasks []map[string]any `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID, "target_role_id": "backend", "assignee_user_id": "me"}, &page)
	if len(page.Tasks) != 1 || page.Tasks[0]["role"] != "backend" {
		t.Errorf("list by role key and me: %v", page.Tasks)
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID, "target_role_id": "qa"}, &page)
	if len(page.Tasks) != 0 {
		t.Errorf("role filter ignored: %v", page.Tasks)
	}
	if msg := f.callExpectErr(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID, "target_role_id": "nope"}); !strings.Contains(msg, "backend") {
		t.Errorf("unknown role should list the valid keys: %q", msg)
	}

	var upd map[string]any
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": created["id"], "target_role_id": "qa", "assignee_user_id": "",
	}, &upd)
	if upd["role"] != "qa" || upd["assignee"] != nil {
		t.Errorf("update: %v", upd)
	}

	// tasks.get: the agent shape plus the description; verbose for the
	// full row.
	var got struct {
		Task map[string]any `json:"task"`
	}
	f.callJSON(t, "nottario.tasks.get", map[string]any{"project_id": f.projectID, "task_id": created["id"]}, &got)
	if got.Task["description"] != "the body" || got.Task["role"] != "qa" {
		t.Errorf("get: %v", got.Task)
	}
	if _, ok := got.Task["via_mcp"]; ok {
		t.Errorf("get carries via_mcp by default: %v", got.Task)
	}
	f.callJSON(t, "nottario.tasks.get", map[string]any{"project_id": f.projectID, "task_id": created["id"], "verbose": true}, &got)
	if _, ok := got.Task["target_role_id"]; !ok {
		t.Errorf("verbose get lacks the full row: %v", got.Task)
	}
}
