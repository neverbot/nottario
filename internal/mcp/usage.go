package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/usage"
)

// usageMiddleware counts every tools/call and tools/list per project,
// token and tool: bytes in are the call's arguments, bytes out the text
// the agent receives (for tools/list, the catalogue it loads). It sits
// outside the toolset filter, so a narrowed catalogue is measured as
// the client sees it. Recording never fails the call.
func usageMiddleware(d Deps) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method != "tools/call" && method != "tools/list" {
				return next(ctx, method, req)
			}
			res, err := next(ctx, method, req)
			c, cerr := callerFromContext(ctx)
			if cerr != nil || d.Pool == nil {
				return res, err
			}
			call := usage.Call{ProjectID: c.ProjectID, TokenID: c.TokenID, Tool: method, Failed: err != nil}
			switch r := res.(type) {
			case *sdk.CallToolResult:
				for _, ct := range r.Content {
					if t, ok := ct.(*sdk.TextContent); ok {
						call.BytesOut += len(t.Text)
					}
				}
				call.Failed = call.Failed || r.IsError
			case *sdk.ListToolsResult:
				if b, mErr := json.Marshal(r.Tools); mErr == nil {
					call.BytesOut = len(b)
				}
			}
			if p, ok := req.(*sdk.CallToolRequest); ok && p.Params != nil {
				call.Tool = p.Params.Name
				call.BytesIn = len(p.Params.Arguments)
			}
			_ = usage.Record(context.WithoutCancel(ctx), d.Pool, call)
			return res, err
		}
	}
}
