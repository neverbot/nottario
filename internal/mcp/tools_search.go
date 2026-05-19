package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/search"
)

type searchInput struct {
	ProjectID string   `json:"project_id" jsonschema:"uuid of the project to search in"`
	Query     string   `json:"query" jsonschema:"keywords to search; plainto_tsquery semantics"`
	Kinds     []string `json:"kinds,omitempty" jsonschema:"optional filter: any subset of 'task','document','arch_node'"`
	Limit     int      `json:"limit,omitempty" jsonschema:"max results (default 50)"`
}

func registerSearch(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.search",
		Description: "Cross-cutting full-text search across tasks, documents and architecture nodes of a project, ranked by relevance. Each hit carries 'kind' so you can decide how to follow it (task_id, doc_path, node_slug).",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		var kinds []search.Kind
		for _, k := range in.Kinds {
			kinds = append(kinds, search.Kind(k))
		}
		hits, err := search.Search(ctx, d.Pool, in.Query, search.Filter{
			ProjectID: pid,
			Kinds:     kinds,
			Limit:     in.Limit,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"hits": hits})
	})
}
