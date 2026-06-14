// Package docs owns the shared markdown context: skills, project
// documentation, and free-form notes. Every document carries a path,
// a kind, optional frontmatter, and a linear version history. Search
// runs over the `search_vector` Postgres column with FTS.
package docs

import (
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
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
	ID               uuid.UUID        `json:"id"`
	Scope            Scope            `json:"scope"`
	ProjectID        *uuid.UUID       `json:"project_id"`
	Path             string           `json:"path"`
	Kind             Kind             `json:"kind"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	ContentMD        string           `json:"content"`
	Frontmatter      map[string]any   `json:"frontmatter"`
	CurrentVersion   int              `json:"current_version"`
	DeletedAt        *time.Time       `json:"deleted_at"`
	CreatedByUserID  *uuid.UUID       `json:"created_by_user_id"`
	CreatedByTokenID *uuid.UUID       `json:"-"`
	UpdatedByUserID  *uuid.UUID       `json:"updated_by_user_id"`
	UpdatedByTokenID *uuid.UUID       `json:"-"`
	CreatedViaMCP    *identity.ViaMCP `json:"created_via_mcp,omitempty"`
	UpdatedViaMCP    *identity.ViaMCP `json:"updated_via_mcp,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	// ContentHTML is filled by the web layer before serialization
	// (see ReadDocHandler / ReadDocVersionHandler). It is never
	// persisted and never set by the docs repo. Empty when the
	// caller did not request HTML.
	ContentHTML string `json:"content_html,omitempty"`
}

// Version is one entry in `document_versions`.
type Version struct {
	ID            uuid.UUID        `json:"id"`
	DocumentID    uuid.UUID        `json:"document_id"`
	Version       int              `json:"version"`
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	ContentMD     string           `json:"content"`
	Frontmatter   map[string]any   `json:"frontmatter"`
	Message       string           `json:"message"`
	AuthorUserID  *uuid.UUID       `json:"author_user_id"`
	AuthorTokenID *uuid.UUID       `json:"-"`
	ViaMCP        *identity.ViaMCP `json:"via_mcp,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	// ContentHTML is filled by the web layer before serialization;
	// see the comment on Document.ContentHTML.
	ContentHTML string `json:"content_html,omitempty"`
}

// ValidScope reports whether s is a recognised scope.
func ValidScope(s Scope) bool { return s == ScopeProject || s == ScopeGlobal }

// ValidKind reports whether k is a recognised kind.
func ValidKind(k Kind) bool { return k == KindSkill || k == KindContext || k == KindNote }
