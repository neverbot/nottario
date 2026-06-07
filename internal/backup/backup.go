// Package backup runs periodic pg_dump backups in-process.
//
// Disabled when Config.Dir is empty. When enabled, a single goroutine
// sleeps until the next NOTTARIO_BACKUP_AT slot in local time, shells
// out to pg_dump --format=custom against Config.DatabaseURL, writes
// the dump atomically, and prunes files older than KeepDays. Errors
// are logged and the loop continues; the goroutine never returns
// while ctx is alive.
package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config holds the inputs for Run. Dir empty disables backups.
type Config struct {
	Dir         string
	DatabaseURL string
	At          string // "HH:MM"
	KeepDays    int
	Logger      *slog.Logger
	// Now allows tests to inject time.
	Now func() time.Time
}

// FilePrefix and FileSuffix bracket every dump filename.
const (
	FilePrefix = "nottario-"
	FileSuffix = ".dump"
)

// Run blocks until ctx is cancelled. Returns nil immediately if
// Config.Dir is empty. Safe to call once from main().
func Run(ctx context.Context, c Config) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Dir == "" {
		c.Logger.Info("backups disabled (NOTTARIO_BACKUP_DIR not set)")
		return nil
	}
	if c.At == "" {
		c.At = "03:00"
	}
	if c.KeepDays <= 0 {
		c.KeepDays = 7
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	h, m, err := parseClock(c.At)
	if err != nil {
		return fmt.Errorf("parse NOTTARIO_BACKUP_AT: %w", err)
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return fmt.Errorf("mkdir backup dir: %w", err)
	}
	c.Logger.Info("backups enabled", "dir", c.Dir, "at", c.At, "keep_days", c.KeepDays)
	for {
		next := nextFire(c.Now(), h, m)
		wait := time.Until(next)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		if err := dumpOnce(ctx, c); err != nil {
			c.Logger.Error("backup failed", "err", err)
			continue
		}
		if err := pruneOldDumps(c.Dir, c.KeepDays, c.Now()); err != nil {
			c.Logger.Warn("backup prune failed", "err", err)
		}
	}
}

func parseClock(s string) (int, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("expected HH:MM")
	}
	h, herr := strconv.Atoi(parts[0])
	m, merr := strconv.Atoi(parts[1])
	if herr != nil || merr != nil {
		return 0, 0, errors.New("non-numeric HH or MM")
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, errors.New("HH must be 0-23, MM must be 0-59")
	}
	return h, m, nil
}

func nextFire(now time.Time, h, m int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

func dumpOnce(ctx context.Context, c Config) error {
	now := c.Now()
	name := fmt.Sprintf("%s%s%s", FilePrefix, now.Format("2006-01-02-1504"), FileSuffix)
	tmp := filepath.Join(c.Dir, name+".tmp")
	final := filepath.Join(c.Dir, name)
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--file="+tmp, c.DatabaseURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("pg_dump: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	c.Logger.Info("backup written", "file", final)
	return nil
}

var dumpNameRe = regexp.MustCompile(`^` + FilePrefix + `\d{4}-\d{2}-\d{2}-\d{4}` + regexp.QuoteMeta(FileSuffix) + `$`)

func pruneOldDumps(dir string, keepDays int, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-time.Duration(keepDays) * 24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() || !dumpNameRe.MatchString(e.Name()) {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
