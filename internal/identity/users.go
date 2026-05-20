package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpsertFromGithub inserts or updates a user from the GitHub
// profile. The bool result is true when the user did not exist
// before; the caller uses that to apply the "first user becomes
// admin" rule.
func UpsertFromGithub(ctx context.Context, pool *pgxpool.Pool, githubID int64, login, displayName, avatarURL string) (*User, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u User
	var created bool
	err = tx.QueryRow(ctx, `
		SELECT id, github_login, github_id, display_name,
		       COALESCE(avatar_url, ''), is_admin, created_at, last_seen_at
		FROM users
		WHERE github_id = $1
	`, githubID).Scan(
		&u.ID, &u.GithubLogin, &u.GithubID, &u.DisplayName,
		&u.AvatarURL, &u.IsAdmin, &u.CreatedAt, &u.LastSeenAt,
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Brand-new user.
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			return nil, false, fmt.Errorf("count users: %w", err)
		}
		isAdmin := count == 0
		err = tx.QueryRow(ctx, `
			INSERT INTO users (github_login, github_id, display_name, avatar_url, is_admin)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, github_login, github_id, display_name,
			          COALESCE(avatar_url, ''), is_admin, created_at, last_seen_at
		`, login, githubID, displayName, avatarURL, isAdmin).Scan(
			&u.ID, &u.GithubLogin, &u.GithubID, &u.DisplayName,
			&u.AvatarURL, &u.IsAdmin, &u.CreatedAt, &u.LastSeenAt,
		)
		if err != nil {
			return nil, false, fmt.Errorf("insert user: %w", err)
		}
		created = true
	case err != nil:
		return nil, false, fmt.Errorf("select user: %w", err)
	default:
		// Update mutable display fields from GitHub.
		_, err = tx.Exec(ctx, `
			UPDATE users SET display_name = $2, avatar_url = $3, github_login = $4
			WHERE id = $1
		`, u.ID, displayName, avatarURL, login)
		if err != nil {
			return nil, false, fmt.Errorf("update user: %w", err)
		}
		u.DisplayName = displayName
		u.AvatarURL = avatarURL
		u.GithubLogin = login
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit: %w", err)
	}
	return &u, created, nil
}

// GetUser fetches a user by id.
func GetUser(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*User, error) {
	var u User
	err := pool.QueryRow(ctx, `
		SELECT id, github_login, github_id, display_name,
		       COALESCE(avatar_url, ''), is_admin, created_at, last_seen_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.GithubLogin, &u.GithubID, &u.DisplayName,
		&u.AvatarURL, &u.IsAdmin, &u.CreatedAt, &u.LastSeenAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchUserSeen records the user's most recent activity.
func TouchUserSeen(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `UPDATE users SET last_seen_at = now() WHERE id = $1`, id)
	return err
}
