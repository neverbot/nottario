package config

import (
	"encoding/base64"
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
