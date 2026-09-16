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
const skillInstallInstructions = "First check whether you already have this bundle: hash the SHA256SUMS file inside the installed directory (sha256sum <dir>/SHA256SUMS, or shasum -a 256) and compare \"sha256:<hex>\" with bundle_version. Equal means up to date: stop, nothing to download or restart. bundle_version is NOT a hash of the zip or of any single skill file; only SHA256SUMS. If it differs or SHA256SUMS is missing, fetch the zip from download_url using any HTTP tool you have (curl, wget, Invoke-WebRequest, Python urllib, Node fetch). Replace the contents of preferred_dir with the zip contents (files not listed in SHA256SUMS are leftovers of an older bundle). If you can't write there, fall back to fallback_dir. Create the directory if it doesn't exist. Optionally verify with sha256sum -c SHA256SUMS run inside the directory. The client loads skills at session start, so tell the human to restart it for the new skills to take effect. The download_url is signed and expires in 5 minutes."

func registerSkill(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.skill.install",
		Description: "Returns {download_url, format, bundle_version, install} for the skill bundle zip. bundle_version = \"sha256:\" + SHA-256 of the SHA256SUMS file shipped inside the zip: compare it with that file in your installed directory and skip the download when equal. Otherwise fetch the URL (5-min HMAC TTL, no Authorization needed), unzip into install.preferred_dir, and remind the human to restart their client. Bundle bytes never traverse the MCP response.",
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
				"manifest":      skill.ManifestName,
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
