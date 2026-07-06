package identity

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// MemberRow is a denormalised (member, role) entry for display.
// A member with zero role assignments appears once with RoleID == nil
// and the role_* fields empty — the presence of the row still tells
// the UI the user belongs to the project.
type MemberRow struct {
	UserID      uuid.UUID  `json:"user_id"`
	GithubLogin string     `json:"github_login"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url"`
	IsAdmin     bool       `json:"is_admin"`
	RoleID      *uuid.UUID `json:"role_id,omitempty"`
	RoleKey     string     `json:"role_key,omitempty"`
	RoleLabel   string     `json:"role_label,omitempty"`
	RoleColor   string     `json:"role_color,omitempty"`
}

// AddMembership makes user a member of project and grants them role.
// Idempotent on both halves. The two writes happen in a transaction
// so a partially-populated state (member without the requested role,
// or role assignment without a member row) never surfaces.
func AddMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	if err := q.EnsureMembership(ctx, dbq.EnsureMembershipParams{
		UserID: userID, ProjectID: projectID,
	}); err != nil {
		return fmt.Errorf("ensure membership: %w", err)
	}
	if err := q.AssignRole(ctx, dbq.AssignRoleParams{
		UserID: userID, ProjectID: projectID, RoleID: roleID,
	}); err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return tx.Commit(ctx)
}

// RemoveMembership revokes a single role from a user within a project.
// The member row itself stays — dropping the last role does NOT remove
// the user from the project. Use RemoveMember for that.
func RemoveMembership(ctx context.Context, pool *pgxpool.Pool, userID, projectID, roleID uuid.UUID) error {
	return dbq.New(pool).UnassignRole(ctx, dbq.UnassignRoleParams{
		UserID: userID, ProjectID: projectID, RoleID: roleID,
	})
}

// RemoveMember removes the user from the project entirely; their role
// assignments cascade away via the composite FK on membership_roles.
func RemoveMember(ctx context.Context, pool *pgxpool.Pool, userID, projectID uuid.UUID) error {
	return dbq.New(pool).RemoveMembership(ctx, dbq.RemoveMembershipParams{
		UserID: userID, ProjectID: projectID,
	})
}

// ListMembers returns every (user, role) tuple in the project. A member
// with no roles still appears exactly once, with RoleID == nil.
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
// holds, across all projects. Projects the user belongs to without any
// role assignments are omitted here — they still count for access
// (see IsProjectMember), but they don't feed the role-scoped views.
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
// Returns an empty slice for a member with no role assignments.
func UserRoleIDs(ctx context.Context, pool *pgxpool.Pool, userID, projectID uuid.UUID) ([]uuid.UUID, error) {
	return dbq.New(pool).ListUserRoleIDsInProject(ctx, dbq.ListUserRoleIDsInProjectParams{
		UserID: userID, ProjectID: projectID,
	})
}
