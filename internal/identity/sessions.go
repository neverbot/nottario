package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionCookieName is the cookie used to carry the session id.
const SessionCookieName = "nottario_session"

// SessionTTL is the session lifetime. Sessions older than this are
// rejected and pruned.
const SessionTTL = 30 * 24 * time.Hour

// ErrSessionInvalid is returned when a cookie cannot be authenticated
// or the underlying row is missing/expired/revoked.
var ErrSessionInvalid = errors.New("session invalid")

// NewSession creates a new session row for the user.
func NewSession(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, ua, ip string) (*Session, error) {
	expiresAt := time.Now().Add(SessionTTL)
	var s Session
	var ipArg any = nil
	if ip != "" {
		ipArg = ip
	}
	err := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, created_at, last_seen_at, expires_at,
		          COALESCE(user_agent, ''), COALESCE(host(ip), '')
	`, userID, expiresAt, ua, ipArg).Scan(
		&s.ID, &s.UserID, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt,
		&s.UserAgent, &s.IP,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return &s, nil
}

// GetSession fetches an active session by id.
func GetSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Session, error) {
	var s Session
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, created_at, last_seen_at, expires_at,
		       COALESCE(user_agent, ''), COALESCE(host(ip), '')
		FROM sessions
		WHERE id = $1 AND expires_at > now()
	`, id).Scan(
		&s.ID, &s.UserID, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt,
		&s.UserAgent, &s.IP,
	)
	if err != nil {
		return nil, ErrSessionInvalid
	}
	return &s, nil
}

// TouchSession bumps last_seen_at; called from auth middleware.
func TouchSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, id)
	return err
}

// DeleteSession removes a session (used on logout).
func DeleteSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

// EncodeCookie produces a signed cookie value carrying the session id.
// Format: base64(uuid).hex(hmac).
func EncodeCookie(sessionID uuid.UUID, key []byte) string {
	idBytes, _ := sessionID.MarshalBinary()
	idPart := base64.RawURLEncoding.EncodeToString(idBytes)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(idPart))
	sig := hex.EncodeToString(mac.Sum(nil))
	return idPart + "." + sig
}

// DecodeCookie validates the signature and returns the session id.
func DecodeCookie(value string, key []byte) (uuid.UUID, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, ErrSessionInvalid
	}
	idPart, sigPart := parts[0], parts[1]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(idPart))
	expected := mac.Sum(nil)
	gotSig, err := hex.DecodeString(sigPart)
	if err != nil {
		return uuid.Nil, ErrSessionInvalid
	}
	if !hmac.Equal(expected, gotSig) {
		return uuid.Nil, ErrSessionInvalid
	}
	idBytes, err := base64.RawURLEncoding.DecodeString(idPart)
	if err != nil {
		return uuid.Nil, ErrSessionInvalid
	}
	id, err := uuid.FromBytes(idBytes)
	if err != nil {
		return uuid.Nil, ErrSessionInvalid
	}
	return id, nil
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, value string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

// ClearSessionCookie expires the cookie on the client.
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
