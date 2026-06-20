package mcp

import (
	"context"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/skill"
)

// SkillInstallInput is intentionally empty: the install descriptor
// applies to the bundle as a whole. Callers receive a time-bound
// download URL and the install instructions.
type SkillInstallInput struct{}

// skillInstallTTL is the lifetime of the signed download URL returned
// by `nottario.skill.install`. Generous enough to absorb retries on a
// slow link; short enough that a leaked URL stops being useful before
// any realistic attacker reaches it.
const skillInstallTTL = 5 * time.Minute

// skillInstallInstructions is the canonical "what do I do with this"
// prose returned alongside the download URL. We embed it in the
// response (rather than relying on the agent having read the skill
// bundle first) so that the very first install — when the bundle is
// not yet on disk — still knows what to do.
const skillInstallInstructions = "Fetch the zip from download_url using any HTTP tool you have (curl, wget, Invoke-WebRequest, Python urllib, Node fetch — pick whatever works on the host). Extract its contents into preferred_dir, overwriting existing files. If you can't write there, fall back to fallback_dir. Create the directory if it doesn't exist. The bundle is loaded by the client (Claude Code, etc.) at session start, so a session restart is required for the new skills to take effect. The download_url is signed and expires in 5 minutes."

func registerSkill(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.install",
		Description: "Returns a signed download URL for the Nottario skill bundle as a single zip, plus install instructions. The agent fetches the URL out-of-band with any HTTP tool it has, unzips into the client's skill directory, and restarts the client to pick up the new bundle. The bundle content does NOT flow through this tool's response — the URL is the payload.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, _ SkillInstallInput) (*sdk.CallToolResult, any, error) {
		version, err := skill.BundleVersion(ctx, d.Pool)
		if err != nil {
			return toolError(err.Error())
		}
		// Build an absolute URL pointing at this same server. The MCP
		// transport carries the original request URL in the context so
		// we reuse it; falling back to a relative URL if not available.
		base := skillZipBaseURL(ctx)
		signed := base
		if d.SessionKey != nil {
			signed = skill.SignZipURL(base, d.SessionKey, skillInstallTTL)
		}
		return jsonResult(map[string]any{
			"download_url":   signed,
			"format":         "zip",
			"bundle_version": version,
			"install": map[string]any{
				"name":          "nottario",
				"preferred_dir": "<workspace>/.claude/skills/nottario",
				"fallback_dir":  "~/.claude/skills/nottario",
				"instructions":  skillInstallInstructions,
			},
		})
	})
}

// skillZipBaseURL returns an absolute URL pointing at /skill.zip on
// the server handling this MCP request. We look at the originating
// HTTP request the SDK stashed in the context; if it's not there
// (e.g. a unit test bypassing the streamable transport), we return a
// path-only URL — the agent's HTTP client will resolve it against the
// MCP base.
func skillZipBaseURL(ctx context.Context) string {
	if base := externalBaseURL(ctx); base != "" {
		return strings.TrimRight(base, "/") + "/skill.zip"
	}
	return "/skill.zip"
}
