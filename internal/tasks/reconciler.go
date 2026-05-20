package tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RollUpReconciler periodically scans for feature tasks whose
// children are all done but whose own state is still todo or doing
// (possible if a previous SetState ran without the transactional
// rollUp, or if a parent was set back to todo manually). It closes
// them via the same transactional rollUp the live path uses, so
// nothing observed via the API ever sees the "stuck feature" state.
//
// Cheap: one indexed query per interval, then a per-stuck transaction.
// Default interval 60s — small enough to be invisible to humans,
// large enough not to load the database.
type RollUpReconciler struct {
	Pool     *pgxpool.Pool
	Interval time.Duration
	Logger   *slog.Logger
}

// Run blocks until ctx is cancelled, scanning every Interval. Safe
// to call concurrently with the live SetState path because both use
// the same FOR UPDATE locks; whichever transaction acquires the
// parent row first wins.
func (r *RollUpReconciler) Run(ctx context.Context) error {
	if r.Interval <= 0 {
		r.Interval = 60 * time.Second
	}
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	// One scan at startup so anything that drifted while the process
	// was down gets corrected immediately.
	if err := r.scanOnce(ctx); err != nil && ctx.Err() == nil {
		r.Logger.Warn("rollup reconciler scan failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.scanOnce(ctx); err != nil && ctx.Err() == nil {
				r.Logger.Warn("rollup reconciler scan failed", "err", err)
			}
		}
	}
}

func (r *RollUpReconciler) scanOnce(ctx context.Context) error {
	rows, err := r.Pool.Query(ctx, `
		SELECT id
		FROM tasks t
		WHERE t.type = 'feature'
		  AND t.state <> 'done'
		  AND EXISTS (SELECT 1 FROM tasks c WHERE c.parent_task_id = t.id)
		  AND NOT EXISTS (SELECT 1 FROM tasks c WHERE c.parent_task_id = t.id AND c.state <> 'done')
	`)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.closeFeature(ctx, id); err != nil {
			r.Logger.Warn("rollup reconciler failed on feature", "id", id, "err", err)
		} else {
			r.Logger.Info("rollup reconciler closed stuck feature", "id", id)
		}
	}
	return nil
}

func (r *RollUpReconciler) closeFeature(ctx context.Context, id uuid.UUID) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	idCopy := id
	if err := rollUpParentDoneTx(ctx, tx, &idCopy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
