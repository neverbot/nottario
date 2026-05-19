// Package config loads runtime configuration from environment
// variables. All values are validated up-front; the binary refuses
// to start with an incomplete configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"

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

	cfg := &Config{
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		PublicURL:          getenv("PUBLIC_URL", "http://localhost:8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GithubClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
		GithubClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	rawKey := os.Getenv("SESSION_KEY")
	if rawKey == "" {
		return nil, errors.New("SESSION_KEY is required (32 random bytes, base64-encoded)")
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
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

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
