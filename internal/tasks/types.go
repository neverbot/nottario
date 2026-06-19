// Package tasks owns task entities and their related concepts:
// dependencies between tasks, commit links and comments. It exposes
// the queries the REST and (eventually) the MCP layers use.
package tasks

import (
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
)

// Type is a label that the human assigns to a task to clarify its
// nature; it does not change behaviour, only presentation.
type Type string

const (
	TypeTask    Type = "task"
	TypeBug     Type = "bug"
	TypeChore   Type = "chore"
	TypeSpike   Type = "spike"
	TypeFeature Type = "feature"
)

// State is the simple lifecycle a task moves through.
type State string

const (
	StateTodo   State = "todo"
	StateDoing  State = "doing"
	StateDone   State = "done"
	StateWontDo State = "wont_do"
)

// Task is the central entity of the work domain.
type Task struct {
	ID               uuid.UUID        `json:"id"`
	ProjectID        uuid.UUID        `json:"project_id"`
	ParentTaskID     *uuid.UUID       `json:"parent_task_id"`
	Type             Type             `json:"type"`
	Title            string           `json:"title"`
	DescriptionMD    string           `json:"description"`
	State            State            `json:"state"`
	Priority         int              `json:"priority"`
	AssigneeUserID   *uuid.UUID       `json:"assignee_user_id"`
	TargetRoleID     *uuid.UUID       `json:"target_role_id"`
	ActualStart      *time.Time       `json:"actual_start"`
	ActualEnd        *time.Time       `json:"actual_end"`
	CreatedByUserID  *uuid.UUID       `json:"created_by_user_id"`
	CreatedByTokenID *uuid.UUID       `json:"-"`
	ViaMCP           *identity.ViaMCP `json:"via_mcp,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	CycleID          uuid.UUID        `json:"cycle_id"`
	EditedAt         *time.Time       `json:"edited_at,omitempty"`
	EditedByUserID   *uuid.UUID       `json:"edited_by_user_id,omitempty"`
}

// Dependency is the directed relation "task depends on another task".
type Dependency struct {
	TaskID      uuid.UUID `json:"task_id"`
	DependsOnID uuid.UUID `json:"depends_on_id"`
}

// CommitLink ties a task to one or more git commits.
type CommitLink struct {
	TaskID  uuid.UUID `json:"task_id"`
	Repo    string    `json:"repo"`
	SHA     string    `json:"sha"`
	Message string    `json:"message"`
	AddedAt time.Time `json:"added_at"`
}

// Comment is a markdown note attached to a task.
type Comment struct {
	ID             uuid.UUID        `json:"id"`
	TaskID         uuid.UUID        `json:"task_id"`
	AuthorUserID   *uuid.UUID       `json:"author_user_id"`
	AuthorTokenID  *uuid.UUID       `json:"-"`
	ViaMCP         *identity.ViaMCP `json:"via_mcp,omitempty"`
	BodyMD         string           `json:"body"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	EditedAt       *time.Time       `json:"edited_at,omitempty"`
	EditedByUserID *uuid.UUID       `json:"edited_by_user_id,omitempty"`
}

// ValidType returns true when t is one of the recognised type values.
func ValidType(t Type) bool {
	switch t {
	case TypeTask, TypeBug, TypeChore, TypeSpike, TypeFeature:
		return true
	}
	return false
}

// ValidState returns true when s is one of the recognised state values.
func ValidState(s State) bool {
	switch s {
	case StateTodo, StateDoing, StateDone, StateWontDo:
		return true
	}
	return false
}

// IsClosed returns true when s represents a terminal "closed" state —
// the work is done with, whether or not it shipped. Used by
// dependency-precondition checks and feature-parent rollup so a
// `wont_do` upstream satisfies a downstream and a feature with a mix
// of done + wont_do children still rolls up to done.
func IsClosed(s State) bool {
	return s == StateDone || s == StateWontDo
}
