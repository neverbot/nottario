package arch

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// FlushTicker scans for expired arch sessions and flushes each into a
// fresh arch_revisions row. It runs in its own goroutine until the
// supplied context is cancelled. Safe to start exactly once per
// process — re-starting would double-flush.
type FlushTicker struct {
	pool    *pgxpool.Pool
	tick    time.Duration
	logger  *slog.Logger
	running atomic.Bool
}

// NewFlushTicker builds a ticker bound to pool. tick is the interval
// between scans; defaults to 30s when zero or negative.
func NewFlushTicker(pool *pgxpool.Pool, tick time.Duration, logger *slog.Logger) *FlushTicker {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FlushTicker{pool: pool, tick: tick, logger: logger}
}

// Start runs the ticker in a goroutine. Returns immediately. Cancel
// ctx to stop.
func (t *FlushTicker) Start(ctx context.Context) {
	if !t.running.CompareAndSwap(false, true) {
		return // already started
	}
	go t.loop(ctx)
}

func (t *FlushTicker) loop(ctx context.Context) {
	tk := time.NewTicker(t.tick)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			t.flushOnce(ctx)
		}
	}
}

func (t *FlushTicker) flushOnce(ctx context.Context) {
	// Use a short timeout so a stuck flush doesn't block the next tick.
	scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, err := dbq.New(t.pool).ListExpiredArchLocks(scanCtx, int32(idleConfig.DefaultSeconds))
	if err != nil {
		t.logger.Warn("arch flush: list expired locks failed", "err", err)
		return
	}
	for _, r := range rows {
		if err := t.flushOne(scanCtx, r.ProjectID); err != nil {
			t.logger.Warn("arch flush: flush project failed", "project_id", r.ProjectID, "err", err)
		}
	}
}

func (t *FlushTicker) flushOne(ctx context.Context, projectID uuid.UUID) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	row, err := q.GetArchLockForUpdate(ctx, projectID)
	if err != nil {
		// Lock may have been flushed by a write that arrived in
		// between (concurrent write eviction). Treat as no-op.
		if isNoRows(err) {
			return nil
		}
		return err
	}
	// Re-check expiry inside the tx — another write may have just
	// extended the lock.
	threshold, err := resolveIdleThreshold(ctx, q, projectID, idleConfig)
	if err != nil {
		return err
	}
	if time.Since(row.LastWriteAt.Time) < threshold {
		return nil
	}
	if err := flushLockedSession(ctx, q, projectID, row, true, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
