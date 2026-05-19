package identity

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemberRow is a denormalised membership entry for display: it
// flattens the user, role and timestamps so the UI can render a
// table in a single pass.
type MemberRow struct {
	UserID      uuid.UUID
	GithubLogin string
	DisplayName string
	AvatarURL   string
	IsAdmin     bool
	RoleID      uuid.UUID
	RoleKey     string
	RoleLabel   string
	RoleColor   string
}

// AddMembership grants role to user within project.
func AddMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO memberships (user_id, project_id, role_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, userID, projectID, roleID)
	return err
}

// RemoveMembership revokes role from user within project.
func RemoveMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM memberships
		WHERE user_id = $1 AND project_id = $2 AND role_id = $3
	`, userID, projectID, roleID)
	return err
}

// ListMembers returns every (user, role) tuple in the project.
func ListMembers(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]MemberRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.id, u.github_login, u.display_name, COALESCE(u.avatar_url, ''), u.is_admin,
		       r.id, r.key, r.label, COALESCE(r.color, '')
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.project_id = $1
		ORDER BY u.display_name, r.label
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberRow{}
	for rows.Next() {
		var row MemberRow
		if err := rows.Scan(
			&row.UserID, &row.GithubLogin, &row.DisplayName, &row.AvatarURL, &row.IsAdmin,
			&row.RoleID, &row.RoleKey, &row.RoleLabel, &row.RoleColor,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// UserMembership flattens a single (project, role) entry as seen
// from a user's perspective — used by whoami so an agent can decide
// which roles to filter `tasks.next` by, in which projects.
type UserMembership struct {
	ProjectID    uuid.UUID
	ProjectSlug  string
	ProjectName  string
	RoleID       uuid.UUID
	RoleKey      string
	RoleLabel    string
	RoleColor    string
	RolePosition int
}

// ListUserMemberships returns every (project, role) tuple the user
// belongs to, across all projects. Ordered by project slug then role
// position so the response is deterministic.
func ListUserMemberships(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]UserMembership, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.slug, p.name,
		       r.id, r.key, r.label, COALESCE(r.color, ''), r.position
		FROM memberships m
		JOIN projects p ON p.id = m.project_id
		JOIN roles    r ON r.id = m.role_id
		WHERE m.user_id = $1
		ORDER BY p.slug, r.position, r.label
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserMembership{}
	for rows.Next() {
		var m UserMembership
		if err := rows.Scan(
			&m.ProjectID, &m.ProjectSlug, &m.ProjectName,
			&m.RoleID, &m.RoleKey, &m.RoleLabel, &m.RoleColor, &m.RolePosition,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UserRoleIDs returns the role uuids the user holds in the project.
func UserRoleIDs(ctx context.Context, pool *pgxpool.Pool, userID, projectID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `
		SELECT role_id FROM memberships
		WHERE user_id = $1 AND project_id = $2
	`, userID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
