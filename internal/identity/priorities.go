package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Priority is one bucket in the project's priority vocabulary.
type Priority struct {
	ProjectID uuid.UUID
	Key       string
	Value     int
	Position  int
	IsDefault bool
}

// DefaultPriorities is the seed catalogue inserted on project creation.
var DefaultPriorities = []Priority{
	{Key: "low", Value: 30, Position: 0, IsDefault: true},
	{Key: "medium", Value: 60, Position: 1, IsDefault: true},
	{Key: "high", Value: 90, Position: 2, IsDefault: true},
	{Key: "critical", Value: 100, Position: 3, IsDefault: true},
}

// ListPriorities returns the priority buckets defined for a project,
// ordered by position.
func ListPriorities(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Priority, error) {
	rows, err := pool.Query(ctx, `
		SELECT project_id, key, value, position, is_default
		FROM project_priorities
		WHERE project_id = $1
		ORDER BY position, value DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Priority{}
	for rows.Next() {
		var p Priority
		if err := rows.Scan(&p.ProjectID, &p.Key, &p.Value, &p.Position, &p.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResolvePriorityKey returns the numeric value for a key, or an error
// if the key is unknown for that project.
func ResolvePriorityKey(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) (int, error) {
	var v int
	err := pool.QueryRow(ctx,
		`SELECT value FROM project_priorities WHERE project_id = $1 AND key = $2`,
		projectID, key,
	).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("unknown priority key %q for this project", key)
	}
	return v, err
}

// UpsertPriority creates or updates a priority bucket.
func UpsertPriority(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string, value, position int) (*Priority, error) {
	if key == "" {
		return nil, errors.New("priority key is required")
	}
	if value < 0 || value > 1000 {
		return nil, errors.New("priority value must be between 0 and 1000")
	}
	var p Priority
	err := pool.QueryRow(ctx, `
		INSERT INTO project_priorities (project_id, key, value, position, is_default)
		VALUES ($1, $2, $3, $4, false)
		ON CONFLICT (project_id, key) DO UPDATE
		SET value = EXCLUDED.value, position = EXCLUDED.position
		RETURNING project_id, key, value, position, is_default
	`, projectID, key, value, position).Scan(&p.ProjectID, &p.Key, &p.Value, &p.Position, &p.IsDefault)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// RemovePriority deletes a priority bucket. Tasks already using its
// numeric value keep their integer priority unchanged; the bucket
// only disappears from the project's vocabulary.
func RemovePriority(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) error {
	cmd, err := pool.Exec(ctx,
		`DELETE FROM project_priorities WHERE project_id = $1 AND key = $2`,
		projectID, key,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("priority not found")
	}
	return nil
}

// SeedDefaultPriorities is called from CreateProject so every new
// project starts with the default catalogue.
func seedDefaultPriorities(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) error {
	for _, p := range DefaultPriorities {
		_, err := tx.Exec(ctx, `
			INSERT INTO project_priorities (project_id, key, value, position, is_default)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT DO NOTHING
		`, projectID, p.Key, p.Value, p.Position)
		if err != nil {
			return fmt.Errorf("seed priority %s: %w", p.Key, err)
		}
	}
	return nil
}
