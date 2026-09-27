package web

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/skill"
)

// An agent running an outdated skill bundle reads outdated rules, and
// a bundle that predates the update check would never learn to run it.
// So the server itself tells every agent, as it connects, which bundle
// is current and how to compare it with what is installed.
func TestMCP_ConnectAnnouncesTheSkillBundleVersion(t *testing.T) {
	f := newMCPFixture(t, 13394, "skill-version-notice")
	want, err := skill.BundleVersion(f.ctx, f.pool.(*pgxpool.Pool))
	if err != nil {
		t.Fatalf("BundleVersion: %v", err)
	}

	init := f.session.InitializeResult()
	if init == nil {
		t.Fatal("no initialize result")
	}
	got := init.Instructions
	if !strings.Contains(got, want) {
		t.Errorf("instructions do not name the current bundle %s:\n%s", want, got)
	}
	// The instructions have to be actionable on their own, for an
	// agent whose installed skills know nothing about this check.
	for _, must := range []string{"SHA256SUMS", "nottario.skill.install", "restart"} {
		if !strings.Contains(got, must) {
			t.Errorf("instructions do not mention %q:\n%s", must, got)
		}
	}

	// whoami carries the same value, for clients that do not show the
	// server's instructions to the model.
	var me map[string]any
	f.callJSON(t, "nottario.whoami", map[string]any{}, &me)
	if me["skill_bundle_version"] != want {
		t.Errorf("whoami skill_bundle_version = %v, want %s", me["skill_bundle_version"], want)
	}
}
