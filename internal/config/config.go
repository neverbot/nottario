// Package config loads runtime configuration from environment
// variables. All values are validated up-front; the binary refuses
// to start with an incomplete configuration.
package config

import (
	"errors"
	"os"
)

// Config holds all process-wide configuration.
type Config struct {
	HTTPAddr    string
	PublicURL   string
	DatabaseURL string
}

// Load reads configuration from the process environment.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		PublicURL:   getenv("PUBLIC_URL", "http://localhost:8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
