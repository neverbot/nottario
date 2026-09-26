package web

import (
	"encoding/json"
	"strings"
	"testing"
)

// mustJSONString renders a decoded tool result back to text so a test
// can look for an id or a name without walking the shape of every
// response.
func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// The cycle family is how an agent understands "which sprint are we
// in" and how a sprint is closed. Ending one is the single most
// consequential call in the domain: it moves every open task forward.
func TestMCP_Cycles_CurrentGetAndEnd(t *testing.T) {
	f := newMCPFixture(t, 13390, "cycles-mcp")

	var current map[string]any
	f.callJSON(t, "nottario.cycles.current", map[string]any{"project_id": f.projectID}, &current)
	cycle, _ := current["cycle"].(map[string]any)
	if cycle == nil {
		cycle = current
	}
	id, _ := cycle["id"].(string)
	if id == "" {
		t.Fatalf("cycles.current returned no cycle: %+v", current)
	}
	if cycle["closed_at"] != nil {
		t.Errorf("the active cycle should be open: %+v", cycle)
	}

	var fetched map[string]any
	f.callJSON(t, "nottario.cycles.get", map[string]any{
		"project_id": f.projectID, "cycle_id": id,
	}, &fetched)
	if !strings.Contains(mustJSONString(fetched), id) {
		t.Errorf("cycles.get returned a different cycle: %+v", fetched)
	}

	// A task in flight moves into the new cycle; that cascade is the
	// whole point of ending one.
	var task map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "carried forward", "claim": true,
	}, &task)

	var ended map[string]any
	f.callJSON(t, "nottario.cycles.end", map[string]any{
		"project_id": f.projectID, "next_name": "sprint-next",
	}, &ended)
	body := mustJSONString(ended)
	if !strings.Contains(body, "sprint-next") {
		t.Fatalf("cycles.end did not open the named cycle: %s", body)
	}
	if !strings.Contains(body, id) {
		t.Errorf("cycles.end does not report which cycle it closed: %s", body)
	}

	var after map[string]any
	f.callJSON(t, "nottario.cycles.current", map[string]any{"project_id": f.projectID}, &after)
	if strings.Contains(mustJSONString(after), id) {
		t.Errorf("the closed cycle is still reported as current: %+v", after)
	}

	// The claimed task followed the sprint rather than being left behind.
	var listed map[string]any
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID}, &listed)
	if !strings.Contains(mustJSONString(listed), "carried forward") {
		t.Errorf("open work did not move into the new cycle: %+v", listed)
	}

	var stale map[string]any
	f.callJSON(t, "nottario.cycles.list", map[string]any{"project_id": f.projectID}, &stale)
	if !strings.Contains(mustJSONString(stale), "sprint-next") {
		t.Errorf("cycles.list does not show the new cycle: %+v", stale)
	}
}

// Checkpointing names a moment in the diagram's history, the way a
// commit names one in the code.
func TestMCP_Arch_Checkpoint(t *testing.T) {
	f := newMCPFixture(t, 13391, "arch-checkpoint-mcp")

	f.callJSON(t, "nottario.arch.upsert_kind", map[string]any{
		"project_id": f.projectID, "key": "service", "label": "Service",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID, "slug": "api", "kind": "service", "name": "API",
	}, nil)

	var snap map[string]any
	f.callJSON(t, "nottario.arch.checkpoint", map[string]any{
		"project_id": f.projectID, "message": "first shape of the diagram",
	}, &snap)
	if !strings.Contains(mustJSONString(snap), "first shape") {
		t.Errorf("checkpoint does not echo its message: %+v", snap)
	}
}

// Ownership changes hands through this tool; the wrong user id must
// not silently succeed.
func TestMCP_Projects_SetOwner(t *testing.T) {
	f := newMCPFixture(t, 13392, "set-owner-mcp")

	var before map[string]any
	f.callJSON(t, "nottario.projects.get", map[string]any{"project_id": f.projectID}, &before)

	// Handing the project to the caller themselves is a no-op that
	// still exercises the whole path.
	f.callJSON(t, "nottario.projects.set_owner", map[string]any{
		"project_id": f.projectID, "new_owner_id": f.userID,
	}, nil)

	var after map[string]any
	f.callJSON(t, "nottario.projects.get", map[string]any{"project_id": f.projectID}, &after)
	if !strings.Contains(mustJSONString(after), f.userID) {
		t.Errorf("the owner is not the caller after set_owner: %+v", after)
	}

	if msg := f.callExpectErr(t, "nottario.projects.set_owner", map[string]any{
		"project_id": f.projectID, "new_owner_id": "00000000-0000-0000-0000-000000000000",
	}); msg == "" {
		t.Error("set_owner accepted a user that does not exist")
	}
}

// The drift report an agent can ask for before trusting the board.
func TestMCP_Tasks_Inconsistencies(t *testing.T) {
	f := newMCPFixture(t, 13393, "inconsistencies-mcp")

	var groundwork, dependent map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "groundwork",
	}, &groundwork)
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "depends on it",
	}, &dependent)
	f.callJSON(t, "nottario.tasks.add_dependency", map[string]any{
		"project_id": f.projectID, "task_id": dependent["id"], "depends_on_id": groundwork["id"],
	}, nil)

	var clean map[string]any
	f.callJSON(t, "nottario.tasks.inconsistencies", map[string]any{"project_id": f.projectID}, &clean)
	if strings.Contains(mustJSONString(clean), groundwork["id"].(string)) {
		t.Errorf("a healthy backlog reports drift: %+v", clean)
	}

	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": groundwork["id"], "state": "done",
	}, nil)
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": dependent["id"], "state": "done",
	}, nil)
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": groundwork["id"], "state": "todo",
	}, nil)

	var drift map[string]any
	f.callJSON(t, "nottario.tasks.inconsistencies", map[string]any{"project_id": f.projectID}, &drift)
	if !strings.Contains(mustJSONString(drift), groundwork["id"].(string)) {
		t.Errorf("reopening a finished precondition was not reported: %+v", drift)
	}
}
