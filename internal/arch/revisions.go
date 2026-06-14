package arch

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
)

// RevisionSummary is one row of the project's arch history list. The
// (potentially large) snapshot is intentionally omitted; clients fetch
// the full snapshot via GetRevision when they need it.
type RevisionSummary struct {
	ID           uuid.UUID        `json:"id"`
	Version      int              `json:"version"`
	Message      string           `json:"message"`
	AuthorUserID *uuid.UUID       `json:"author_user_id"`
	ViaMCP       *identity.ViaMCP `json:"via_mcp,omitempty"`
	WriteCount   int              `json:"write_count"`
	AutoFlushed  bool             `json:"auto_flushed"`
	CreatedAt    time.Time        `json:"created_at"`
}

// Revision is a full snapshot with the graph payload included.
type Revision struct {
	RevisionSummary
	Snapshot map[string]any `json:"snapshot"`
}

// CheckpointResult is what nottario.arch.checkpoint returns to the
// agent after a successful flush.
type CheckpointResult struct {
	Version    int       `json:"version"`
	Message    string    `json:"message"`
	WriteCount int       `json:"write_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// ErrNoActiveSession is returned by Checkpoint when the caller has no
// open arch session for the project.
var ErrNoActiveSession = errors.New("no active arch session for this caller")

// ListRevisions returns the history page for a project.
func ListRevisions(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, limit int, beforeVersion *int) ([]RevisionSummary, error) {
	if limit < 1 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}
	var before pgtype.Int4
	if beforeVersion != nil {
		before = pgtype.Int4{Int32: int32(*beforeVersion), Valid: true}
	}
	rows, err := dbq.New(pool).ListArchRevisions(ctx, dbq.ListArchRevisionsParams{
		ProjectID:     projectID,
		BeforeVersion: before,
		PageLimit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]RevisionSummary, 0, len(rows))
	tokenIDs := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.AuthorTokenID != nil {
			tokenIDs = append(tokenIDs, *r.AuthorTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, pool, tokenIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, RevisionSummary{
			ID:           r.ID,
			Version:      int(r.Version),
			Message:      r.Message,
			AuthorUserID: r.AuthorUserID,
			ViaMCP:       identity.ViaMCPFromMap(r.AuthorTokenID, names),
			WriteCount:   int(r.WriteCount),
			AutoFlushed:  r.AutoFlushed,
			CreatedAt:    r.CreatedAt.Time,
		})
	}
	return out, nil
}

// GetRevision returns one revision by (project_id, version) with its
// full snapshot.
func GetRevision(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, version int) (*Revision, error) {
	row, err := dbq.New(pool).GetArchRevision(ctx, dbq.GetArchRevisionParams{
		ProjectID: projectID,
		Version:   int32(version),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionNotFound
	}
	if err != nil {
		return nil, err
	}
	var snap map[string]any
	if err := json.Unmarshal(row.Snapshot, &snap); err != nil {
		return nil, err
	}
	names, err := identity.LookupTokenNames(ctx, pool, nonNilTokenIDs(row.AuthorTokenID))
	if err != nil {
		return nil, err
	}
	return &Revision{
		RevisionSummary: RevisionSummary{
			ID:           row.ID,
			Version:      int(row.Version),
			Message:      row.Message,
			AuthorUserID: row.AuthorUserID,
			ViaMCP:       identity.ViaMCPFromMap(row.AuthorTokenID, names),
			WriteCount:   int(row.WriteCount),
			AutoFlushed:  row.AutoFlushed,
			CreatedAt:    row.CreatedAt.Time,
		},
		Snapshot: snap,
	}, nil
}

// ErrRevisionNotFound is returned by GetRevision when no row matches.
var ErrRevisionNotFound = errors.New("arch revision not found")

// Checkpoint closes the caller's open session (if any) into a single
// arch_revisions row with the provided message. Returns
// ErrNoActiveSession when the caller does not own the current lock.
func Checkpoint(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, message string) (*CheckpointResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	row, err := q.GetArchLockForUpdate(ctx, projectID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNoActiveSession
	case err != nil:
		return nil, err
	}
	if row.AuthorUserID != by.UserID {
		return nil, ErrNoActiveSession
	}

	version, created, writes, err := flushLockedSessionReturn(ctx, q, projectID, row, false, message)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CheckpointResult{
		Version:    version,
		Message:    message,
		WriteCount: writes,
		CreatedAt:  created,
	}, nil
}

// flushLockedSession builds the snapshot for the project, inserts an
// arch_revisions row and deletes the lock. Must run inside an already-
// open transaction that holds the lock row FOR UPDATE. The auto flag
// controls auto_flushed on the revision; message is the optional
// commit-style message.
func flushLockedSession(ctx context.Context, q *dbq.Queries, projectID uuid.UUID, lock dbq.ArchLock, auto bool, message string) error {
	_, _, _, err := flushLockedSessionReturn(ctx, q, projectID, lock, auto, message)
	return err
}

func flushLockedSessionReturn(
	ctx context.Context,
	q *dbq.Queries,
	projectID uuid.UUID,
	lock dbq.ArchLock,
	auto bool,
	message string,
) (version int, createdAt time.Time, writeCount int, err error) {
	snap, err := q.BuildArchSnapshot(ctx, projectID)
	if err != nil {
		return 0, time.Time{}, 0, err
	}
	prev, err := q.MaxArchRevisionVersion(ctx, projectID)
	if err != nil {
		return 0, time.Time{}, 0, err
	}
	next := int32(prev) + 1
	row, err := q.InsertArchRevision(ctx, dbq.InsertArchRevisionParams{
		ProjectID:     projectID,
		Version:       next,
		Snapshot:      snap,
		Message:       message,
		AuthorUserID:  &lock.AuthorUserID,
		AuthorTokenID: lock.AuthorTokenID,
		WriteCount:    lock.WriteCount,
		AutoFlushed:   auto,
	})
	if err != nil {
		return 0, time.Time{}, 0, err
	}
	if err := q.DeleteArchLock(ctx, projectID); err != nil {
		return 0, time.Time{}, 0, err
	}
	return int(row.Version), row.CreatedAt.Time, int(lock.WriteCount), nil
}

func nonNilTokenIDs(ids ...*uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, p := range ids {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}
