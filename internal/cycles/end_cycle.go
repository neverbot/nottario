package cycles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// publishCycleEvent emits a cycle.* notification on the realtime
// channel. Best-effort: any failure is silently ignored so realtime
// hiccups never affect the EndCycle outcome.
func publishCycleEvent(ctx context.Context, pool *pgxpool.Pool, kind string, projectID, cycleID uuid.UUID) {
	payload := map[string]any{
		"type":       kind,
		"project_id": projectID,
		"cycle_id":   cycleID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, "SELECT pg_notify('nottario_events', $1)", string(b))
}

// EndCycleParams carries the inputs to EndCycle.
type EndCycleParams struct {
	ProjectID uuid.UUID
	NextName  string // optional; defaults to "<label>-<N+1>"
}

// EndCycleResult is what the caller gets back.
type EndCycleResult struct {
	Closed Cycle `json:"closed"`
	Next   Cycle `json:"next"`
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

	// Close the current cycle first so the partial unique index
	// `cycles_one_active_per_project` (project_id WHERE closed_at IS
	// NULL) is satisfied when we insert the next one. Both happen in
	// the same transaction so external observers never see "no active
	// cycle".
	if err := q.CloseCycle(ctx, dbq.CloseCycleParams{
		ID:              closing.ID,
		ClosedByUserID:  by.UserID,
		ClosedByTokenID: by.TokenID,
	}); err != nil {
		return nil, fmt.Errorf("close: %w", err)
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

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Re-read the closed row to capture the updated closed_at.
	updatedClosing, err := Get(ctx, pool, closing.ID)
	if err != nil {
		return nil, fmt.Errorf("reread closed: %w", err)
	}

	publishCycleEvent(ctx, pool, "cycle.closed", closing.ProjectID, closing.ID)
	publishCycleEvent(ctx, pool, "cycle.created", next.ProjectID, next.ID)

	return &EndCycleResult{Closed: *updatedClosing, Next: next}, nil
}
