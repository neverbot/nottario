package mcp

import (
	"context"
	"errors"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/arch"
)

type archProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"uuid of the project"`
}

type archKindUpsertInput struct {
	archProjectInput
	Key         string `json:"key" jsonschema:"short identifier for the kind (snake-case, e.g. 'worker')"`
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
	Slug string `json:"slug" jsonschema:"the project-scoped slug of the node"`
}

type archNodeListInput struct {
	archProjectInput
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"when set, only the direct children of this node are returned"`
	RootOnly   bool   `json:"root_only,omitempty" jsonschema:"when true and parent_slug is empty, only top-level nodes are returned"`
}

type archNodeUpsertInput struct {
	archProjectInput
	Slug        string         `json:"slug" jsonschema:"snake-case readable id, scoped to the project"`
	ParentSlug  string         `json:"parent_slug,omitempty" jsonschema:"slug of the parent node, or empty for a root"`
	Kind        string         `json:"kind" jsonschema:"one of the project's kind keys (system, service, module, component, external, or any custom)"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty" jsonschema:"markdown description"`
	Metadata    map[string]any `json:"metadata,omitempty" jsonschema:"free-form metadata (lang, framework, port, etc.)"`
	LinkedRepo  string         `json:"linked_repo,omitempty" jsonschema:"'owner/repo' if the node maps to a GitHub repo (empty to clear)"`
	LinkedPath  string         `json:"linked_path,omitempty" jsonschema:"path inside the linked repo (e.g. 'internal/auth')"`
	Position    *int           `json:"position,omitempty" jsonschema:"sibling ordering"`
}

type archNodeMoveInput struct {
	archNodeRefInput
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"new parent slug, or empty to become a root"`
}

type archNodeRemoveInput struct {
	archNodeRefInput
	Cascade bool `json:"cascade,omitempty" jsonschema:"when true, descendants are deleted; otherwise the call is rejected if children exist"`
}

type archEdgeUpsertInput struct {
	archProjectInput
	FromSlug    string `json:"from_slug"`
	ToSlug      string `json:"to_slug"`
	Kind        string `json:"kind" jsonschema:"depends_on | uses | calls | reads | writes | publishes | subscribes (or custom)"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type archEdgeListInput struct {
	archProjectInput
	NodeSlug  string `json:"node_slug,omitempty"`
	Direction string `json:"direction,omitempty" jsonschema:"'in', 'out' or '' for both"`
	Kind      string `json:"kind,omitempty"`
}

type archEdgeRemoveInput struct {
	archProjectInput
	EdgeID string `json:"edge_id" jsonschema:"uuid of the edge to delete"`
}

type archLinkInput struct {
	archNodeRefInput
	DocPath string `json:"doc_path,omitempty" jsonschema:"path of a markdown document to attach"`
	TaskID  string `json:"task_id,omitempty" jsonschema:"uuid of a task to attach"`
}

func registerArch(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_kinds",
		Description: "Lists the kind catalogue of a project (system, service, module, component, external, plus custom kinds). The default catalogue is seeded the first time a project's architecture is touched.",
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
		Description: "Creates or updates a kind. The skill recommends reusing one of the defaults before adding a new one.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archKindUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		k, err := arch.UpsertKind(ctx, d.Pool, pid, arch.Kind{
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
		if err := arch.DeleteKind(ctx, d.Pool, pid, in.Key); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_nodes",
		Description: "Lists nodes of a project, optionally filtered by parent (parent_slug) or to roots only (root_only=true).",
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
		Description: "Fetches a node by slug, with its direct children, incident edges and attached documents/tasks.",
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
		Description: "Creates or updates a node keyed by (project_id, slug). The slug is the readable identifier you use in markdown and other tools; it must match [a-z0-9][a-z0-9._-]*. parent_slug nests this node under another; pass empty for a root. kind must already be present in the project's catalogue (see arch.list_kinds).",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		n, err := arch.UpsertNode(ctx, d.Pool, pid, arch.UpsertParams{
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
		n, err := arch.MoveNode(ctx, d.Pool, pid, in.Slug, in.ParentSlug)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(n)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.remove_node",
		Description: "Deletes a node. When the node has children, pass cascade=true to delete the whole subtree.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeRemoveInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if err := arch.RemoveNode(ctx, d.Pool, pid, in.Slug, in.Cascade); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.list_edges",
		Description: "Lists edges of a project (with both endpoint slugs and names). Use node_slug + direction to filter to one node.",
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
		Description: "Creates or updates a directed edge between two existing nodes. Self-loops are rejected. Kind is free-form but commonly 'depends_on', 'uses', 'calls', 'reads', 'writes', 'publishes', 'subscribes'.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archEdgeUpsertInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		e, err := arch.UpsertEdge(ctx, d.Pool, pid, arch.EdgeUpsertParams{
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
		if err := arch.RemoveEdge(ctx, d.Pool, pid, eid); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.link_doc",
		Description: "Attaches a markdown document (by path) to a node so future readers can find related context.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.DocPath == "" {
			return toolError("doc_path is required")
		}
		if err := arch.LinkDoc(ctx, d.Pool, pid, in.Slug, in.DocPath); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.unlink_doc",
		Description: "Removes a previous document link from a node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archLinkInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		if in.DocPath == "" {
			return toolError("doc_path is required")
		}
		if err := arch.UnlinkDoc(ctx, d.Pool, pid, in.Slug, in.DocPath); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.link_task",
		Description: "Attaches a task (by uuid) to a node so future readers can find related work.",
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
		if err := arch.LinkTask(ctx, d.Pool, pid, uuid.Nil, tid, in.Slug); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.unlink_task",
		Description: "Removes a previous task link from a node.",
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
		if err := arch.UnlinkTask(ctx, d.Pool, pid, in.Slug, tid); err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})
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
