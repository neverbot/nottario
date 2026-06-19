package mcp

import (
	"context"
	"errors"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
)

// cyclesProjectInput is the common input for read endpoints scoped to
// a project.
type cyclesProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
}

// cycleGetInput fetches a specific cycle by id.
type cycleGetInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	CycleID   string `json:"cycle_id" jsonschema:"cycle uuid"`
}

// cycleEndInput closes the project's active cycle and opens the next.
type cycleEndInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	NextName  string `json:"next_name,omitempty" jsonschema:"name for the new cycle (default: auto-numbered)"`
}

func registerCycles(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.cycles.list",
		Description: "Lists every cycle of a project (newest first). closed_at is null on the active cycle.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in cyclesProjectInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		out, err := cycles.List(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"cycles": out})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.cycles.current",
		Description: "Returns the project's active cycle.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in cyclesProjectInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		c, err := cycles.ActiveCycle(ctx, d.Pool, pid)
		if err != nil {
			if errors.Is(err, cycles.ErrNoActiveCycle) {
				return toolError("no active cycle for project")
			}
			return toolError(err.Error())
		}
		return jsonResult(c)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.cycles.get",
		Description: "Fetches a cycle by id.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in cycleGetInput) (*sdk.CallToolResult, any, error) {
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		cid, err := uuid.Parse(in.CycleID)
		if err != nil {
			return toolError("cycle_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		c, err := cycles.Get(ctx, d.Pool, cid)
		if err != nil || c.ProjectID != pid {
			return toolError("cycle not found")
		}
		return jsonResult(c)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.cycles.end",
		Description: "Owner/admin only. Closes the active cycle and opens the next atomically. In-flight tasks cascade per cycles rules. Returns {closed, next}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in cycleEndInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		if err := identity.RequireProjectOwner(ctx, d.Pool, pid, c.UserID, c.IsAdmin); err != nil {
			return toolError(err.Error())
		}
		by := cycles.Authorship{UserID: &c.UserID}
		if c.Source == identity.SourceToken {
			tid := c.TokenID
			by.TokenID = &tid
		}
		res, err := cycles.EndCycle(ctx, d.Pool, cycles.EndCycleParams{
			ProjectID: pid,
			NextName:  in.NextName,
		}, by)
		if err != nil {
			if errors.Is(err, cycles.ErrNoActiveCycle) {
				return toolError(err.Error())
			}
			return toolError(err.Error())
		}
		return jsonResult(res)
	})
}
