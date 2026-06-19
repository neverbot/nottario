package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/search"
)

type searchInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project uuid"`
	Query     string   `json:"query" jsonschema:"plainto_tsquery"`
	Kinds     []string `json:"kinds,omitempty" jsonschema:"subset of 'task','document','arch_node'"`
	Limit     int      `json:"limit,omitempty" jsonschema:"max results (default 20, max 100)"`
	Verbose   bool     `json:"verbose,omitempty" jsonschema:"include the raw description fallback alongside the highlighted snippet"`
}

// slimHit drops the raw `description` fallback that ships in
// `search.Hit` for the web UI — the MCP caller only needs the
// highlighted snippet (`description_html`). Empty fields stay omitted
// so the wire payload mirrors omitempty semantics.
type slimHit struct {
	Kind            string  `json:"kind"`
	ProjectID       string  `json:"project_id"`
	Rank            float32 `json:"rank"`
	Title           string  `json:"title"`
	TitleHTML       string  `json:"title_html,omitempty"`
	DescriptionHTML string  `json:"description_html,omitempty"`
	TaskID          string  `json:"task_id,omitempty"`
	DocPath         string  `json:"doc_path,omitempty"`
	DocScope        string  `json:"doc_scope,omitempty"`
	NodeSlug        string  `json:"node_slug,omitempty"`
	NodeKind        string  `json:"node_kind,omitempty"`
	TaskState       string  `json:"task_state,omitempty"`
	TaskType        string  `json:"task_type,omitempty"`
}

func toSlimHits(hits []search.Hit) []slimHit {
	out := make([]slimHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, slimHit{
			Kind: string(h.Kind), ProjectID: h.ProjectID, Rank: h.Rank,
			Title: h.Title, TitleHTML: h.TitleHTML, DescriptionHTML: h.DescriptionHTML,
			TaskID: h.TaskID, DocPath: h.DocPath, DocScope: h.DocScope,
			NodeSlug: h.NodeSlug, NodeKind: h.NodeKind,
			TaskState: h.TaskState, TaskType: h.TaskType,
		})
	}
	return out
}

func registerSearch(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.search",
		Description: "Full-text search across tasks, documents and arch nodes. Hits are slim by default (no raw description; highlighted snippet only). Default limit 20 (max 100). Pass verbose=true to keep the raw description fallback.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, any, error) {
		pid, err := archProject(ctx, d, in.ProjectID)
		if err != nil {
			return toolError(err.Error())
		}
		var kinds []search.Kind
		for _, k := range in.Kinds {
			kinds = append(kinds, search.Kind(k))
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		hits, err := search.Search(ctx, d.Pool, in.Query, search.Filter{
			ProjectID: pid,
			Kinds:     kinds,
			Limit:     limit,
		})
		if err != nil {
			return toolError(err.Error())
		}
		if in.Verbose {
			return jsonResult(map[string]any{"hits": hits})
		}
		return jsonResult(map[string]any{"hits": toSlimHits(hits)})
	})
}
