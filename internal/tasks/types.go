// Package tasks owns task entities and their related concepts:
// dependencies between tasks, commit links and comments. It exposes
// the queries the REST and (eventually) the MCP layers use.
package tasks

import (
	"time"

	"github.com/google/uuid"
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
	StateTodo  State = "todo"
	StateDoing State = "doing"
	StateDone  State = "done"
)

// Task is the central entity of the work domain.
type Task struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	ParentTaskID     *uuid.UUID
	Type             Type
	Title            string
	DescriptionMD    string
	State            State
	Priority         int
	AssigneeUserID   *uuid.UUID
	TargetRoleID     *uuid.UUID
	ActualStart      *time.Time
	ActualEnd        *time.Time
	CreatedByUserID  *uuid.UUID
	CreatedByTokenID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CycleID          uuid.UUID
}

// Dependency is the directed relation "task depends on another task".
type Dependency struct {
	TaskID      uuid.UUID
	DependsOnID uuid.UUID
}

// CommitLink ties a task to one or more git commits.
type CommitLink struct {
	TaskID  uuid.UUID
	Repo    string
	SHA     string
	Message string
	AddedAt time.Time
}

// Comment is a markdown note attached to a task.
type Comment struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	AuthorUserID  *uuid.UUID
	AuthorTokenID *uuid.UUID
	BodyMD        string
	CreatedAt     time.Time
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
	case StateTodo, StateDoing, StateDone:
		return true
	}
	return false
}
