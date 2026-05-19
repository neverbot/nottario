// Package mcp exposes Nottario over the Model Context Protocol so that
// agents can read and write tasks (and, in later milestones, markdown
// documents and the architecture graph). Authentication is by API
// token; every tool call carries an explicit project_id where one is
// required (no per-session "active project" state).
package mcp

import (
	"context"
	"errors"

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
