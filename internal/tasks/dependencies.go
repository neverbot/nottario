package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	defer tx.Rollback(ctx)

	// Resolve the project from the task row so we can scope the lock.
	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT project_id FROM tasks WHERE id = $1`, taskID).Scan(&projectID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("task not found")
		}
		return err
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock($1::int, hashtext($2::text))`,
		depLockNamespace, projectID,
	); err != nil {
		return err
	}

	// Lock both endpoints (ordered by id for deterministic acquisition)
	// so a SetState on either side and any other dep mutation touching
	// these rows serialize with us.
	if _, err := tx.Exec(ctx,
		`SELECT id FROM tasks WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`,
		taskID, dependsOnID,
	); err != nil {
		return err
	}

	cycle, err := wouldCreateCycleTx(ctx, tx, taskID, dependsOnID)
	if err != nil {
		return err
	}
	if cycle {
		return ErrCycle
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_dependencies (task_id, depends_on_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, taskID, dependsOnID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RemoveDependency drops the edge if it exists.
func RemoveDependency(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM task_dependencies WHERE task_id = $1 AND depends_on_id = $2
	`, taskID, dependsOnID)
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
	rows, err := pool.Query(ctx, `
		SELECT d.task_id, d.depends_on_id
		FROM task_dependencies d
		JOIN tasks t ON t.id = d.task_id
		WHERE t.project_id = $1
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectDependency{}
	for rows.Next() {
		var p ProjectDependency
		if err := rows.Scan(&p.TaskID, &p.DependsOnID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListDependenciesOf returns the IDs the task depends on.
func ListDependenciesOf(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `
		SELECT depends_on_id FROM task_dependencies WHERE task_id = $1
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListDependentsOf returns the IDs of tasks that depend on this one.
func ListDependentsOf(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `
		SELECT task_id FROM task_dependencies WHERE depends_on_id = $1
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// wouldCreateCycleTx walks the dependency graph from dependsOn and
// checks whether taskID is reachable. If it is, the new edge would
// close a cycle. Runs inside the caller's transaction so it sees a
// consistent snapshot.
func wouldCreateCycleTx(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, taskID, dependsOnID uuid.UUID) (bool, error) {
	var hit bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE reachable(id) AS (
			SELECT depends_on_id FROM task_dependencies WHERE task_id = $1
			UNION
			SELECT d.depends_on_id
			FROM task_dependencies d
			JOIN reachable r ON r.id = d.task_id
		)
		SELECT EXISTS (SELECT 1 FROM reachable WHERE id = $2)
		   OR $1 = $2
	`, dependsOnID, taskID).Scan(&hit)
	if err != nil {
		return false, fmt.Errorf("cycle check: %w", err)
	}
	return hit, nil
}
