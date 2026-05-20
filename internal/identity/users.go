package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
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
	q := dbq.New(tx)

	existing, err := q.GetUserByGithubID(ctx, githubID)
	var (
		u       User
		created bool
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		count, err := q.CountUsers(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("count users: %w", err)
		}
		isAdmin := count == 0
		row, err := q.InsertUser(ctx, dbq.InsertUserParams{
			GithubLogin: login,
			GithubID:    githubID,
			DisplayName: displayName,
			AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
			IsAdmin:     isAdmin,
		})
		if err != nil {
			return nil, false, fmt.Errorf("insert user: %w", err)
		}
		u = userFromRow(row.ID, row.GithubLogin, row.GithubID, row.DisplayName, row.AvatarUrl, row.IsAdmin, row.CreatedAt, row.LastSeenAt)
		created = true
	case err != nil:
		return nil, false, fmt.Errorf("select user: %w", err)
	default:
		if err := q.UpdateUserProfile(ctx, dbq.UpdateUserProfileParams{
			ID:          existing.ID,
			DisplayName: displayName,
			AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
			GithubLogin: login,
		}); err != nil {
			return nil, false, fmt.Errorf("update user: %w", err)
		}
		u = userFromRow(existing.ID, login, existing.GithubID, displayName, avatarURL, existing.IsAdmin, existing.CreatedAt, existing.LastSeenAt)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit: %w", err)
	}
	return &u, created, nil
}

// GetUser fetches a user by id.
func GetUser(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*User, error) {
	row, err := dbq.New(pool).GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u := userFromRow(row.ID, row.GithubLogin, row.GithubID, row.DisplayName, row.AvatarUrl, row.IsAdmin, row.CreatedAt, row.LastSeenAt)
	return &u, nil
}

// TouchUserSeen records the user's most recent activity.
func TouchUserSeen(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	return dbq.New(pool).TouchUserLastSeen(ctx, id)
}

// UserSummary is the lightweight shape returned by ListAllUsers: User
// fields plus a project_count derived from memberships, useful for the
// global Users directory.
type UserSummary struct {
	User
	ProjectCount int
}

// ListAllUsers returns every user on the instance plus how many
// projects they belong to. Visible to any authenticated caller.
func ListAllUsers(ctx context.Context, pool *pgxpool.Pool) ([]UserSummary, error) {
	rows, err := dbq.New(pool).ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]UserSummary, 0, len(rows))
	for _, r := range rows {
		u := userFromRow(r.ID, r.GithubLogin, r.GithubID, r.DisplayName, r.AvatarUrl, r.IsAdmin, r.CreatedAt, r.LastSeenAt)
		out = append(out, UserSummary{User: u, ProjectCount: int(r.ProjectCount)})
	}
	return out, nil
}

func userFromRow(id uuid.UUID, login string, gid int64, name string, avatar string, admin bool, created, lastSeen pgtype.Timestamptz) User {
	var last *time.Time
	if lastSeen.Valid {
		v := lastSeen.Time
		last = &v
	}
	return User{
		ID:          id,
		GithubLogin: login,
		GithubID:    gid,
		DisplayName: name,
		AvatarURL:   avatar,
		IsAdmin:     admin,
		CreatedAt:   created.Time,
		LastSeenAt:  last,
	}
}
