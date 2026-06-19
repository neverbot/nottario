package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/skill"
)

// SkillListInput takes no arguments.
type SkillListInput struct{}

// SkillReadInput specifies which file to fetch.
type SkillReadInput struct {
	Path string `json:"path" jsonschema:"skill file path (e.g. 'skill.md', 'domains/tasks.md')"`
}

func registerSkill(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.list",
		Description: "Lists skill files: embedded bundle + per-org overrides at scope=global kind=skill. Each entry has {path, origin}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ SkillListInput) (*sdk.CallToolResult, any, error) {
		entries, err := skill.List(ctx, d.Pool)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"files": entries})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.read",
		Description: "Reads a skill file (override at scope=global kind=skill takes precedence over the embedded copy). Start with skill.md.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in SkillReadInput) (*sdk.CallToolResult, any, error) {
		data, _, err := skill.Read(ctx, d.Pool, in.Path)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(string(data))
	})
}
