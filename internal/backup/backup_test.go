package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// dumpOnce intentionally has no unit test: it shells out to pg_dump
// and requires a real Postgres reachable at Config.DatabaseURL. The
// integration path is exercised by the operator running the binary
// against the dev compose stack.

func TestParseClock(t *testing.T) {
	cases := []struct {
		in       string
		wantH    int
		wantM    int
		wantErr  bool
		errLabel string
	}{
		{in: "03:00", wantH: 3, wantM: 0},
		{in: "00:00", wantH: 0, wantM: 0},
		{in: "23:59", wantH: 23, wantM: 59},
		{in: "9:05", wantH: 9, wantM: 5},
		{in: "", wantErr: true, errLabel: "empty"},
		{in: "0300", wantErr: true, errLabel: "no colon"},
		{in: "ab:cd", wantErr: true, errLabel: "non-numeric"},
		{in: "24:00", wantErr: true, errLabel: "H out of range"},
		{in: "12:60", wantErr: true, errLabel: "M out of range"},
		{in: "1:2:3", wantErr: true, errLabel: "too many parts"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			h, m, err := parseClock(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q (%s), got h=%d m=%d", tc.in, tc.errLabel, h, m)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseClock(%q): %v", tc.in, err)
			}
			if h != tc.wantH || m != tc.wantM {
				t.Errorf("parseClock(%q) = (%d,%d), want (%d,%d)", tc.in, h, m, tc.wantH, tc.wantM)
			}
		})
	}
}

func TestNextFire_LaterToday(t *testing.T) {
	now := time.Date(2026, 6, 7, 1, 30, 0, 0, time.Local)
	got := nextFire(now, 3, 0)
	want := time.Date(2026, 6, 7, 3, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("nextFire(01:30, 03:00) = %v, want %v", got, want)
	}
}

func TestNextFire_EarlierToday_RollsToTomorrow(t *testing.T) {
	now := time.Date(2026, 6, 7, 4, 30, 0, 0, time.Local)
	got := nextFire(now, 3, 0)
	want := time.Date(2026, 6, 8, 3, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("nextFire(04:30, 03:00) = %v, want %v", got, want)
	}
}

func TestNextFire_ExactlyNow_RollsToTomorrow(t *testing.T) {
	now := time.Date(2026, 6, 7, 3, 0, 0, 0, time.Local)
	got := nextFire(now, 3, 0)
	want := time.Date(2026, 6, 8, 3, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("nextFire(equal-now) = %v, want %v", got, want)
	}
}

func TestPruneOldDumps_DeletesOldKeepsNew(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.Local)
	old := filepath.Join(dir, "nottario-2026-05-01-0300.dump")
	fresh := filepath.Join(dir, "nottario-2026-06-06-0300.dump")
	other := filepath.Join(dir, "not-a-dump.txt")
	wrongPattern := filepath.Join(dir, "nottario-backup.dump")
	for _, p := range []string{old, fresh, other, wrongPattern} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Age `old` to 30 days ago, fresh to 1 day ago, leave the
	// non-matching files untouched (we still expect them to survive).
	if err := os.Chtimes(old, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(fresh, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("chtimes fresh: %v", err)
	}
	if err := os.Chtimes(other, now.Add(-365*24*time.Hour), now.Add(-365*24*time.Hour)); err != nil {
		t.Fatalf("chtimes other: %v", err)
	}
	if err := os.Chtimes(wrongPattern, now.Add(-365*24*time.Hour), now.Add(-365*24*time.Hour)); err != nil {
		t.Fatalf("chtimes wrongPattern: %v", err)
	}

	if err := pruneOldDumps(dir, 7, now); err != nil {
		t.Fatalf("pruneOldDumps: %v", err)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old dump to be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected fresh dump to survive, got err = %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("expected non-dump file to survive, got err = %v", err)
	}
	if _, err := os.Stat(wrongPattern); err != nil {
		t.Errorf("expected non-matching dump-ish file to survive, got err = %v", err)
	}
}

func TestPruneOldDumps_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := pruneOldDumps(dir, 7, time.Now()); err != nil {
		t.Errorf("pruneOldDumps on empty dir: %v", err)
	}
}
