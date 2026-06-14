// Package identity owns user accounts, sessions, API tokens,
// projects, roles, memberships and the GitHub OAuth flow. It also
// exposes the auth middleware that resolves a Caller from either a
// session cookie or a Bearer token.
package identity

import (
	"time"

	"github.com/google/uuid"
)

// User is a human identity, mapped to a GitHub account.
type User struct {
	ID          uuid.UUID  `json:"id"`
	GithubLogin string     `json:"github_login"`
	GithubID    int64      `json:"github_id"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url"`
	IsAdmin     bool       `json:"is_admin"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
}

// Session is a browser session backed by a signed cookie.
type Session struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
}

// Project is a unit of work that groups tasks, documents and an
// architectural diagram. It has a list of GitHub repos as metadata
// and its own catalogue of roles.
//
// Stats and Members are optional cheap enrichments populated by
// ListProjects for the projects-list cards; they remain nil on
// endpoints that don't need them (GetProject, Insert/Update).
type Project struct {
	ID              uuid.UUID       `json:"id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	PrimaryLanguage string          `json:"primary_language"`
	ProjectType     string          `json:"project_type"`
	MCPPageSize     int             `json:"mcp_page_size"`
	DefaultView     string          `json:"default_view"`
	CycleLabel      string          `json:"cycle_label"`
	OwnerUserID     uuid.UUID       `json:"owner_user_id"`
	CreatedByUserID *uuid.UUID      `json:"created_by_user_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Stats           *ProjectStats   `json:"stats,omitempty"`
	Members         []ProjectMember `json:"members,omitempty"`
}

// ProjectStats summarises a project's task counts and most recent
// activity. Feature parents are excluded from the counts because
// they're aggregates of their children.
type ProjectStats struct {
	TodoCount      int        `json:"todo_count"`
	DoingCount     int        `json:"doing_count"`
	DoneCount      int        `json:"done_count"`
	WontDoCount    int        `json:"wont_do_count"`
	LastActivityAt *time.Time `json:"last_activity_at"`
}

// ProjectMember is a lightweight user reference attached to a
// project for display purposes (avatar stack on the cards).
type ProjectMember struct {
	UserID      uuid.UUID `json:"user_id"`
	GithubLogin string    `json:"github_login"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
}

// Role is a per-project label (backend, frontend, qa, ...).
type Role struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	Color     string    `json:"color"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// Membership ties a user to a project under a role.
type Membership struct {
	UserID    uuid.UUID `json:"user_id"`
	ProjectID uuid.UUID `json:"project_id"`
	RoleID    uuid.UUID `json:"role_id"`
	CreatedAt time.Time `json:"created_at"`
}

// APIToken is a credential issued to a user; agents present it as a
// Bearer token. The plaintext value is only available at issuance.
type APIToken struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	Name          string     `json:"name"`
	Prefix        string     `json:"prefix"`
	DefaultRoleID *uuid.UUID `json:"default_role_id"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
}
