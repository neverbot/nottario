package tasks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Inconsistency is one issue found in a project's task graph.
// `Reason` is a stable machine-readable key; `Details` carries
// per-reason payload (e.g. the dependent ids that triggered the
// flag).
type Inconsistency struct {
	TaskID  uuid.UUID      `json:"task_id"`
	Reason  string         `json:"reason"`
	Details map[string]any `json:"details,omitempty"`
}

// Reason keys. New checks add new keys here; consumers treat unknown
// keys as opaque and still render them.
const (
	ReasonDependentAlreadyDone = "dependent_already_done"
)

// ListInconsistencies scans the project for known shapes of broken
// state. v1 covers `dependent_already_done`: a non-done task whose
// dependent has already shipped, which means either the parent task
// was forgotten or the dependent shouldn't have been allowed through.
func ListInconsistencies(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Inconsistency, error) {
	q := dbq.New(pool)
	rows, err := q.ListDoneDependentsInconsistencies(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Inconsistency, 0, len(rows))
	for _, r := range rows {
		ids := make([]string, 0, len(r.DoneDependentIds))
		for _, id := range r.DoneDependentIds {
			ids = append(ids, id.String())
		}
		out = append(out, Inconsistency{
			TaskID: r.TaskID,
			Reason: ReasonDependentAlreadyDone,
			Details: map[string]any{
				"dependent_task_ids": ids,
			},
		})
	}
	return out, nil
}
