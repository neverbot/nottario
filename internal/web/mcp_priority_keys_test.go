package web

import (
	"strings"
	"testing"
)

// TestMCP_Tasks_PriorityIsSetByKeyOnly pins the rule that agents
// cannot leave a task on a number no bucket names: the task tools take
// a priority key, refuse a raw number, and name the valid keys when
// the key is wrong.
func TestMCP_Tasks_PriorityIsSetByKeyOnly(t *testing.T) {
	f := newMCPFixture(t, 13977, "priority-keys")

	// No priority at all lands on the project's default bucket.
	var task struct {
		ID       string `json:"id"`
		Priority int    `json:"priority"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "default priority",
	}, &task)
	if task.Priority != 60 {
		t.Fatalf("default priority = %d, want 60 (medium)", task.Priority)
	}

	// A key resolves to its bucket, on create and on update.
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "high", "priority_key": "high",
	}, &task)
	if task.Priority != 90 {
		t.Fatalf("priority_key=high gave %d, want 90", task.Priority)
	}
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": task.ID, "priority_key": "low",
	}, &task)
	if task.Priority != 30 {
		t.Fatalf("update priority_key=low gave %d, want 30", task.Priority)
	}

	// A raw number is refused on both tools and changes nothing.
	f.callExpectErr(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "raw", "priority": 70,
	})
	f.callExpectErr(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": task.ID, "priority": 70,
	})
	var after struct {
		Task struct {
			Priority int `json:"priority"`
		} `json:"task"`
	}
	f.callJSON(t, "nottario.tasks.get", map[string]any{
		"project_id": f.projectID, "task_id": task.ID,
	}, &after)
	if after.Task.Priority != 30 {
		t.Fatalf("priority after refused raw update = %d, want 30", after.Task.Priority)
	}
	var listed struct {
		Tasks []struct {
			Title string `json:"title"`
		} `json:"tasks"`
	}
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID}, &listed)
	for _, x := range listed.Tasks {
		if x.Title == "raw" {
			t.Fatal("a task was created from a call carrying a raw priority")
		}
	}

	// An unknown key says which keys exist.
	calls := map[string]map[string]any{
		"nottario.tasks.create": {"project_id": f.projectID, "title": "x", "priority_key": "p70"},
		"nottario.tasks.update": {"project_id": f.projectID, "task_id": task.ID, "priority_key": "p70"},
	}
	for tool, args := range calls {
		msg := f.callExpectErr(t, tool, args)
		for _, want := range []string{"p70", "low", "medium", "high", "critical"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s unknown-key error %q does not mention %q", tool, msg, want)
			}
		}
	}
}
