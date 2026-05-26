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
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
}

// cycleGetInput fetches a specific cycle by id.
type cycleGetInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
	CycleID   string `json:"cycle_id" jsonschema:"uuid of the cycle"`
}

// cycleEndInput closes the project's active cycle and opens the next.
type cycleEndInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
	NextName  string `json:"next_name,omitempty" jsonschema:"optional name for the new cycle; defaults to '<cycle_label>-<N+1>'"`
}

func registerCycles(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.cycles.list",
		Description: "Lists every cycle of a project (newest first). Each entry has id, name, position, opened_at and closed_at (null when active). Exactly one cycle per project has closed_at = null.",
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
		Description: "Returns the project's currently active cycle (the unique row with closed_at = null). Use this to discover which cycle new tasks land in by default.",
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
		Description: "Fetches a single cycle by id. The cycle must belong to the given project.",
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
		Description: "Closes the project's active cycle and opens the next one in a single transaction. Owner-gated (project owner or instance admin). In-flight work moves forward per cascade rules: partial feature subtrees move whole (children re-stamped), standalone non-done tasks move, done tasks stay stamped on the closing cycle. Returns {closed, next}.",
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
