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
	ID              uuid.UUID
	ProjectID       uuid.UUID
	Name            string
	Position        int
	OpenedAt        time.Time
	ClosedAt        *time.Time
	ClosedByUserID  *uuid.UUID
	ClosedByTokenID *uuid.UUID
}

// Authorship attributes a mutation. Mirrors the same struct in
// internal/tasks so callers can pass identity through without
// importing one package from the other.
type Authorship struct {
	UserID  *uuid.UUID
	TokenID *uuid.UUID
}
