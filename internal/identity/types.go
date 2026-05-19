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
	ID          uuid.UUID
	GithubLogin string
	GithubID    int64
	DisplayName string
	AvatarURL   string
	IsAdmin     bool
	CreatedAt   time.Time
	LastSeenAt  *time.Time
}

// Session is a browser session backed by a signed cookie.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	UserAgent  string
	IP         string
}

// Project is a unit of work that groups tasks, documents and an
// architectural diagram. It has a list of GitHub repos as metadata
// and its own catalogue of roles.
type Project struct {
	ID              uuid.UUID
	Slug            string
	Name            string
	Description     string
	PrimaryLanguage string
	ProjectType     string
	CreatedByUserID *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Repos           []string
}

// Role is a per-project label (backend, frontend, qa, ...).
type Role struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Key       string
	Label     string
	Color     string
	Position  int
	CreatedAt time.Time
}

// Membership ties a user to a project under a role.
type Membership struct {
	UserID    uuid.UUID
	ProjectID uuid.UUID
	RoleID    uuid.UUID
	CreatedAt time.Time
}

// APIToken is a credential issued to a user; agents present it as a
// Bearer token. The plaintext value is only available at issuance.
type APIToken struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Name          string
	Prefix        string
	DefaultRoleID *uuid.UUID
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	RevokedAt     *time.Time
}
