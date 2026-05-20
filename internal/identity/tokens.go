package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

	var t APIToken
	err = pool.QueryRow(ctx, `
		INSERT INTO api_tokens (user_id, name, token_hash, prefix, default_role_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, prefix, default_role_id,
		          created_at, last_used_at, revoked_at
	`, userID, name, hash[:], prefix, defaultRoleID).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.DefaultRoleID,
		&t.CreatedAt, &t.LastUsedAt, &t.RevokedAt,
	)
	if err != nil {
		return "", nil, fmt.Errorf("insert token: %w", err)
	}
	return plaintext, &t, nil
}

// LookupToken validates a plaintext token, returns the (token, user)
// pair and bumps last_used_at. Revoked tokens are rejected.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, plaintext string) (*APIToken, *User, error) {
	hash := sha256.Sum256([]byte(plaintext))
	var t APIToken
	var u User
	err := pool.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.name, t.prefix, t.default_role_id,
		       t.created_at, t.last_used_at, t.revoked_at,
		       u.id, u.github_login, u.github_id, u.display_name,
		       COALESCE(u.avatar_url, ''), u.is_admin, u.created_at, u.last_seen_at
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL
	`, hash[:]).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.DefaultRoleID,
		&t.CreatedAt, &t.LastUsedAt, &t.RevokedAt,
		&u.ID, &u.GithubLogin, &u.GithubID, &u.DisplayName,
		&u.AvatarURL, &u.IsAdmin, &u.CreatedAt, &u.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lookup token: %w", err)
	}
	_, _ = pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE id = $1`, t.ID)
	return &t, &u, nil
}

// ListTokens returns the tokens owned by a user.
func ListTokens(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]APIToken, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, name, prefix, default_role_id,
		       created_at, last_used_at, revoked_at
		FROM api_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.DefaultRoleID,
			&t.CreatedAt, &t.LastUsedAt, &t.RevokedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken marks a token revoked. Only the owner or an admin may
// revoke; this is enforced by passing the requesting caller's
// (userID, isAdmin) pair.
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, requestingUserID uuid.UUID, isAdmin bool) error {
	cmd, err := pool.Exec(ctx, `
		UPDATE api_tokens
		SET revoked_at = now()
		WHERE id = $1
		  AND revoked_at IS NULL
		  AND ($2::bool OR user_id = $3)
	`, id, isAdmin, requestingUserID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrTokenInvalid
	}
	return nil
}
