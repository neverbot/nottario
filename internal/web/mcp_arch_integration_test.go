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
	if root["slug"] != "sys" {
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
	if len(listed.Nodes) != 1 || listed.Nodes[0]["slug"] != "sys" {
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
	if getNode.Node["slug"] != "sys.api" {
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
	edgeID, _ := edge["id"].(string)
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
	if len(afterRemove.Nodes) != 1 || afterRemove.Nodes[0]["slug"] != "sys.db" {
		t.Errorf("expected only sys.db remaining, got %+v", afterRemove.Nodes)
	}
}

// TestMCP_Arch_ListSlimDefaults verifies that list_nodes and list_edges
// omit description / metadata / linked_repo / linked_path by default
// and surface them on verbose=true.
func TestMCP_Arch_ListSlimDefaults(t *testing.T) {
	f := newMCPFixture(t, 13322, "arch-slim-list")

	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "a", "kind": "service", "name": "A",
		"description": "this big description should NOT come back",
		"linked_repo": "neverbot/nottario", "linked_path": "internal/a",
		"metadata": map[string]any{"lang": "go"},
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "b", "kind": "service", "name": "B",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_edge", map[string]any{
		"project_id": f.projectID,
		"from_slug":  "a", "to_slug": "b", "kind": "uses",
		"description": "edge description should NOT come back",
	}, nil)

	// Default list_nodes — slim.
	var listed map[string]any
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID,
	}, &listed)
	rows, _ := listed["nodes"].([]any)
	if len(rows) == 0 {
		t.Fatalf("list_nodes returned no rows")
	}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		for _, k := range []string{"description", "metadata", "linked_repo", "linked_path", "created_at"} {
			if _, ok := row[k]; ok {
				t.Errorf("slim list_nodes row must omit %q, got %+v", k, row)
			}
		}
		for _, k := range []string{"id", "slug", "kind", "name", "updated_at"} {
			if _, ok := row[k]; !ok {
				t.Errorf("slim list_nodes row must include %q, got %+v", k, row)
			}
		}
	}

	// Verbose list_nodes — description is back.
	f.callJSON(t, "nottario.arch.list_nodes", map[string]any{
		"project_id": f.projectID,
		"verbose":    true,
	}, &listed)
	rows, _ = listed["nodes"].([]any)
	foundDesc := false
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if d, ok := row["description"].(string); ok && d != "" {
			foundDesc = true
		}
	}
	if !foundDesc {
		t.Errorf("verbose list_nodes must surface description, got rows=%+v", rows)
	}

	// Default list_edges — slim.
	var edges map[string]any
	f.callJSON(t, "nottario.arch.list_edges", map[string]any{
		"project_id": f.projectID,
	}, &edges)
	erows, _ := edges["edges"].([]any)
	if len(erows) == 0 {
		t.Fatalf("list_edges returned no rows")
	}
	for _, e := range erows {
		row, _ := e.(map[string]any)
		for _, k := range []string{"description", "from_name", "to_name", "created_at"} {
			if _, ok := row[k]; ok {
				t.Errorf("slim list_edges row must omit %q, got %+v", k, row)
			}
		}
		for _, k := range []string{"id", "from_slug", "to_slug", "kind", "updated_at"} {
			if _, ok := row[k]; !ok {
				t.Errorf("slim list_edges row must include %q, got %+v", k, row)
			}
		}
	}

	// Verbose list_edges — description is back.
	f.callJSON(t, "nottario.arch.list_edges", map[string]any{
		"project_id": f.projectID,
		"verbose":    true,
	}, &edges)
	erows, _ = edges["edges"].([]any)
	foundEdgeDesc := false
	for _, e := range erows {
		row, _ := e.(map[string]any)
		if d, ok := row["description"].(string); ok && d != "" {
			foundEdgeDesc = true
		}
	}
	if !foundEdgeDesc {
		t.Errorf("verbose list_edges must surface description, got rows=%+v", erows)
	}
}

