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
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
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
	var ipAddr *netip.Addr
	if ip != "" {
		if a, err := netip.ParseAddr(ip); err == nil {
			ipAddr = &a
		}
	}
	row, err := dbq.New(pool).InsertSession(ctx, dbq.InsertSessionParams{
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		UserAgent: pgtype.Text{String: ua, Valid: ua != ""},
		Ip:        ipAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return &Session{
		ID:         row.ID,
		UserID:     row.UserID,
		CreatedAt:  row.CreatedAt.Time,
		LastSeenAt: row.LastSeenAt.Time,
		ExpiresAt:  row.ExpiresAt.Time,
		UserAgent:  row.UserAgent,
		IP:         row.Ip,
	}, nil
}

// GetSession fetches an active session by id.
func GetSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Session, error) {
	row, err := dbq.New(pool).GetActiveSession(ctx, id)
	if err != nil {
		return nil, ErrSessionInvalid
	}
	return &Session{
		ID:         row.ID,
		UserID:     row.UserID,
		CreatedAt:  row.CreatedAt.Time,
		LastSeenAt: row.LastSeenAt.Time,
		ExpiresAt:  row.ExpiresAt.Time,
		UserAgent:  row.UserAgent,
		IP:         row.Ip,
	}, nil
}

// TouchSession bumps last_seen_at; called from auth middleware.
func TouchSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	return dbq.New(pool).TouchSessionLastSeen(ctx, id)
}

// DeleteSession removes a session (used on logout).
func DeleteSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	return dbq.New(pool).DeleteSessionByID(ctx, id)
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
