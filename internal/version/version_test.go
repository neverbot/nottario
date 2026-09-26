package version

import (
	"strings"
	"testing"
)

// The version string is what the admin banner and /version report,
// and what a self-hoster quotes in a bug report. It has to carry all
// three build stamps, including the defaults of an unstamped build.
func TestString(t *testing.T) {
	t.Cleanup(func(v, c, d string) func() {
		return func() { Version, Commit, Date = v, c, d }
	}(Version, Commit, Date))

	if got := String(); got != "dev (none, unknown)" {
		t.Errorf("unstamped build reports %q", got)
	}

	Version, Commit, Date = "v1.2.3", "abc1234", "2026-09-26"
	got := String()
	for _, want := range []string{"v1.2.3", "abc1234", "2026-09-26"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from %q", want, got)
		}
	}
}
