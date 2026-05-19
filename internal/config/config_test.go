package config

import (
	"testing"
)

func TestLoad_DefaultsApplied(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:y@localhost/db")
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
	if cfg.DatabaseURL != "postgres://x:y@localhost/db" {
		t.Errorf("DatabaseURL not picked up: %q", cfg.DatabaseURL)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no DATABASE_URL should fail")
	}
}

func TestLoad_HTTPAddrOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:y@localhost/db")
	t.Setenv("HTTP_ADDR", ":9999")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Errorf("HTTPAddr override not applied: %q", cfg.HTTPAddr)
	}
}
