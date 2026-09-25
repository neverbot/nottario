package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const fixturePkg = "./testdata/fixtures"

// wantedLines reads the fixture and returns the line numbers marked
// with a `// WANT` comment: the calls the analyser is supposed to
// catch. Reading them from the source keeps the expectations next to
// the code they describe instead of in a list that drifts.
func wantedLines(t *testing.T, path string) map[int]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	out := map[int]bool{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		// Suffix, not substring: the fixture's own prose mentions the
		// marker, and a test that trips over its own documentation is
		// a test nobody trusts.
		if strings.HasSuffix(strings.TrimSpace(sc.Text()), "// WANT") {
			out[line] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("the fixture marks no unsafe call — the test would pass on a analyser that does nothing")
	}
	return out
}

var reportLine = regexp.MustCompile(`^(.*\.go):(\d+):\d+: (.*)$`)

// The analyser guards every pgx call site in the repo against SQL
// injection, and CI trusts it. This pins both halves of its job: it
// reports the unsafe shapes, and it stays quiet on the safe ones,
// including the patterns this codebase uses on purpose.
func TestSqlcheck_FlagsUnsafeCallsAndNothingElse(t *testing.T) {
	var out bytes.Buffer
	violations, err := run(&out, fixturePkg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := wantedLines(t, filepath.Join("testdata", "fixtures", "fixtures.go"))
	got := map[int]string{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		m := reportLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparseable diagnostic: %q", line)
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("bad line number in %q", line)
		}
		got[n] = m[3]
	}

	if violations != len(got) {
		t.Errorf("returned %d violations but printed %d diagnostics", violations, len(got))
	}
	for line := range want {
		if _, ok := got[line]; !ok {
			t.Errorf("fixtures.go:%d is unsafe and was NOT flagged", line)
		}
	}
	for line, reason := range got {
		if !want[line] {
			t.Errorf("fixtures.go:%d was flagged but is safe: %s", line, reason)
		}
	}
}

// The repo's own code has to stay clean, and running the analyser
// over it also proves it works on a real package with real pgx types,
// not only on a handful of fixtures.
func TestSqlcheck_RepoIsClean(t *testing.T) {
	var out bytes.Buffer
	violations, err := run(&out, "../../../internal/...")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if violations != 0 {
		t.Errorf("sqlcheck reports %d violation(s) in internal/:\n%s", violations, out.String())
	}
}
