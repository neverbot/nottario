package web

import (
	"strings"
	"testing"
)

// Exercises the `nottario.docs.*` MCP tool family: write/read,
// optimistic-concurrency version_conflict, history, delete, search.
func TestMCP_Docs_WriteReadVersionDelete(t *testing.T) {
	f := newMCPFixture(t, 13330, "docs")
	path := "projects/" + f.projectID + "/notes/x.md"

	// First write: ExpectedVersion 0 (new doc).
	var v1 map[string]any
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             path,
		"content":          "# v1\n\nfirst",
		"expected_version": 0,
		"message":          "init",
	}, &v1)
	if v1["current_version"].(float64) != 1 {
		t.Fatalf("expected CurrentVersion=1, got %v", v1["current_version"])
	}

	// Read back.
	var read map[string]any
	f.callJSON(t, "nottario.docs.read", map[string]any{
		"project_id": f.projectID,
		"path":       path,
	}, &read)
	if !strings.Contains(read["content"].(string), "first") {
		t.Errorf("read content unexpected: %+v", read)
	}

	// Version conflict: send the wrong expected_version. The docs.write
	// tool reports this through the response body (`error":"version_conflict"`)
	// rather than IsError, so check the structured content.
	var conflict map[string]any
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             path,
		"content":          "stale",
		"expected_version": 0,
		"message":          "stale-update",
	}, &conflict)
	if conflict["error"] != "version_conflict" {
		t.Errorf("expected error=version_conflict, got: %+v", conflict)
	}

	// Successful update.
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             path,
		"content":          "# v2\n\nsecond",
		"expected_version": 1,
		"message":          "update",
	}, nil)

	// History.
	var hist struct {
		Versions []map[string]any `json:"versions"`
	}
	f.callJSON(t, "nottario.docs.history", map[string]any{
		"project_id": f.projectID,
		"path":       path,
	}, &hist)
	if len(hist.Versions) < 2 {
		t.Errorf("expected ≥2 versions, got %d", len(hist.Versions))
	}

	// read_version v1.
	var v1read map[string]any
	f.callJSON(t, "nottario.docs.read_version", map[string]any{
		"project_id": f.projectID,
		"path":       path,
		"version":    1,
	}, &v1read)
	if !strings.Contains(v1read["content"].(string), "first") {
		t.Errorf("read_version v1: %+v", v1read)
	}

	// List documents.
	var list struct {
		Documents []map[string]any `json:"documents"`
	}
	f.callJSON(t, "nottario.docs.list", map[string]any{
		"project_id": f.projectID,
	}, &list)
	if len(list.Documents) == 0 {
		t.Error("docs.list returned 0")
	}

	// Search.
	var hits struct {
		Hits []map[string]any `json:"hits"`
	}
	f.callJSON(t, "nottario.docs.search", map[string]any{
		"project_id": f.projectID,
		"query":      "second",
	}, &hits)
	if len(hits.Hits) == 0 {
		t.Error("docs.search returned 0 for known content")
	}

	// Delete.
	f.callJSON(t, "nottario.docs.delete", map[string]any{
		"project_id": f.projectID,
		"path":       path,
	}, nil)
	// After delete, read errors.
	msg := f.callExpectErr(t, "nottario.docs.read", map[string]any{
		"project_id": f.projectID,
		"path":       path,
	})
	if msg == "" {
		t.Error("expected read error after delete")
	}
}
