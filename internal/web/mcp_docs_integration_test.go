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

// TestMCP_Docs_ReadHeadOnly verifies the head_only flag returns a
// preview with truncated/body_length markers instead of the full body.
func TestMCP_Docs_ReadHeadOnly(t *testing.T) {
	f := newMCPFixture(t, 13335, "docs-head-only")

	// Body larger than the 400-char preview limit.
	long := ""
	for i := 0; i < 600; i++ {
		long += "x"
	}
	body := "---\ntitle: Head Only Test\n---\n" + long
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             "context/big.md",
		"content":          body,
		"expected_version": 0,
	}, nil)

	// head_only: preview only.
	var head map[string]any
	f.callJSON(t, "nottario.docs.read", map[string]any{
		"project_id": f.projectID,
		"path":       "context/big.md",
		"head_only":  true,
	}, &head)
	if head["title"] != "Head Only Test" {
		t.Errorf("head_only must include title, got %+v", head)
	}
	preview, _ := head["content"].(string)
	if got := len(preview); got != 400 {
		t.Errorf("head_only preview must be 400 chars, got %d", got)
	}
	if head["truncated"] != true {
		t.Errorf("head_only must mark truncated=true when body exceeds preview, got %v", head["truncated"])
	}
	if blf, _ := head["body_length"].(float64); int(blf) != 600 {
		t.Errorf("head_only body_length expected 600, got %v", head["body_length"])
	}

	// Full read returns the complete body.
	var full map[string]any
	f.callJSON(t, "nottario.docs.read", map[string]any{
		"project_id": f.projectID,
		"path":       "context/big.md",
	}, &full)
	if c, _ := full["content"].(string); len(c) < 600 {
		t.Errorf("full read must return full body, got len=%d", len(c))
	}
	if _, ok := full["truncated"]; ok {
		t.Errorf("full read must not include truncated key, got %+v", full)
	}
}

// The docs.write ack is deliberately slim: it must carry the new
// version so the caller can chain the next write, and must NOT echo
// the body back. Echoing hands the agent a second copy of something
// it just composed, which on a large document is the single most
// expensive thing this tool could do to its context.
func TestMCP_Docs_WriteReturnsSlimAck(t *testing.T) {
	f := newMCPFixture(t, 13336, "docs-slim-ack")
	path := "projects/" + f.projectID + "/notes/slim.md"
	body := "# Slim\n\n" + strings.Repeat("filler paragraph. ", 200)

	var ack map[string]any
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             path,
		"content":          body,
		"expected_version": 0,
		"message":          "init",
	}, &ack)

	for _, k := range []string{"path", "current_version", "updated_at"} {
		if _, ok := ack[k]; !ok {
			t.Errorf("ack is missing %q: %+v", k, ack)
		}
	}
	if _, ok := ack["content"]; ok {
		t.Error("ack echoed the document body back")
	}
	// Anything carrying the body would be far larger than the handful
	// of scalars the ack is meant to be.
	if got := len(ack); got > 4 {
		t.Errorf("ack has %d keys, expected a slim shape: %+v", got, ack)
	}
	if ack["path"] != path {
		t.Errorf("path = %v, want %q", ack["path"], path)
	}
	if v, ok := ack["current_version"].(float64); !ok || v != 1 {
		t.Errorf("current_version = %v, want 1", ack["current_version"])
	}

	// The version in the ack is usable directly as the next
	// expected_version — that is the whole reason it is returned.
	var ack2 map[string]any
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id":       f.projectID,
		"path":             path,
		"content":          body + "\nmore",
		"expected_version": int(ack["current_version"].(float64)),
		"message":          "update",
	}, &ack2)
	if v, ok := ack2["current_version"].(float64); !ok || v != 2 {
		t.Errorf("second write current_version = %v, want 2", ack2["current_version"])
	}
}