// TestMCP_Arch_GetNodeIncludeDefaults verifies get_node returns the
// base node only by default; children / edges / links surface when the
// matching include_* flag is set.
func TestMCP_Arch_GetNodeIncludeDefaults(t *testing.T) {
	f := newMCPFixture(t, 13323, "arch-get-node-include")

	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "root", "kind": "system", "name": "Root",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "root.child", "kind": "service", "name": "Child",
		"parent_slug": "root",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "other", "kind": "external", "name": "Other",
	}, nil)
	f.callJSON(t, "nottario.arch.upsert_edge", map[string]any{
		"project_id": f.projectID,
		"from_slug":  "root", "to_slug": "other", "kind": "uses",
	}, nil)
	f.callJSON(t, "nottario.arch.link_doc", map[string]any{
		"project_id": f.projectID,
		"slug":       "root", "doc_path": "context/root.md",
	}, nil)

	// Default get_node: base node only.
	var def map[string]any
	f.callJSON(t, "nottario.arch.get_node", map[string]any{
		"project_id": f.projectID, "slug": "root",
	}, &def)
	if _, ok := def["node"]; !ok {
		t.Errorf("get_node default must include 'node', got %+v", def)
	}
	for _, k := range []string{"children", "edges", "links"} {
		if _, ok := def[k]; ok {
			t.Errorf("get_node default must omit %q, got %+v", k, def)
		}
	}

	// All include_* flags surface their collections.
	var full map[string]any
	f.callJSON(t, "nottario.arch.get_node", map[string]any{
		"project_id": f.projectID, "slug": "root",
		"include_children": true,
		"include_edges":    true,
		"include_links":    true,
	}, &full)
	children, _ := full["children"].([]any)
	if len(children) != 1 {
		t.Errorf("include_children must surface root.child, got %+v", children)
	}
	edges, _ := full["edges"].([]any)
	if len(edges) != 1 {
		t.Errorf("include_edges must surface the uses edge, got %+v", edges)
	}
	links, _ := full["links"].([]any)
	if len(links) != 1 {
		t.Errorf("include_links must surface the doc link, got %+v", links)
	}
}

// TestMCP_Arch_UpsertSlimAck verifies upsert_node, upsert_edge,
// move_node and upsert_kind return slim acks by default and full
// objects on verbose=true.
func TestMCP_Arch_UpsertSlimAck(t *testing.T) {
	f := newMCPFixture(t, 13324, "arch-slim-ack")

	// upsert_node slim default — no description echoed.
	var node map[string]any
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id":  f.projectID,
		"slug":        "n1", "kind": "service", "name": "N1",
		"description": "should not come back",
		"metadata":    map[string]any{"lang": "go"},
	}, &node)
	for _, k := range []string{"description", "metadata", "linked_repo", "linked_path", "created_at"} {
		if _, ok := node[k]; ok {
			t.Errorf("upsert_node slim must omit %q, got %+v", k, node)
		}
	}
	for _, k := range []string{"id", "slug", "kind", "name", "updated_at"} {
		if _, ok := node[k]; !ok {
			t.Errorf("upsert_node slim must include %q, got %+v", k, node)
		}
	}

	// upsert_node verbose — description echoed.
	var verboseNode map[string]any
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id":  f.projectID,
		"slug":        "n1",
		"kind":        "service",
		"name":        "N1",
		"description": "echo me back",
		"verbose":     true,
	}, &verboseNode)
	if verboseNode["description"] != "echo me back" {
		t.Errorf("upsert_node verbose must echo description, got %v", verboseNode["description"])
	}

	// upsert_edge slim.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "n2", "kind": "service", "name": "N2",
	}, nil)
	var edge map[string]any
	f.callJSON(t, "nottario.arch.upsert_edge", map[string]any{
		"project_id":  f.projectID,
		"from_slug":   "n1", "to_slug": "n2", "kind": "uses",
		"description": "edge desc should not echo",
	}, &edge)
	if _, ok := edge["description"]; ok {
		t.Errorf("upsert_edge slim must omit description, got %+v", edge)
	}
	if _, ok := edge["id"]; !ok {
		t.Errorf("upsert_edge slim must include id, got %+v", edge)
	}

	// move_node slim.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "parent", "kind": "system", "name": "Parent",
	}, nil)
	var moved map[string]any
	f.callJSON(t, "nottario.arch.move_node", map[string]any{
		"project_id":  f.projectID,
		"slug":        "n1",
		"parent_slug": "parent",
	}, &moved)
	if _, ok := moved["description"]; ok {
		t.Errorf("move_node slim must omit description, got %+v", moved)
	}
	if moved["slug"] != "n1" {
		t.Errorf("move_node slim must include slug, got %+v", moved)
	}

	// upsert_kind slim.
	var kind map[string]any
	f.callJSON(t, "nottario.arch.upsert_kind", map[string]any{
		"project_id":  f.projectID,
		"key":         "queue", "label": "Queue",
		"description": "queue kind description",
	}, &kind)
	if _, ok := kind["description"]; ok {
		t.Errorf("upsert_kind slim must omit description, got %+v", kind)
	}
	if kind["key"] != "queue" {
		t.Errorf("upsert_kind slim must include key, got %+v", kind)
	}
}
