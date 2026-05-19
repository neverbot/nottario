// Package docs owns the shared markdown context: skills, project
// documentation, and free-form notes. Every document carries a path,
// a kind, optional frontmatter, and a linear version history. Search
// runs over the `search_vector` Postgres column with FTS.
package docs

import (
	"time"

	"github.com/google/uuid"
)

// Scope distinguishes documents that live within a single project
// (scope = "project") from those visible across the whole instance
// (scope = "global"). Global documents have project_id = NULL and are
// managed by the instance admin.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Kind labels a document for human and agent navigation.
type Kind string

const (
	KindSkill   Kind = "skill"
	KindContext Kind = "context"
	KindNote    Kind = "note"
)

// Document is the row in `documents` plus its (already-decoded)
// frontmatter. ContentMD is the markdown body *without* the
// frontmatter front block — agents and humans see only the body. The
// raw frontmatter sits in Frontmatter.
type Document struct {
	ID                uuid.UUID
	Scope             Scope
	ProjectID         *uuid.UUID
	Path              string
	Kind              Kind
	Title             string
	Description       string
	ContentMD         string
	Frontmatter       map[string]any
	CurrentVersion    int
	DeletedAt         *time.Time
	CreatedByUserID   *uuid.UUID
	CreatedByTokenID  *uuid.UUID
	UpdatedByUserID   *uuid.UUID
	UpdatedByTokenID  *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Version is one entry in `document_versions`.
type Version struct {
	ID            uuid.UUID
	DocumentID    uuid.UUID
	Version       int
	Title         string
	Description   string
	ContentMD     string
	Frontmatter   map[string]any
	Message       string
	AuthorUserID  *uuid.UUID
	AuthorTokenID *uuid.UUID
	CreatedAt     time.Time
}

// ValidScope reports whether s is a recognised scope.
func ValidScope(s Scope) bool { return s == ScopeProject || s == ScopeGlobal }

// ValidKind reports whether k is a recognised kind.
func ValidKind(k Kind) bool { return k == KindSkill || k == KindContext || k == KindNote }
