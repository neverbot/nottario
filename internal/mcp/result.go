package mcp

import (
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// jsonResult marshals v as pretty JSON and wraps it in a CallToolResult.
// Tool clients receive a single text content with the JSON payload.
func jsonResult(v any) (*sdk.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: string(b)}},
	}, v, nil
}

// textResult wraps a plain string in a CallToolResult.
func textResult(s string) (*sdk.CallToolResult, any, error) {
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: s}},
	}, nil, nil
}

// toolError marks a result as an error to the MCP client without
// returning a Go error (which the SDK would translate as a protocol-
// level failure). Use this for user-visible problems like
// "project_id is required".
func toolError(msg string) (*sdk.CallToolResult, any, error) {
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: msg}},
	}, nil, nil
}
