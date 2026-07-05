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

// IssueToken creates a new API token for the user inside the given
// project and returns the plaintext exactly once. Tokens are
// project-scoped: an agent presenting this token will only ever
// authenticate against the project it was minted in.
func IssueToken(ctx context.Context, pool *pgxpool.Pool, userID, projectID uuid.UUID, name string, defaultRoleID *uuid.UUID) (plaintext string, token *APIToken, err error) {
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
		ProjectID:     projectID,
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
		ProjectID:     row.ProjectID,
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

// ListProjectTokens returns every token issued for a project, ordered
// newest-first. Includes revoked tokens for audit display.
func ListProjectTokens(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]APIToken, error) {
	rows, err := dbq.New(pool).ListProjectTokens(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]APIToken, 0, len(rows))
	for _, r := range rows {
		out = append(out, APIToken{
			ID:            r.ID,
			UserID:        r.UserID,
			ProjectID:     r.ProjectID,
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

// UserTokenRow is a token row enriched with its project name for the
// cross-project `/me` tokens view. Separate type from APIToken because
// APIToken is used elsewhere and shouldn't grow project fields.
type UserTokenRow struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	ProjectName   string     `json:"project_name"`
	ProjectSlug   string     `json:"project_slug"`
	Name          string     `json:"name"`
	Prefix        string     `json:"prefix"`
	DefaultRoleID *uuid.UUID `json:"default_role_id"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
}

// ListUserTokens returns every token the given user has issued, across
// every project. Powers the cross-project tokens table under `/me`.
func ListUserTokens(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]UserTokenRow, error) {
	rows, err := dbq.New(pool).ListUserTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]UserTokenRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, UserTokenRow{
			ID:            r.ID,
			UserID:        r.UserID,
			ProjectID:     r.ProjectID,
			ProjectName:   r.ProjectName,
			ProjectSlug:   r.ProjectSlug,
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

// GetToken fetches a token by id. Returns ErrTokenInvalid if missing.
// Used by the revoke handler to validate the token belongs to the
// project in the URL before running the authz check.
func GetToken(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*APIToken, error) {
	row, err := dbq.New(pool).GetAPIToken(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	return &APIToken{
		ID:            row.ID,
		UserID:        row.UserID,
		ProjectID:     row.ProjectID,
		Name:          row.Name,
		Prefix:        row.Prefix,
		DefaultRoleID: row.DefaultRoleID,
		CreatedAt:     row.CreatedAt.Time,
		LastUsedAt:    timeOrNil(row.LastUsedAt),
		RevokedAt:     timeOrNil(row.RevokedAt),
	}, nil
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
		ProjectID:     r.ProjectID,
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
