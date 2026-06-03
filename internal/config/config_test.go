package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validKey = "xK7p9q2L8mZv4N6r5T3sW8yA1b2C3d4E5f6G7h8I9j0="

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x:y@localhost/db")
	t.Setenv("SESSION_KEY", validKey)
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET", "client-secret")
}

func TestLoad_DefaultsApplied(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Errorf("PublicURL default = %q, want http://localhost:8080", cfg.PublicURL)
	}
	if !cfg.AuthEnabled {
		t.Error("AuthEnabled = false, want true with both github creds present")
	}
	if got := len(cfg.SessionKey); got < 32 {
		t.Errorf("SessionKey length = %d, want >= 32", got)
	}
	want, _ := base64.StdEncoding.DecodeString(validKey)
	if string(cfg.SessionKey) != string(want) {
		t.Error("SessionKey not decoded correctly")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestLoad_MissingSessionKey(t *testing.T) {
	setRequired(t)
	t.Setenv("SESSION_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when SESSION_KEY is empty")
	}
}

func TestLoad_SessionKeyNotBase64(t *testing.T) {
	setRequired(t)
	t.Setenv("SESSION_KEY", "not_base64!!!")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-base64 SESSION_KEY")
	}
}

func TestLoad_SessionKeyTooShort(t *testing.T) {
	setRequired(t)
	t.Setenv("SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("short")))
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short SESSION_KEY")
	}
}

func TestLoad_MissingGithubCreds(t *testing.T) {
	setRequired(t)
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when GITHUB_OAUTH_CLIENT_ID is empty")
	}
}

func TestLoad_HTTPAddrOverride(t *testing.T) {
	setRequired(t)
	t.Setenv("HTTP_ADDR", ":9999")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Errorf("HTTPAddr override not applied: %q", cfg.HTTPAddr)
	}
}

// writeSecretFile writes `content` to a fresh file under t.TempDir and
// returns its path. The file is cleaned up automatically by t.Cleanup.
func writeSecretFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoad_SessionKeyFile_TakesPrecedence(t *testing.T) {
	setRequired(t)
	// Plain env points at a deliberately-invalid value; the FILE
	// variant carries the real key. Loader must pick the FILE one.
	t.Setenv("SESSION_KEY", "not_base64!!!")
	t.Setenv("SESSION_KEY_FILE", writeSecretFile(t, "session_key", validKey))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want, _ := base64.StdEncoding.DecodeString(validKey)
	if string(cfg.SessionKey) != string(want) {
		t.Error("SESSION_KEY_FILE was not honoured (loader picked the plain env)")
	}
}

func TestLoad_SessionKeyFile_StripsTrailingNewline(t *testing.T) {
	setRequired(t)
	t.Setenv("SESSION_KEY", "")
	// Common when secrets are produced by `openssl rand -base64 32 >
	// session_key` — the file ends in `\n`.
	t.Setenv("SESSION_KEY_FILE", writeSecretFile(t, "session_key", validKey+"\n"))
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoad_SessionKeyFile_MissingFile(t *testing.T) {
	setRequired(t)
	t.Setenv("SESSION_KEY", "")
	t.Setenv("SESSION_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist"))
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when SESSION_KEY_FILE points at a missing file")
	}
	if !strings.Contains(err.Error(), "SESSION_KEY_FILE") {
		t.Errorf("error should mention SESSION_KEY_FILE for diagnosability, got: %v", err)
	}
}

func TestLoad_GithubSecretFile_TakesPrecedence(t *testing.T) {
	setRequired(t)
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET", "stale-value")
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET_FILE", writeSecretFile(t, "gh", "real-secret\n"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GithubClientSecret != "real-secret" {
		t.Errorf("GithubClientSecret = %q, want %q", cfg.GithubClientSecret, "real-secret")
	}
}
