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

	// Backup configuration. BackupDir empty disables the in-process
	// pg_dump goroutine entirely.
	BackupDir      string
	BackupAt       string // "HH:MM" 24h local time.
	BackupKeepDays int
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

	return cfg, nil
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
