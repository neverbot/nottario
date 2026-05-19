package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCycle is returned by AddDependency when the new edge would
// introduce a cycle in the dependency graph.
var ErrCycle = errors.New("dependency would create a cycle")

// AddDependency declares that task depends on dependsOn. Both must
// belong to the same project. Cycles are rejected with ErrCycle.
func AddDependency(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) error {
	if taskID == dependsOnID {
		return errors.New("task cannot depend on itself")
	}
	cycle, err := wouldCreateCycle(ctx, pool, taskID, dependsOnID)
	if err != nil {
		return err
	}
	if cycle {
		return ErrCycle
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO task_dependencies (task_id, depends_on_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, taskID, dependsOnID)
	return err
}

// RemoveDependency drops the edge if it exists.
func RemoveDependency(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM task_dependencies WHERE task_id = $1 AND depends_on_id = $2
	`, taskID, dependsOnID)
	return err
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

// wouldCreateCycle walks the dependency graph from dependsOn and
// checks whether taskID is reachable. If it is, the new edge would
// close a cycle.
func wouldCreateCycle(ctx context.Context, pool *pgxpool.Pool, taskID, dependsOnID uuid.UUID) (bool, error) {
	var hit bool
	err := pool.QueryRow(ctx, `
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
