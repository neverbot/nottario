// Package cycles is the execution-batching domain. A cycle is a
// labeled bucket of tasks; closing one moves in-flight work to the
// next cycle and stamps done work with its delivery batch.
package cycles

import (
	"time"

	"github.com/google/uuid"
)

// Cycle is one execution batch in a project's history.
type Cycle struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	Name            string     `json:"name"`
	Position        int        `json:"position"`
	OpenedAt        time.Time  `json:"opened_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	ClosedByUserID  *uuid.UUID `json:"closed_by_user_id"`
	ClosedByTokenID *uuid.UUID `json:"-"`
}

// Authorship attributes a mutation. Mirrors the same struct in
// internal/tasks so callers can pass identity through without
// importing one package from the other.
type Authorship struct {
	UserID  *uuid.UUID
	TokenID *uuid.UUID
}
