// Package config loads runtime configuration from environment
// variables. All values are validated up-front; the binary refuses
// to start with an incomplete configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"time"

	"github.com/joho/godotenv"
)

// Config holds all process-wide configuration.
type Config struct {
	HTTPAddr    string
	PublicURL   string
	DatabaseURL string

	// AuthEnabled is true when GitHub OAuth credentials and a session
	// key are all present. When false, the auth-protected endpoints
	// return 503; this is only meant for local hacking, not for any
	// real deployment.
	AuthEnabled        bool
	SessionKey         []byte
	GithubClientID     string
	GithubClientSecret string

	// GithubOrg, when set, restricts OAuth logins to members of this
	// GitHub organisation. Empty disables the gate (anyone with a
	// GitHub account can sign in). API tokens are unaffected.
	GithubOrg string

	// Backup configuration. BackupDir empty disables the in-process
	// pg_dump goroutine entirely.
	BackupDir      string
	BackupAt       string // "HH:MM" 24h local time.
	BackupKeepDays int

	// Arch versioning. ArchLockIdleSeconds is the global default
	// idle threshold after which an open editing session is auto-
	// flushed into a revision; per-project rows can override via
	// projects.arch_lock_idle_seconds. ArchTickSeconds is the
	// interval at which the background flush goroutine scans for
	// expired locks.
	ArchLockIdleSeconds int
	ArchTickSeconds     int

	// Self-update poller. When SelfUpdateEnabled is true (the
	// default), an in-process goroutine polls the upstream GitHub
	// repository once every SelfUpdateInterval and exposes the
	// result on /api/version/status so an admin banner can surface
	// "a newer image is available". Set SELF_UPDATE_CHECK_ENABLED=
	// false to skip the outbound request entirely (air-gapped or
	// privacy-conscious deployments).
	SelfUpdateEnabled  bool
	SelfUpdateInterval time.Duration
	SelfUpdateUpstream string

	// Notifications system. When NotificationsEnabled is true (the
	// default) the app produces per-user notifications for the
	// events documented in /self-hosting and exposes the drawer +
	// preferences UI. When false the bell disappears, endpoints
	// refuse writes and return empty reads, and no rows are
	// inserted — useful for stripped-down deployments that don't
	// want the persistence overhead.
	NotificationsEnabled bool
}

// LoadDotEnv reads a .env file from the working directory if it
// exists. Missing file is not an error. Existing env vars are not
// overridden.
func LoadDotEnv() {
	_ = godotenv.Load()
}

// Load reads configuration from the process environment, loading a
// .env file in the working directory first when present.
func Load() (*Config, error) {
	LoadDotEnv()

	clientSecret, err := getSecret("GITHUB_OAUTH_CLIENT_SECRET")
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		PublicURL:          getenv("PUBLIC_URL", "http://localhost:8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GithubClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
		GithubClientSecret: clientSecret,
		GithubOrg:          strings.TrimSpace(os.Getenv("GITHUB_OAUTH_ORG")),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	rawKey, err := getSecret("SESSION_KEY")
	if err != nil {
		return nil, err
	}
	if rawKey == "" {
		return nil, errors.New("SESSION_KEY is required (32 random bytes, base64-encoded)")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rawKey))
	if err != nil {
		return nil, fmt.Errorf("SESSION_KEY is not valid base64: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("SESSION_KEY must decode to at least 32 bytes, got %d", len(key))
	}
	cfg.SessionKey = key

	cfg.AuthEnabled = cfg.GithubClientID != "" && cfg.GithubClientSecret != ""
	if !cfg.AuthEnabled {
		return nil, errors.New("GITHUB_OAUTH_CLIENT_ID and GITHUB_OAUTH_CLIENT_SECRET are required")
	}

	cfg.BackupDir = os.Getenv("NOTTARIO_BACKUP_DIR")
	cfg.BackupAt = getenv("NOTTARIO_BACKUP_AT", "03:00")
	days := 7
	if s := os.Getenv("NOTTARIO_BACKUP_KEEP_DAYS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("NOTTARIO_BACKUP_KEEP_DAYS must be a positive integer, got %q", s)
		}
		days = v
	}
	cfg.BackupKeepDays = days

	cfg.ArchLockIdleSeconds = 120
	if s := os.Getenv("NOTTARIO_ARCH_LOCK_IDLE_SECONDS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("NOTTARIO_ARCH_LOCK_IDLE_SECONDS must be a positive integer, got %q", s)
		}
		cfg.ArchLockIdleSeconds = v
	}
	cfg.ArchTickSeconds = 30
	if s := os.Getenv("NOTTARIO_ARCH_TICK_SECONDS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("NOTTARIO_ARCH_TICK_SECONDS must be a positive integer, got %q", s)
		}
		cfg.ArchTickSeconds = v
	}

	cfg.SelfUpdateEnabled = getenvBool("SELF_UPDATE_CHECK_ENABLED", true)
	cfg.SelfUpdateInterval = 24 * time.Hour
	if s := os.Getenv("SELF_UPDATE_CHECK_INTERVAL"); s != "" {
		v, err := time.ParseDuration(s)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("SELF_UPDATE_CHECK_INTERVAL must be a positive Go duration, got %q", s)
		}
		cfg.SelfUpdateInterval = v
	}
	cfg.SelfUpdateUpstream = getenv("SELF_UPDATE_UPSTREAM", "neverbot/nottario")

	cfg.NotificationsEnabled = getenvBool("NOTIFICATIONS_ENABLED", true)

	return cfg, nil
}

// getenvBool parses truthy env values. Accepts "1"/"true"/"yes"/"on"
// as true and "0"/"false"/"no"/"off" as false, case-insensitive.
// Anything else (including empty) returns fallback so a typo does
// not silently flip a default.
func getenvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// getSecret reads a secret from a `<name>_FILE` env pointing at a file
// (12-factor / Docker secrets convention) and falls back to the plain
// `<name>` env. Trailing whitespace from the file is stripped — common
// when secrets are written with `echo …` and end up with a newline.
// `_FILE` takes precedence when both are set so an operator can wire a
// secret-mount without touching the value var.
func getSecret(name string) (string, error) {
	if path := os.Getenv(name + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE %q: %w", name, path, err)
		}
		return strings.TrimRight(string(b), "\r\n\t "), nil
	}
	return os.Getenv(name), nil
}
