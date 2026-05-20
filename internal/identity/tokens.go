package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// TokenPrefix is the textual prefix prepended to every plaintext
// token so that "is this thing a nottario token?" is obvious in
// logs and config files.
const TokenPrefix = "ntr_"

// ErrTokenInvalid is returned by LookupToken for any non-match.
var ErrTokenInvalid = errors.New("token invalid")

// IssueToken creates a new API token for the user and returns the
// plaintext exactly once.
func IssueToken(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, name string, defaultRoleID *uuid.UUID) (plaintext string, token *APIToken, err error) {
	raw := make([]byte, 24) // 24 random bytes -> 32 base64 chars
	if _, err = rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("read random: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	plaintext = TokenPrefix + body
	hash := sha256.Sum256([]byte(plaintext))
	prefix := plaintext[:min(12, len(plaintext))]

	row, err := dbq.New(pool).InsertAPIToken(ctx, dbq.InsertAPITokenParams{
		UserID:        userID,
		Name:          name,
		TokenHash:     hash[:],
		Prefix:        prefix,
		DefaultRoleID: defaultRoleID,
	})
	if err != nil {
		return "", nil, fmt.Errorf("insert token: %w", err)
	}
	return plaintext, tokenFromInsertRow(row), nil
}

// LookupToken validates a plaintext token, returns the (token, user)
// pair and bumps last_used_at. Revoked tokens are rejected.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, plaintext string) (*APIToken, *User, error) {
	hash := sha256.Sum256([]byte(plaintext))
	q := dbq.New(pool)
	row, err := q.LookupAPIToken(ctx, hash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lookup token: %w", err)
	}
	_ = q.TouchTokenLastUsed(ctx, row.TokenID)
	t := APIToken{
		ID:            row.TokenID,
		UserID:        row.UserID,
		Name:          row.TokenName,
		Prefix:        row.Prefix,
		DefaultRoleID: row.DefaultRoleID,
		CreatedAt:     row.TokenCreatedAt.Time,
		LastUsedAt:    timeOrNil(row.LastUsedAt),
		RevokedAt:     timeOrNil(row.RevokedAt),
	}
	u := User{
		ID:          row.UserIDFull,
		GithubLogin: row.GithubLogin,
		GithubID:    row.GithubID,
		DisplayName: row.DisplayName,
		AvatarURL:   row.AvatarUrl,
		IsAdmin:     row.IsAdmin,
		CreatedAt:   row.UserCreatedAt.Time,
		LastSeenAt:  timeOrNil(row.LastSeenAt),
	}
	return &t, &u, nil
}

// ListTokens returns the tokens owned by a user.
func ListTokens(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]APIToken, error) {
	rows, err := dbq.New(pool).ListUserTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]APIToken, 0, len(rows))
	for _, r := range rows {
		out = append(out, APIToken{
			ID:            r.ID,
			UserID:        r.UserID,
			Name:          r.Name,
			Prefix:        r.Prefix,
			DefaultRoleID: r.DefaultRoleID,
			CreatedAt:     r.CreatedAt.Time,
			LastUsedAt:    timeOrNil(r.LastUsedAt),
			RevokedAt:     timeOrNil(r.RevokedAt),
		})
	}
	return out, nil
}

// RevokeToken marks a token revoked. Only the owner or an admin may
// revoke; this is enforced by passing the requesting caller's
// (userID, isAdmin) pair.
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, requestingUserID uuid.UUID, isAdmin bool) error {
	rows, err := dbq.New(pool).RevokeAPIToken(ctx, dbq.RevokeAPITokenParams{
		ID:              id,
		IsAdmin:         isAdmin,
		RequesterUserID: requestingUserID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTokenInvalid
	}
	return nil
}

func tokenFromInsertRow(r dbq.InsertAPITokenRow) *APIToken {
	return &APIToken{
		ID:            r.ID,
		UserID:        r.UserID,
		Name:          r.Name,
		Prefix:        r.Prefix,
		DefaultRoleID: r.DefaultRoleID,
		CreatedAt:     r.CreatedAt.Time,
		LastUsedAt:    timeOrNil(r.LastUsedAt),
		RevokedAt:     timeOrNil(r.RevokedAt),
	}
}

func timeOrNil(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	v := ts.Time
	return &v
}
