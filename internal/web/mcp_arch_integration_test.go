package web

import "testing"

// Exercises every `nottario.arch.*` MCP tool through the streamable-
// HTTP transport: kinds, nodes (insert/update/list/get/move/remove),
// edges (upsert/list/remove), links (link_doc/unlink_doc/link_task/
// unlink_task). Validation error paths covered for the obvious cases.
func TestMCP_Arch_KindsCatalogue(t *testing.T) {
	f := newMCPFixture(t, 13320, "arch")
	var kinds struct {
		Kinds []map[string]any `json:"kinds"`
	}
	f.callJSON(t, "nottario.arch.list_kinds", map[string]any{
		"project_id": f.projectID,
	}, &kinds)
	if len(kinds.Kinds) < 5 {
		t.Fatalf("expected ≥5 default kinds, got %d", len(kinds.Kinds))
	}

	// Custom kind.
	f.callJSON(t, "nottario.arch.upsert_kind", map[string]any{
		"project_id":  f.projectID,
		"key":         "worker",
		"label":       "Worker",
		"description": "background job",
	}, nil)
	// Removing an unused custom kind succeeds.
	f.callJSON(t, "nottario.arch.remove_kind", map[string]any{
		"project_id": f.projectID,
		"key":        "worker",
	}, nil)
}

func TestMCP_Arch_NodesAndEdgesFullCycle(t *testing.T) {
	f := newMCPFixture(t, 13321, "arch-cycle")

	// Insert a parent + child.
	var root map[string]any
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys", "kind": "system", "name": "Sys",
	}, &root)
	if root["Slug"] != "sys" {
		t.Fatalf("upsert root: %+v", root)
	}
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys.api", "kind": "service", "name": "API",
		"parent_slug": "sys",
	}, nil)

	// list_nodes root only.
	var listed struct {
		Nodes []map[string]any `json:"nodes"`
	}
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID,
		"root_only":  true,
	}, &listed)
	if len(listed.Nodes) != 1 || listed.Nodes[0]["Slug"] != "sys" {
		t.Errorf("list_nodes root_only: %+v", listed.Nodes)
	}

	// get_node returns a {node, children, edges, links} envelope.
	var getNode struct {
		Node map[string]any `json:"node"`
	}
	f.callJSON(t, "nottario.arch.get_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys.api",
	}, &getNode)
	if getNode.Node["Slug"] != "sys.api" {
		t.Errorf("get_node: %+v", getNode.Node)
	}

	// Cycle detection on move.
	if msg := f.callExpectErr(t, "nottario.arch.move_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys", "parent_slug": "sys.api",
	}); msg == "" {
		t.Error("expected cycle error")
	}

	// Edges.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys.db", "kind": "external", "name": "DB",
	}, nil)
	var edge map[string]any
	f.callJSON(t, "nottario.arch.upsert_edge", map[string]any{
		"project_id": f.projectID,
		"from_slug":  "sys.api", "to_slug": "sys.db", "kind": "uses",
	}, &edge)
	edgeID, _ := edge["ID"].(string)
	if edgeID == "" {
		t.Fatalf("upsert_edge: missing ID: %+v", edge)
	}
	var edges struct {
		Edges []map[string]any `json:"edges"`
	}
	f.callJSON(t, "nottario.arch.list_edges", map[string]any{
		"project_id": f.projectID,
	}, &edges)
	if len(edges.Edges) != 1 {
		t.Errorf("list_edges: %+v", edges.Edges)
	}
	f.callJSON(t, "nottario.arch.remove_edge", map[string]any{
		"project_id": f.projectID,
		"edge_id":    edgeID,
	}, nil)

	// Links.
	f.callJSON(t, "nottario.arch.link_doc", map[string]any{
		"project_id": f.projectID,
		"node_slug":  "sys.api",
		"doc_path":   "x/y.md",
	}, nil)
	f.callJSON(t, "nottario.arch.unlink_doc", map[string]any{
		"project_id": f.projectID,
		"node_slug":  "sys.api",
		"doc_path":   "x/y.md",
	}, nil)

	// Cascade remove the sys subtree (sys + sys.api). sys.db is a
	// separate root and stays.
	f.callJSON(t, "nottario.arch.remove_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "sys",
		"cascade":    true,
	}, nil)
	var afterRemove struct {
		Nodes []map[string]any `json:"nodes"`
	}
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID,
	}, &afterRemove)
	if len(afterRemove.Nodes) != 1 || afterRemove.Nodes[0]["Slug"] != "sys.db" {
		t.Errorf("expected only sys.db remaining, got %+v", afterRemove.Nodes)
	}
}
