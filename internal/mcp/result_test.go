package mcp

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool results land in an agent's context, so the JSON text carries no
// indentation.
func TestJSONResult_Compact(t *testing.T) {
	res, _, err := jsonResult(map[string]any{"task": map[string]any{"id": "x", "steps": []int{1, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Content[0].(*sdk.TextContent).Text
	if want := `{"task":{"id":"x","steps":[1,2]}}`; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}
