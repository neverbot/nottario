package identity

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver wires the database and session key into the middleware
// constructors. A single Resolver is shared across the server.
type Resolver struct {
	Pool         *pgxpool.Pool
	SessionKey   []byte
	CookieSecure bool
}

// NewResolver constructs a Resolver.
func NewResolver(pool *pgxpool.Pool, sessionKey []byte, cookieSecure bool) *Resolver {
	return &Resolver{Pool: pool, SessionKey: sessionKey, CookieSecure: cookieSecure}
}

// ResolveSession reads the session cookie from r, validates and
// returns the Caller. The bool reports whether a valid session was
// found; on false, no error is returned (anonymous request).
func (rv *Resolver) ResolveSession(r *http.Request) (Caller, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Caller{}, false
	}
	id, err := DecodeCookie(c.Value, rv.SessionKey)
	if err != nil {
		return Caller{}, false
	}
	sess, err := GetSession(r.Context(), rv.Pool, id)
	if err != nil {
		return Caller{}, false
	}
	user, err := GetUser(r.Context(), rv.Pool, sess.UserID)
	if err != nil {
		return Caller{}, false
	}
	_ = TouchSession(r.Context(), rv.Pool, sess.ID)
	return Caller{
		UserID:    user.ID,
		IsAdmin:   user.IsAdmin,
		Source:    SourceSession,
		SessionID: sess.ID,
	}, true
}

// ResolveToken reads the Authorization: Bearer header from r and
// resolves it to a Caller. The bool reports whether a valid token
// was found.
func (rv *Resolver) ResolveToken(r *http.Request) (Caller, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return Caller{}, false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return Caller{}, false
	}
	plaintext := strings.TrimSpace(auth[len(prefix):])
	if plaintext == "" {
		return Caller{}, false
	}
	token, user, err := LookupToken(r.Context(), rv.Pool, plaintext)
	if err != nil {
		return Caller{}, false
	}
	c := Caller{
		UserID:  user.ID,
		IsAdmin: user.IsAdmin,
		Source:  SourceToken,
		TokenID: token.ID,
	}
	return c, true
}
