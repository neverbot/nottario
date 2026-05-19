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
	Path string `json:"path" jsonschema:"relative path to a skill file, e.g. 'skill.md', 'references/identity.md', 'domains/tasks.md'"`
}

func registerSkill(server *sdk.Server, _ Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.list",
		Description: "Lists every file in the skill bundle. Use this to discover deeper guides before reading them.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ SkillListInput) (*sdk.CallToolResult, any, error) {
		files, err := skill.List()
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"files": files})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.read",
		Description: "Reads a skill file and returns its markdown content. Always start with 'skill.md'; load deeper files (under 'references/' and 'domains/') only when needed.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in SkillReadInput) (*sdk.CallToolResult, any, error) {
		data, err := skill.Read(in.Path)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(string(data))
	})
}
