package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ErrCycle is returned by AddDependency when the new edge would
// introduce a cycle in the dependency graph.
var ErrCycle = errors.New("dependency would create a cycle")

// depLockNamespace is the int4 we hand to pg_advisory_xact_lock to
// tag dependency-graph mutations. The second int4 is the hash of the
// project id so concurrent AddDependency calls in DIFFERENT projects
// don't contend. Within a project, all AddDependency calls serialize,
// which is what prevents the 3+-node cycle race (e.g. A→B, B→C, C→A
// added concurrently from three different agents).
const depLockNamespace int32 = 0x44455053 // "DEPS" ascii

// AddDependency declares that task depends on dependsOn. Both must
// belong to the same project. Cycles are rejected with ErrCycle.
//
// Concurrency model:
//   - Take an xact-scoped advisory lock keyed on the project so all
//     dep mutations within a project serialize. Cheap because deps
//     are added rarely relative to other writes; bullet-proof against
//     N-node cycle races.
//   - Take a row-level lock on both endpoints so a SetState(done)
//     racing against this insert is forced to serialize too.
func AddDependency(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) error {
	if taskID == dependsOnID {
		return errors.New("task cannot depend on itself")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	projectID, err := q.ProjectIDForTask(ctx, taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("task not found")
	}
	if err != nil {
		return err
	}
	if err := q.AcquireDepLock(ctx, dbq.AcquireDepLockParams{
		Namespace: depLockNamespace,
		ProjectID: projectID.String(),
	}); err != nil {
		return err
	}
	if _, err := q.LockTwoTaskRows(ctx, []uuid.UUID{taskID, dependsOnID}); err != nil {
		return err
	}
	// Reachability walks FROM dependsOn (the precondition we're about
	// to add) checking whether it can already reach the dependent
	// (taskID) — if it can, the new edge closes a cycle.
	cycle, err := q.WouldCreateCycle(ctx, dbq.WouldCreateCycleParams{
		Start:  dependsOnID,
		Target: taskID,
	})
	if err != nil {
		return fmt.Errorf("cycle check: %w", err)
	}
	if cycle.Bool {
		return ErrCycle
	}
	if _, err := q.InsertDependency(ctx, dbq.InsertDependencyParams{
		TaskID:      taskID,
		DependsOnID: dependsOnID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RemoveDependency drops the edge if it exists.
func RemoveDependency(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) error {
	_, err := dbq.New(pool).RemoveDependency(ctx, dbq.RemoveDependencyParams{
		TaskID:      taskID,
		DependsOnID: dependsOnID,
	})
	return err
}

// ProjectDependency is one edge from the project-wide list.
type ProjectDependency struct {
	TaskID      uuid.UUID
	DependsOnID uuid.UUID
}

// ListAllDependencies returns every dependency edge of a project,
// used by the Gantt to compute topological positions for `todo`
// tasks in a single round-trip.
func ListAllDependencies(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]ProjectDependency, error) {
	rows, err := dbq.New(pool).ListProjectDependencies(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectDependency, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectDependency{TaskID: r.TaskID, DependsOnID: r.DependsOnID})
	}
	return out, nil
}

// ListDependenciesOf returns the IDs the task depends on.
func ListDependenciesOf(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]uuid.UUID, error) {
	return dbq.New(pool).ListDependsOn(ctx, taskID)
}

// ListDependentsOf returns the IDs of tasks that depend on this one.
func ListDependentsOf(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]uuid.UUID, error) {
	return dbq.New(pool).ListDependents(ctx, taskID)
}
