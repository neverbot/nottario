// Package mcp exposes Nottario over the Model Context Protocol so that
// agents can read and write tasks (and, in later milestones, markdown
// documents and the architecture graph). Authentication is by API
// token; every tool call carries an explicit project_id where one is
// required (no per-session "active project" state).
package mcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/neverbot/nottario/internal/identity"
)

type callerCtxKey struct{}

// withCaller stores the resolved caller in ctx for tool handlers to read.
func withCaller(ctx context.Context, c identity.Caller) context.Context {
	return context.WithValue(ctx, callerCtxKey{}, c)
}

// callerFromContext retrieves the caller. Tool handlers use this to
// authorise and to record authorship.
func callerFromContext(ctx context.Context) (identity.Caller, error) {
	c, ok := ctx.Value(callerCtxKey{}).(identity.Caller)
	if !ok {
		return identity.Caller{}, errors.New("no caller in context (auth middleware did not run)")
	}
	return c, nil
}

type baseURLCtxKey struct{}

// withExternalBaseURL stores the public-facing base URL of the
// incoming HTTP request in ctx so tool handlers can compose absolute
// URLs that an out-of-band HTTP client will actually be able to
// fetch.
func withExternalBaseURL(ctx context.Context, base string) context.Context {
	if base == "" {
		return ctx
	}
	return context.WithValue(ctx, baseURLCtxKey{}, base)
}

// externalBaseURL returns the public-facing base URL the MCP request
// came in on (scheme + host + port), without any path. Empty when
// the middleware hasn't run (unit tests bypassing the streamable
// transport, etc.).
func externalBaseURL(ctx context.Context) string {
	s, _ := ctx.Value(baseURLCtxKey{}).(string)
	return s
}

// externalBaseURLFromRequest reconstructs the scheme + host the
// client used to reach us. Honours X-Forwarded-Proto / X-Forwarded-
// Host when present (Traefik, Caddy, etc.); otherwise inferred from
// r.TLS and r.Host.
func externalBaseURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		scheme = v
	}
	host := r.Host
	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		host = v
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}
