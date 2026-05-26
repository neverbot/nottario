package cycles

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// EndCycleParams carries the inputs to EndCycle.
type EndCycleParams struct {
	ProjectID uuid.UUID
	NextName  string // optional; defaults to "<label>-<N+1>"
}

// EndCycleResult is what the caller gets back.
type EndCycleResult struct {
	Closed Cycle
	Next   Cycle
}

// EndCycle atomically closes the project's active cycle and opens
// the next one, moving in-flight work forward per the cascade rules
// described in the spec (docs/superpowers/specs/2026-05-26-cycles-design.md).
func EndCycle(ctx context.Context, pool *pgxpool.Pool, p EndCycleParams, by Authorship) (*EndCycleResult, error) {
	if p.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	if err := q.AcquireCycleLock(ctx, p.ProjectID.String()); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}

	closingRow, err := q.LockActiveCycle(ctx, p.ProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoActiveCycle
	}
	if err != nil {
		return nil, fmt.Errorf("lock active: %w", err)
	}
	closing := rowToCycle(closingRow)

	name := p.NextName
	if name == "" {
		label, err := q.GetProjectCycleLabel(ctx, p.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("get label: %w", err)
		}
		name = fmt.Sprintf("%s-%d", label, closing.Position+1)
	}
	pos, err := q.NextCyclePosition(ctx, p.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("next position: %w", err)
	}
	nextRow, err := q.InsertCycle(ctx, dbq.InsertCycleParams{
		ProjectID: p.ProjectID,
		Name:      name,
		Position:  pos,
	})
	if err != nil {
		return nil, fmt.Errorf("insert next: %w", err)
	}
	next := rowToCycle(nextRow)

	// Step 1: move every partial feature subtree (incl. its done
	// children, re-stamping them) to the new cycle.
	if _, err := q.MovePartialFeatureSubtrees(ctx, dbq.MovePartialFeatureSubtreesParams{
		FromCycle: closing.ID,
		ToCycle:   next.ID,
	}); err != nil {
		return nil, fmt.Errorf("move partial features: %w", err)
	}

	// Step 2: move any remaining non-done tasks (those whose feature
	// parent — if any — was done, or who have no feature parent).
	if _, err := q.MoveStandaloneNonDone(ctx, dbq.MoveStandaloneNonDoneParams{
		FromCycle: closing.ID,
		ToCycle:   next.ID,
	}); err != nil {
		return nil, fmt.Errorf("move standalone non-done: %w", err)
	}

	if err := q.CloseCycle(ctx, dbq.CloseCycleParams{
		ID:              closing.ID,
		ClosedByUserID:  by.UserID,
		ClosedByTokenID: by.TokenID,
	}); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Re-read the closed row to capture the updated closed_at.
	updatedClosing, err := Get(ctx, pool, closing.ID)
	if err != nil {
		return nil, fmt.Errorf("reread closed: %w", err)
	}
	return &EndCycleResult{Closed: *updatedClosing, Next: next}, nil
}
