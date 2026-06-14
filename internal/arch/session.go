package arch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Authorship attributes an arch mutation. UserID is the human behind
// the call (taken from a session OR an api token's owner); TokenID is
// the api token id when the call came via MCP, nil for direct human
// edits through the (today read-only) web UI. The lock is tracked per
// UserID so a single human's parallel agents share one session.
type Authorship struct {
	UserID  uuid.UUID
	TokenID *uuid.UUID
}

// LockedError is returned by write paths when the arch diagram is
// currently locked by a different user. The fields let the caller
// render a useful response (and let the REST/MCP layer translate to
// 423 Locked with a structured body).
type LockedError struct {
	ProjectID         uuid.UUID
	LockedByUserID    uuid.UUID
	LastWriteAt       time.Time
	RetryAfterSeconds int
}

func (e *LockedError) Error() string {
	return fmt.Sprintf(
		"arch diagram for project %s is locked by another user; retry in %ds",
		e.ProjectID, e.RetryAfterSeconds,
	)
}

// IsLocked reports whether err is (or wraps) a LockedError.
func IsLocked(err error) (*LockedError, bool) {
	var le *LockedError
	if errors.As(err, &le) {
		return le, true
	}
	return nil, false
}

// IdleConfig carries the runtime configuration the session helper
// needs to evaluate "is the existing lock expired" inline.
type IdleConfig struct {
	// Default idle threshold in seconds. Used when a project has no
	// arch_lock_idle_seconds override.
	DefaultSeconds int
}

// DefaultIdleConfig falls back to 120 seconds when nothing is wired.
var DefaultIdleConfig = IdleConfig{DefaultSeconds: 120}

// idleConfig is the package-level current configuration. The main
// binary calls SetIdleConfig at startup from the runtime env; tests
// override it via the same setter.
var idleConfig = DefaultIdleConfig

// SetIdleConfig overrides the default lock idle window. Safe to call
// at process start; not safe for hot-reload (no synchronisation —
// readers in withSession use the value as-is).
func SetIdleConfig(c IdleConfig) {
	if c.DefaultSeconds < 1 {
		c.DefaultSeconds = DefaultIdleConfig.DefaultSeconds
	}
	idleConfig = c
}

// LockedAuthor returns the lock owner inside an already-open tx, or
// nil if there is no lock. Used by the MCP checkpoint tool.
func LockedAuthor(ctx context.Context, q *dbq.Queries, projectID uuid.UUID) (*dbq.ArchLock, error) {
	row, err := q.GetArchLock(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// withSession runs fn inside a transaction that has acquired (or
// extended) the arch lock for the given author. On a foreign-author
// non-expired lock it returns *LockedError without invoking fn.
//
// fn receives the open pgx.Tx and the bound dbq.Queries. fn MUST NOT
// commit/rollback — withSession owns the lifecycle.
func withSession(
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID uuid.UUID,
	by Authorship,
	fn func(tx pgx.Tx, q *dbq.Queries) error,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	if err := acquireOrExtendLock(ctx, q, projectID, by, idleConfig); err != nil {
		return err
	}
	if err := fn(tx, q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// acquireOrExtendLock implements the lock state machine inside the
// caller's transaction. Returns *LockedError when a different,
// non-expired author holds the lock.
func acquireOrExtendLock(
	ctx context.Context,
	q *dbq.Queries,
	projectID uuid.UUID,
	by Authorship,
	idle IdleConfig,
) error {
	row, err := q.GetArchLockForUpdate(ctx, projectID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No lock — create one.
		base, err := q.MaxArchRevisionVersion(ctx, projectID)
		if err != nil {
			return err
		}
		return q.InsertArchLock(ctx, dbq.InsertArchLockParams{
			ProjectID:     projectID,
			AuthorUserID:  by.UserID,
			AuthorTokenID: by.TokenID,
			BaseVersion:   int32(base),
		})
	case err != nil:
		return err
	}

	if row.AuthorUserID == by.UserID {
		// Same author — extend.
		return q.ExtendArchLock(ctx, dbq.ExtendArchLockParams{
			ProjectID:     projectID,
			AuthorTokenID: by.TokenID,
		})
	}

	// Different author — check expiry.
	threshold, err := resolveIdleThreshold(ctx, q, projectID, idle)
	if err != nil {
		return err
	}
	age := time.Since(row.LastWriteAt.Time)
	if age >= threshold {
		// Lock expired — flush it (one revision) and acquire fresh.
		if err := flushLockedSession(ctx, q, projectID, row, true /*auto*/, ""); err != nil {
			return err
		}
		base, err := q.MaxArchRevisionVersion(ctx, projectID)
		if err != nil {
			return err
		}
		return q.InsertArchLock(ctx, dbq.InsertArchLockParams{
			ProjectID:     projectID,
			AuthorUserID:  by.UserID,
			AuthorTokenID: by.TokenID,
			BaseVersion:   int32(base),
		})
	}

	// Active foreign lock — refuse with retry-after.
	retry := int((threshold - age).Round(time.Second).Seconds())
	if retry < 1 {
		retry = 1
	}
	return &LockedError{
		ProjectID:         projectID,
		LockedByUserID:    row.AuthorUserID,
		LastWriteAt:       row.LastWriteAt.Time,
		RetryAfterSeconds: retry,
	}
}

// resolveIdleThreshold returns the effective idle window for this
// project as a duration: project override when set, IdleConfig default
// otherwise.
func resolveIdleThreshold(ctx context.Context, q *dbq.Queries, projectID uuid.UUID, idle IdleConfig) (time.Duration, error) {
	override, err := q.GetProjectArchIdleSeconds(ctx, projectID)
	if err != nil {
		return 0, err
	}
	var v pgtype.Int4 = override
	if v.Valid {
		return time.Duration(v.Int32) * time.Second, nil
	}
	return time.Duration(idle.DefaultSeconds) * time.Second, nil
}
