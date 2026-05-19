package identity

import (
	"context"

	"github.com/google/uuid"
)

// Source identifies how a caller authenticated.
type Source string

const (
	// SourceSession means the caller authenticated with a session cookie.
	SourceSession Source = "session"
	// SourceToken means the caller authenticated with an API token.
	SourceToken Source = "token"
)

// Caller is the authenticated principal attached to a request.
type Caller struct {
	UserID    uuid.UUID
	IsAdmin   bool
	ProjectID uuid.UUID // active project for this request; zero value if none
	Roles     []uuid.UUID
	Source    Source
	TokenID   uuid.UUID // populated when Source == SourceToken
	SessionID uuid.UUID // populated when Source == SourceSession
}

type callerCtxKey struct{}

// WithCaller stores the caller in the request context.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerCtxKey{}, c)
}

// CallerFromContext extracts the caller. The bool reports presence.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerCtxKey{}).(Caller)
	return c, ok
}
