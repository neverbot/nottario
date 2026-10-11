package web

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Agents keep a task's checklist through four tools, read it back with
// tasks.get, see the progress on list rows, and are told when they
// close a task with steps still undone.
func TestMCP_TaskSteps(t *testing.T) {
	f := newMCPFixture(t, 14402, "mcp-steps")
	mk := func(title string) string {
		t.Helper()
		var out struct {
			ID string `json:"id"`
		}
		f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": title}, &out)
		return out.ID
	}
	type stepsOut struct {
		Steps []stepView `json:"steps"`
	}
	task := mk("with steps")
	ref := func(extra map[string]any) map[string]any {
		args := map[string]any{"project_id": f.projectID, "task_id": task}
		for k, v := range extra {
			args[k] = v
		}
		return args
	}

	var added stepsOut
	f.callJSON(t, "nottario.tasks.add_steps", ref(map[string]any{"texts": []string{"a", "b", "c"}}), &added)
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(stepViewTexts(added.Steps), want) {
		t.Fatalf("add_steps: %v", stepViewTexts(added.Steps))
	}
	var ticked stepView
	f.callJSON(t, "nottario.tasks.update_step", ref(map[string]any{"step_id": added.Steps[0].ID, "done": true}), &ticked)
	if !ticked.Done {
		t.Errorf("update_step done: %+v", ticked)
	}
	var moved stepsOut
	f.callJSON(t, "nottario.tasks.move_step", ref(map[string]any{"step_id": added.Steps[2].ID, "position": 0}), &moved)
	if want := []string{"c", "a", "b"}; !reflect.DeepEqual(stepViewTexts(moved.Steps), want) {
		t.Errorf("move_step: %v", stepViewTexts(moved.Steps))
	}
	var removed struct {
		Removed bool `json:"removed"`
	}
	f.callJSON(t, "nottario.tasks.remove_step", ref(map[string]any{"step_id": added.Steps[1].ID}), &removed)
	if !removed.Removed {
		t.Error("remove_step did not answer removed")
	}

	var got struct {
		Task struct {
			Steps *struct{ Done, Total int } `json:"steps"`
		} `json:"task"`
		Steps []stepView `json:"steps"`
	}
	f.callJSON(t, "nottario.tasks.get", ref(nil), &got)
	if want := []string{"c", "a"}; !reflect.DeepEqual(stepViewTexts(got.Steps), want) {
		t.Errorf("tasks.get steps: %v", stepViewTexts(got.Steps))
	}
	if got.Task.Steps == nil || got.Task.Steps.Done != 1 || got.Task.Steps.Total != 2 {
		t.Errorf("tasks.get progress: %+v", got.Task.Steps)
	}

	var page struct {
		Tasks []struct {
			ID    string                     `json:"id"`
			Steps *struct{ Done, Total int } `json:"steps"`
		} `json:"tasks"`
	}
	bare := mk("no steps")
	f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectID}, &page)
	for _, row := range page.Tasks {
		switch row.ID {
		case task:
			if row.Steps == nil || row.Steps.Total != 2 {
				t.Errorf("list row progress: %+v", row.Steps)
			}
		case bare:
			if row.Steps != nil {
				t.Errorf("task without steps lists progress %+v", row.Steps)
			}
		}
	}

	// Rejections.
	f.callExpectErr(t, "nottario.tasks.add_steps", ref(map[string]any{"texts": []string{"two\nlines"}}))
	f.callExpectErr(t, "nottario.tasks.update_step", ref(map[string]any{"step_id": "nope", "done": true}))
	many := make([]string, 49)
	for i := range many {
		many[i] = fmt.Sprintf("s%d", i)
	}
	if msg := f.callExpectErr(t, "nottario.tasks.add_steps", ref(map[string]any{"texts": many})); !strings.Contains(msg, "48 more fit") {
		t.Errorf("cap message = %q", msg)
	}
	other := mk("other")
	f.callExpectErr(t, "nottario.tasks.update_step", map[string]any{
		"project_id": f.projectID, "task_id": other, "step_id": added.Steps[0].ID, "done": false,
	})

	type closeOut struct {
		Task struct {
			State string `json:"state"`
		} `json:"task"`
		State        string `json:"state"`
		PendingSteps []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"pending_steps"`
		Notice string `json:"notice"`
	}
	var closed closeOut
	f.callJSON(t, "nottario.tasks.close", ref(map[string]any{"comment": "done enough"}), &closed)
	if closed.Task.State != "done" {
		t.Errorf("close state = %q", closed.Task.State)
	}
	if len(closed.PendingSteps) != 1 || closed.PendingSteps[0].Text != "c" || !strings.Contains(closed.Notice, "Tell the human") {
		t.Errorf("close pending = %+v notice %q", closed.PendingSteps, closed.Notice)
	}

	// set_state to done says the same; without steps, nothing.
	viaState := mk("via set_state")
	f.callJSON(t, "nottario.tasks.add_steps", map[string]any{"project_id": f.projectID, "task_id": viaState, "texts": []string{"left"}}, nil)
	var stateOut closeOut
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{"project_id": f.projectID, "task_id": viaState, "state": "done"}, &stateOut)
	if stateOut.State != "done" || len(stateOut.PendingSteps) != 1 || stateOut.Notice == "" {
		t.Errorf("set_state done: %+v", stateOut)
	}
	var clean closeOut
	f.callJSON(t, "nottario.tasks.close", map[string]any{"project_id": f.projectID, "task_id": bare}, &clean)
	if clean.PendingSteps != nil || clean.Notice != "" {
		t.Errorf("close without steps carries a notice: %+v", clean)
	}
}
