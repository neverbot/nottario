package mcp

import (
	"context"
	"errors"
	"time"

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
	Slug            string `json:"slug" jsonschema:"node slug"`
	IncludeChildren bool   `json:"include_children,omitempty" jsonschema:"get_node: include children (default off)"`
	IncludeEdges    bool   `json:"include_edges,omitempty" jsonschema:"get_node: include incident edges (default off)"`
	IncludeLinks    bool   `json:"include_links,omitempty" jsonschema:"get_node: include doc/task links (default off)"`
}

type archNodeListInput struct {
	archProjectInput
	ParentSlug string `json:"parent_slug,omitempty" jsonschema:"direct children of this node"`
	RootOnly   bool   `json:"root_only,omitempty" jsonschema:"only top-level nodes"`
	Verbose    bool   `json:"verbose,omitempty" jsonschema:"full Node per row instead of slim shape"`
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
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"full Edge per row instead of slim shape"`
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
		Description: "Lists nodes (slim by default: id, slug, parent_id, kind, name, position, updated_at). Filter by parent_slug or root_only. Pass verbose=true for the full Node.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in archNodeListInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		ns, err := arch.ListNodes(ctx, d.Pool, pid, in.ParentSlug, in.RootOnly)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"nodes": slimNodeList(ns, in.Verbose)})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.arch.get_node",
		Description: "Fetches a node with its description. include_children / include_edges / include_links opt in to the related collections (each default off — they can be large).",
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
		out := map[string]any{"node": n}
		if in.IncludeChildren {
			children, _ := arch.ListNodes(ctx, d.Pool, pid, in.Slug, false)
			out["children"] = slimNodeList(children, false)
		}
		if in.IncludeEdges {
			edges, _ := arch.ListEdges(ctx, d.Pool, pid, arch.EdgeFilter{NodeSlug: in.Slug})
			out["edges"] = slimEdgeList(edges, false)
		}
		if in.IncludeLinks {
			links, _ := arch.ListLinks(ctx, d.Pool, pid, in.Slug)
			out["links"] = links
		}
		return jsonResult(out)
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
		Description: "Lists edges (slim by default: id, from_slug, to_slug, kind, label, updated_at). node_slug+direction filters to one node. Pass verbose=true for the full Edge.",
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
		return jsonResult(map[string]any{"edges": slimEdgeList(es, in.Verbose)})
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

type slimNode struct {
	ID        uuid.UUID  `json:"id"`
	Slug      string     `json:"slug"`
	ParentID  *uuid.UUID `json:"parent_id"`
	Kind      string     `json:"kind"`
	Name      string     `json:"name"`
	Position  int        `json:"position"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type slimEdge struct {
	ID        uuid.UUID `json:"id"`
	FromSlug  string    `json:"from_slug"`
	ToSlug    string    `json:"to_slug"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func slimNodeList(ns []arch.Node, verbose bool) any {
	if verbose {
		return ns
	}
	out := make([]slimNode, 0, len(ns))
	for _, n := range ns {
		out = append(out, slimNode{
			ID: n.ID, Slug: n.Slug, ParentID: n.ParentID,
			Kind: n.Kind, Name: n.Name, Position: n.Position,
			UpdatedAt: n.UpdatedAt,
		})
	}
	return out
}

func slimEdgeList(es []arch.EdgeView, verbose bool) any {
	if verbose {
		return es
	}
	out := make([]slimEdge, 0, len(es))
	for _, e := range es {
		out = append(out, slimEdge{
			ID: e.ID, FromSlug: e.FromSlug, ToSlug: e.ToSlug,
			Kind: e.Kind, Label: e.Label, UpdatedAt: e.UpdatedAt,
		})
	}
	return out
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
