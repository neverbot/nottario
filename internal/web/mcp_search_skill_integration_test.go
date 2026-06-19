package web

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// `nottario.search` and `nottario.skill.*` are small families and
// share a test file. Search hits a seeded task via the unified FTS
// query; skill list/read hit the embedded bundle (no DB writes).
func TestMCP_Search(t *testing.T) {
	f := newMCPFixture(t, 13340, "search-mcp")

	// Seed an arch node containing the query term so search has
	// something to return without depending on task seeding.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "thingamajig", "kind": "service", "name": "thingamajig",
		"description": "the thingamajig service handles widget orchestration",
	}, nil)

	var hits struct {
		Hits []map[string]any `json:"hits"`
	}
	f.callJSON(t, "nottario.search", map[string]any{
		"project_id": f.projectID,
		"query":      "thingamajig",
	}, &hits)
	if len(hits.Hits) == 0 {
		t.Error("search returned 0 hits for known content")
	}
	// Slim default: no raw description fallback on any hit.
	for i, h := range hits.Hits {
		if _, ok := h["description"]; ok {
			t.Errorf("slim search hit %d must omit 'description', got %+v", i, h)
		}
	}

	// verbose=true brings the raw description back.
	var verbose struct {
		Hits []map[string]any `json:"hits"`
	}
	f.callJSON(t, "nottario.search", map[string]any{
		"project_id": f.projectID,
		"query":      "thingamajig",
		"verbose":    true,
	}, &verbose)
	hasDescField := false
	for _, h := range verbose.Hits {
		if _, ok := h["description"]; ok {
			hasDescField = true
			break
		}
	}
	if !hasDescField {
		t.Errorf("verbose search must surface 'description', got hits=%+v", verbose.Hits)
	}
}

func TestMCP_Skill(t *testing.T) {
	f := newMCPFixture(t, 13341, "skill")

	var list struct {
		Files []map[string]any `json:"files"`
	}
	f.callJSON(t, "nottario.skill.list", map[string]any{}, &list)
	if len(list.Files) == 0 {
		t.Fatal("skill.list returned 0 files")
	}

	// Read the first file by its path. skill.read returns the raw
	// markdown as a single TextContent block (not JSON), so we read
	// it directly off the SDK call result.
	first := list.Files[0]
	path, _ := first["path"].(string)
	if path == "" {
		t.Fatalf("first file has no path: %+v", first)
	}
	res, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{
		Name:      "nottario.skill.read",
		Arguments: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatalf("skill.read: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("skill.read empty content")
	}
	tc, _ := res.Content[0].(*sdk.TextContent)
	if tc == nil || tc.Text == "" {
		t.Errorf("skill.read returned empty content for %s", path)
	}
}
