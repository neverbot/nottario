package mcp

import (
	"context"
	"errors"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/arch"
)

// archBy resolves the MCP caller to an arch.Authorship value. Returns
// an empty Authorship and the wrapped error when no caller is in the
// context (auth middleware did not run).
func archBy(ctx context.Context) (arch.Authorship, error) {
	c, err := callerFromContext(ctx)
	if err != nil {
		return arch.Authorship{}, err
	}
	return arch.Authorship{UserID: c.UserID, TokenID: ptrUUID(c.TokenID)}, nil
}

type archProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
}

type archKindUpsertInput struct {
	archProjectInput
	Key         string `json:"key" jsonschema:"snake-case identifier"`
	Label       string `json:"label" jsonschema:"display label"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

type archKindDeleteInput struct {
	archProjectInput
	Key string `json:"key"`
}

type archNodeRefInput struct {
	archProjectInput
	Slug string `json:"slug" jsonschema:"node slug"`
}

type archNodeListInput struct {
	archProjectInput
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"direct children of this node"`
	RootOnly   bool   `json:"root_only,omitempty" jsonschema:"only top-level nodes"`
}

type archNodeUpsertInput struct {
	archProjectInput
	Slug        string         `json:"slug" jsonschema:"snake-case id"`
	ParentSlug  string         `json:"parent_slug,omitempty" jsonschema:"parent slug, '' for root"`
	Kind        string         `json:"kind" jsonschema:"a kind key from arch.list_kinds"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty" jsonschema:"markdown"`
	Metadata    map[string]any `json:"metadata,omitempty" jsonschema:"free-form metadata"`
	LinkedRepo  string         `json:"linked_repo,omitempty" jsonschema:"'owner/repo' or '' to clear"`
	LinkedPath  string         `json:"linked_path,omitempty" jsonschema:"path inside the linked repo"`
	Position    *int           `json:"position,omitempty" jsonschema:"sibling order"`
}

