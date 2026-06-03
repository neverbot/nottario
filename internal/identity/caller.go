package identity

import (
	"context"
	"fmt"

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

// ProjectScopeError is returned by RequireProjectScope when the
// caller's API token is bound to a different project than the one
// being targeted by the request.
type ProjectScopeError struct {
	TokenProjectID  uuid.UUID
	TargetProjectID uuid.UUID
}

func (e *ProjectScopeError) Error() string {
	return fmt.Sprintf("token scoped to project %s, request targets %s", e.TokenProjectID, e.TargetProjectID)
}

// RequireProjectScope enforces the per-token project boundary. When
// the caller authenticated with an API token, the token's ProjectID
// MUST match the target project; otherwise the request is rejected
// with a ProjectScopeError. Session callers (browser cookies) are not
// scoped and pass through.
//
// Admin tokens are NOT exempt: "is_admin" is an instance-wide
// concept; project scope is independent and stricter.
func RequireProjectScope(c Caller, target uuid.UUID) error {
	if c.Source != SourceToken {
		return nil
	}
	if c.ProjectID == target {
		return nil
	}
	return &ProjectScopeError{TokenProjectID: c.ProjectID, TargetProjectID: target}
}
