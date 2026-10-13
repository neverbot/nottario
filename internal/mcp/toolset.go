package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// A toolset narrows the tool catalogue one MCP connection sees. Every
// tool definition a client loads costs tokens in the agent's context
// for the whole session, and some clients load them all up front, so
// an agent that never touches the architecture graph should not carry
// its ten tools. The filter lives in the MCP URL the human configures:
//
//	/mcp?tools=tasks,docs     only these domains (whoami always stays)
//	/mcp?shell=true           the client has a shell: hide docs.read and
//	                          docs.write, it moves documents with
//	                          docs.download_url / docs.upload_url
//
// No parameters means the whole catalogue, as before.
type toolset struct {
	domains map[string]bool // nil = every domain
	shell   bool
}

// toolDomains are the values ?tools= accepts: the second segment of a
// tool name (nottario.<domain>.<verb>).
var toolDomains = []string{"arch", "cycles", "docs", "projects", "search", "skill", "tasks"}

// shellHidden are the tools a client with a shell does without.
var shellHidden = []string{"nottario.docs.read", "nottario.docs.write"}

func parseToolset(r *http.Request) (toolset, error) {
	q := r.URL.Query()
	var ts toolset
	if raw := q.Get("tools"); raw != "" {
		ts.domains = map[string]bool{}
		for _, d := range strings.Split(raw, ",") {
			d = strings.TrimSpace(strings.ToLower(d))
			if d == "" {
				continue
			}
			if !isToolDomain(d) {
				return toolset{}, errors.New("unknown tools domain " + d + "; valid: " + strings.Join(toolDomains, ","))
			}
			ts.domains[d] = true
		}
	}
	switch strings.ToLower(q.Get("shell")) {
	case "", "false", "0":
	case "true", "1":
		ts.shell = true
	default:
		return toolset{}, errors.New("shell must be true or false")
	}
	return ts, nil
}

func isToolDomain(d string) bool {
	for _, k := range toolDomains {
		if k == d {
			return true
		}
	}
	return false
}

// keeps reports whether a tool belongs in this toolset.
func (ts toolset) keeps(name string) bool {
	if ts.shell {
		for _, h := range shellHidden {
			if name == h {
				return false
			}
		}
	}
	if ts.domains == nil {
		return true
	}
	if name == "nottario.whoami" {
		return true
	}
	parts := strings.SplitN(name, ".", 3) // nottario.<domain>[.<verb>]
	return len(parts) >= 2 && ts.domains[parts[1]]
}

type toolsetCtxKey struct{}

func withToolset(ctx context.Context, ts toolset) context.Context {
	return context.WithValue(ctx, toolsetCtxKey{}, ts)
}

// toolsetMiddleware applies the connection's toolset: tools/list
// leaves out what the toolset hides, and tools/call refuses it, so a
// filtered client cannot reach a tool it was never shown.
func toolsetMiddleware(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		ts, _ := ctx.Value(toolsetCtxKey{}).(toolset)
		switch method {
		case "tools/call":
			if r, ok := req.(*sdk.CallToolRequest); ok && r.Params != nil && !ts.keeps(r.Params.Name) {
				return nil, fmt.Errorf("unknown tool %q", r.Params.Name)
			}
		case "tools/list":
			res, err := next(ctx, method, req)
			if lr, ok := res.(*sdk.ListToolsResult); ok && err == nil {
				kept := lr.Tools[:0:0]
				for _, t := range lr.Tools {
					if ts.keeps(t.Name) {
						kept = append(kept, t)
					}
				}
				lr.Tools = kept
			}
			return res, err
		}
		return next(ctx, method, req)
	}
}
