package cycles

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ErrNoActiveCycle is returned by ActiveCycle when a project has no
// open cycle (closed_at IS NULL row). The migration seeds one per
// project so this should never happen in normal operation; treat as
// an invariant violation.
var ErrNoActiveCycle = errors.New("no active cycle")

func rowToCycle(r dbq.Cycle) Cycle {
	var closedAt *time.Time
	if r.ClosedAt.Valid {
		t := r.ClosedAt.Time
		closedAt = &t
	}
	return Cycle{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		Name:            r.Name,
		Position:        int(r.Position),
		OpenedAt:        r.OpenedAt.Time,
		ClosedAt:        closedAt,
		ClosedByUserID:  r.ClosedByUserID,
		ClosedByTokenID: r.ClosedByTokenID,
	}
}

// List returns the project's cycles ordered by position desc
// (newest first).
func List(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Cycle, error) {
	rows, err := dbq.New(pool).ListCycles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Cycle, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToCycle(r))
	}
	return out, nil
}

// Get returns a single cycle by id.
func Get(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Cycle, error) {
	row, err := dbq.New(pool).GetCycle(ctx, id)
	if err != nil {
		return nil, err
	}
	c := rowToCycle(row)
	return &c, nil
}

// ActiveCycle returns the project's open cycle (the unique one with
// closed_at IS NULL).
func ActiveCycle(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) (*Cycle, error) {
	row, err := dbq.New(pool).GetActiveCycle(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoActiveCycle
	}
	if err != nil {
		return nil, err
	}
	c := rowToCycle(row)
	return &c, nil
}
