package identity

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// MemberRow is a denormalised membership entry for display: it
// flattens the user, role and timestamps so the UI can render a
// table in a single pass.
type MemberRow struct {
	UserID      uuid.UUID `json:"user_id"`
	GithubLogin string    `json:"github_login"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	IsAdmin     bool      `json:"is_admin"`
	RoleID      uuid.UUID `json:"role_id"`
	RoleKey     string    `json:"role_key"`
	RoleLabel   string    `json:"role_label"`
	RoleColor   string    `json:"role_color"`
}

// AddMembership grants role to user within project.
func AddMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	return dbq.New(pool).InsertMembership(ctx, dbq.InsertMembershipParams{
		UserID: userID, ProjectID: projectID, RoleID: roleID,
	})
}

// RemoveMembership revokes role from user within project.
func RemoveMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	return dbq.New(pool).DeleteMembership(ctx, dbq.DeleteMembershipParams{
		UserID: userID, ProjectID: projectID, RoleID: roleID,
	})
}

// ListMembers returns every (user, role) tuple in the project.
func ListMembers(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]MemberRow, error) {
	rows, err := dbq.New(pool).ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, MemberRow{
			UserID:      r.UserID,
			GithubLogin: r.GithubLogin,
			DisplayName: r.DisplayName,
			AvatarURL:   r.AvatarUrl,
			IsAdmin:     r.IsAdmin,
			RoleID:      r.RoleID,
			RoleKey:     r.RoleKey,
			RoleLabel:   r.RoleLabel,
			RoleColor:   r.RoleColor,
		})
	}
	return out, nil
}

// UserMembership flattens a single (project, role) entry as seen
// from a user's perspective — used by whoami so an agent can decide
// which roles to filter `tasks.next` by, in which projects.
type UserMembership struct {
	ProjectID    uuid.UUID `json:"project_id"`
	ProjectSlug  string    `json:"project_slug"`
	ProjectName  string    `json:"project_name"`
	RoleID       uuid.UUID `json:"role_id"`
	RoleKey      string    `json:"role_key"`
	RoleLabel    string    `json:"role_label"`
	RoleColor    string    `json:"role_color"`
	RolePosition int       `json:"role_position"`
}

// ListUserMemberships returns every (project, role) tuple the user
// belongs to, across all projects. Ordered by project slug then role
// position so the response is deterministic.
func ListUserMemberships(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]UserMembership, error) {
	rows, err := dbq.New(pool).ListMembershipsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]UserMembership, 0, len(rows))
	for _, m := range rows {
		out = append(out, UserMembership{
			ProjectID:    m.ProjectID,
			ProjectSlug:  m.ProjectSlug,
			ProjectName:  m.ProjectName,
			RoleID:       m.RoleID,
			RoleKey:      m.RoleKey,
			RoleLabel:    m.RoleLabel,
			RoleColor:    m.RoleColor,
			RolePosition: int(m.RolePosition),
		})
	}
	return out, nil
}

// UserRoleIDs returns the role uuids the user holds in the project.
func UserRoleIDs(ctx context.Context, pool *pgxpool.Pool, userID, projectID uuid.UUID) ([]uuid.UUID, error) {
	return dbq.New(pool).ListUserRoleIDsInProject(ctx, dbq.ListUserRoleIDsInProjectParams{
		UserID: userID, ProjectID: projectID,
	})
}