type archNodeMoveInput struct {
	archNodeRefInput
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"new parent slug, '' for root"`
}

type archNodeRemoveInput struct {
	archNodeRefInput
	Cascade bool `json:"cascade,omitempty" jsonschema:"delete the whole subtree"`
}

type archEdgeUpsertInput struct {
	archProjectInput
	FromSlug    string `json:"from_slug"`
	ToSlug      string `json:"to_slug"`
	Kind        string `json:"kind" jsonschema:"depends_on|uses|calls|reads|writes|publishes|subscribes (or custom)"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type archEdgeListInput struct {
	archProjectInput
	NodeSlug  string `json:"node_slug,omitempty"`
	Direction string `json:"direction,omitempty" jsonschema:"'in','out' or ''"`
	Kind      string `json:"kind,omitempty"`
}

type archEdgeRemoveInput struct {
	archProjectInput
	EdgeID string `json:"edge_id" jsonschema:"edge uuid"`
}

type archLinkInput struct {
	archNodeRefInput
	DocPath string `json:"doc_path,omitempty" jsonschema:"document path"`
	TaskID  string `json:"task_id,omitempty" jsonschema:"task uuid"`
}

func registerArch(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_kinds",
		Description: "Lists the kind catalogue of a project. Defaults seeded on first use.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archProjectInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		ks, err := arch.ListKinds(ctx, d.Pool, pid)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"kinds": ks})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.upsert_kind",
		Description: "Creates or updates a kind. Reuse a default before adding new.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archKindUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		k, err := arch.UpsertKind(ctx, d.Pool, pid, by, arch.Kind{
			Key: in.Key, Label: in.Label, Icon: in.Icon, Color: in.Color, Description: in.Description,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(k)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.remove_kind",
		Description: "Deletes a kind. Fails if any node still uses it.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archKindDeleteInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.DeleteKind(ctx, d.Pool, pid, by, in.Key); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_nodes",
		Description: "Lists nodes; filter by parent_slug or root_only.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeListInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		ns, err := arch.ListNodes(ctx, d.Pool, pid, in.ParentSlug, in.RootOnly)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"nodes": ns})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.get_node",
		Description: "Fetches a node with its children, incident edges and linked docs/tasks.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeRefInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		n, err := arch.GetNode(ctx, d.Pool, pid, in.Slug)
		if errors.Is(err, arch.ErrNodeNotFound) {
			return toolError("node not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		children, _ := arch.ListNodes(ctx, d.Pool, pid, in.Slug, false)
		edges, _ := arch.ListEdges(ctx, d.Pool, pid, arch.EdgeFilter{NodeSlug: in.Slug})
		links, _ := arch.ListLinks(ctx, d.Pool, pid, in.Slug)
		return jsonResult(map[string]any{
			"node":     n,
			"children": children,
			"edges":    edges,
			"links":    links,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.upsert_node",
		Description: "Creates or updates a node keyed by (project_id, slug). Slug matches [a-z0-9][a-z0-9._-]*. kind must exist in arch.list_kinds.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		n, err := arch.UpsertNode(ctx, d.Pool, pid, by, arch.UpsertParams{
			Slug: in.Slug, ParentSlug: in.ParentSlug, Kind: in.Kind, Name: in.Name,
			DescriptionMD: in.Description, Metadata: in.Metadata,
			LinkedRepo: in.LinkedRepo, LinkedPath: in.LinkedPath, Position: in.Position,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(n)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.move_node",
		Description: "Reparents a node. Pass parent_slug='' to make it a root. Cycles are rejected.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeMoveInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		n, err := arch.MoveNode(ctx, d.Pool, pid, by, in.Slug, in.ParentSlug)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(n)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.remove_node",
		Description: "Deletes a node. cascade=true to delete the subtree.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeRemoveInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.RemoveNode(ctx, d.Pool, pid, by, in.Slug, in.Cascade); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_edges",
		Description: "Lists edges. node_slug+direction filters to one node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archEdgeListInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		es, err := arch.ListEdges(ctx, d.Pool, pid, arch.EdgeFilter{
			NodeSlug: in.NodeSlug, Direction: in.Direction, Kind: in.Kind,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"edges": es})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.upsert_edge",
		Description: "Creates or updates a directed edge. Self-loops are rejected.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archEdgeUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		e, err := arch.UpsertEdge(ctx, d.Pool, pid, by, arch.EdgeUpsertParams{
			FromSlug: in.FromSlug, ToSlug: in.ToSlug, Kind: in.Kind,
			Label: in.Label, DescriptionMD: in.Description,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(e)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.remove_edge",
		Description: "Deletes an edge by its uuid.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archEdgeRemoveInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		eid, err := uuid.Parse(in.EdgeID)
		if err != nil {
			return toolError("edge_id must be a uuid")
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.RemoveEdge(ctx, d.Pool, pid, by, eid); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.link_doc",
		Description: "Attaches a document (by path) to a node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.DocPath == "" {
			return toolError("doc_path is required")
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.LinkDoc(ctx, d.Pool, pid, by, in.Slug, in.DocPath); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.unlink_doc",
		Description: "Removes a doc link from a node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.DocPath == "" {
			return toolError("doc_path is required")
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.UnlinkDoc(ctx, d.Pool, pid, by, in.Slug, in.DocPath); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.link_task",
		Description: "Attaches a task (by uuid) to a node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.TaskID == "" {
			return toolError("task_id is required")
		}
		tid, err := uuid.Parse(in.TaskID)
		if err != nil {
			return toolError("task_id must be a uuid")
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.LinkTask(ctx, d.Pool, pid, by, tid, in.Slug); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.unlink_task",
		Description: "Removes a task link from a node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.TaskID == "" {
			return toolError("task_id is required")
		}
		tid, err := uuid.Parse(in.TaskID)
		if err != nil {
			return toolError("task_id must be a uuid")
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.UnlinkTask(ctx, d.Pool, pid, by, in.Slug, tid); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.checkpoint",
		Description: "Snapshots the diagram editing session with a commit-like message. Auto-snapshots on idle if you forget.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archCheckpointInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		by, err := archBy(ctx)
		if err != nil {
			return toolError(err.Error())
		}
		res, err := arch.Checkpoint(ctx, d.Pool, pid, by, in.Message)
		if errors.Is(err, arch.ErrNoActiveSession) {
			return toolError("no active arch session for you on this project — there is nothing to checkpoint right now")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(res)
	})
}

type archCheckpointInput struct {
	archProjectInput
	Message string `json:"message,omitempty" jsonschema:"revision title"`
}

// archProject parses the project uuid and verifies caller access.
func archProject(ctx context.Context, d Deps, projectIDStr string) (uuid.UUID, error) {
	pid, err := uuid.Parse(projectIDStr)
	if err != nil {
		return uuid.Nil, errors.New("project_id must be a uuid")
	}
	if err := requireProjectAccess(ctx, d, pid); err != nil {
		return uuid.Nil, err
	}
	return pid, nil
}
