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

func registerSkill(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.list",
		Description: "Lists every file currently available in the skill: embedded bundle and any per-organisation overrides stored under documents at scope=global, kind=skill, path starting with 'global/skills/'. Each entry has an 'origin' of 'embedded' or 'global'.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ SkillListInput) (*sdk.CallToolResult, any, error) {
		entries, err := skill.List(ctx, d.Pool)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"files": entries})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.read",
		Description: "Reads a skill file. The server first looks for an override at documents scope=global, kind=skill, path='global/skills/<path>'; if absent, returns the file embedded in the binary. Always start with 'skill.md'; load deeper files (under 'references/' and 'domains/') only when needed.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in SkillReadInput) (*sdk.CallToolResult, any, error) {
		data, _, err := skill.Read(ctx, d.Pool, in.Path)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(string(data))
	})
}
