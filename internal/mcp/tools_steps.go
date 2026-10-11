package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/tasks"
)

type stepsAddInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project uuid"`
	TaskID    string   `json:"task_id" jsonschema:"task uuid"`
	Texts     []string `json:"texts" jsonschema:"one or more steps, each a single line of up to 200 characters, appended in this order"`
}

type stepRefInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	TaskID    string `json:"task_id" jsonschema:"task uuid"`
	StepID    string `json:"step_id" jsonschema:"step uuid"`
}

type stepsUpdateInput struct {
	ProjectID string  `json:"project_id" jsonschema:"project uuid"`
	TaskID    string  `json:"task_id" jsonschema:"task uuid"`
	StepID    string  `json:"step_id" jsonschema:"step uuid"`
	Text      *string `json:"text,omitempty" jsonschema:"new text, a single line of up to 200 characters"`
	Done      *bool   `json:"done,omitempty" jsonschema:"true ticks the step, false unticks it"`
}

type stepsMoveInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	TaskID    string `json:"task_id" jsonschema:"task uuid"`
	StepID    string `json:"step_id" jsonschema:"step uuid"`
	Position  int    `json:"position" jsonschema:"0-based target position; clamped to the list"`
}

// pendingStep is a step left undone, as the close response lists it.
type pendingStep struct {
	ID   uuid.UUID `json:"id"`
	Text string    `json:"text"`
}

// pendingNotice adds pending_steps and a notice to a response when a
// task reached done with steps still undone. Closing is allowed; the
// agent must tell the human what was left.
func pendingNotice(out map[string]any, pending []tasks.Step) map[string]any {
	if len(pending) == 0 {
		return out
	}
	list := make([]pendingStep, 0, len(pending))
	for _, s := range pending {
		list = append(list, pendingStep{ID: s.ID, Text: s.Text})
	}
	out["pending_steps"] = list
	noun := "steps"
	if len(pending) == 1 {
		noun = "step"
	}
	out["notice"] = fmt.Sprintf("This task was closed with %d pending %s. Tell the human which steps were left undone.", len(pending), noun)
	return out
}

// payloadMap turns a task payload into a map so fields can be added
// next to it without changing its shape.
func payloadMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(b, &out)
}

// resolveStepTask checks the caller may touch the task the step tools
// name and returns its ids.
func resolveStepTask(ctx context.Context, d Deps, projectID, taskID string) (uuid.UUID, error) {
	pid, tid, err := parseProjectAndTask(projectID, taskID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := requireProjectAccess(ctx, d, pid); err != nil {
		return uuid.Nil, err
	}
	if err := tasks.RequireInProject(ctx, d.Pool, pid, tid); err != nil {
		return uuid.Nil, errors.New("task not found")
	}
	return tid, nil
}

func parseStepID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, errors.New("invalid step_id")
	}
	return id, nil
}

func registerSteps(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.add_steps",
		Description: "Appends steps to a task's checklist and returns the whole list {steps}. A step is a short action worth remembering that needs no owner, role, dependency or commits of its own (if it does, file a subtask). Add a step when new work appears mid-task instead of writing a long comment. Each text is one line, up to 200 characters; a task holds at most 50 steps.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in stepsAddInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		tid, err := resolveStepTask(ctx, d, in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		steps, err := tasks.AddSteps(ctx, d.Pool, tid, in.Texts, authorshipFor(c))
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"steps": steps})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.update_step",
		Description: "Ticks or unticks a step (done), rewrites its text, or both. Tick each step as you finish it, not all at the end. Returns the step.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in stepsUpdateInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		tid, err := resolveStepTask(ctx, d, in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		sid, err := parseStepID(in.StepID)
		if err != nil {
			return toolError(err.Error())
		}
		s, err := tasks.UpdateStep(ctx, d.Pool, tid, sid, tasks.UpdateStepParams{Text: in.Text, Done: in.Done}, authorshipFor(c))
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(s)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.remove_step",
		Description: "Deletes a step. Use it for a step that turned out to be unneeded: steps are either done or deleted, never left pending to mean 'skipped'.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in stepRefInput) (*sdk.CallToolResult, any, error) {
		tid, err := resolveStepTask(ctx, d, in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		sid, err := parseStepID(in.StepID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := tasks.RemoveStep(ctx, d.Pool, tid, sid); err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"removed": true})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.tasks.move_step",
		Description: "Moves a step to a 0-based position (clamped to the list) and returns the whole list {steps} in its new order.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in stepsMoveInput) (*sdk.CallToolResult, any, error) {
		tid, err := resolveStepTask(ctx, d, in.ProjectID, in.TaskID)
		if err != nil {
			return toolError(err.Error())
		}
		sid, err := parseStepID(in.StepID)
		if err != nil {
			return toolError(err.Error())
		}
		steps, err := tasks.MoveStep(ctx, d.Pool, tid, sid, in.Position)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"steps": steps})
	})
}
