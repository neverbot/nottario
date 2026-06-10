package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Priority is one bucket in the project's priority vocabulary.
type Priority struct {
	ProjectID uuid.UUID `json:"project_id"`
	Key       string    `json:"key"`
	Value     int       `json:"value"`
	Position  int       `json:"position"`
	IsDefault bool      `json:"is_default"`
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
	rows, err := dbq.New(pool).ListProjectPriorities(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Priority, 0, len(rows))
	for _, r := range rows {
		out = append(out, Priority{
			ProjectID: r.ProjectID,
			Key:       r.Key,
			Value:     int(r.Value),
			Position:  int(r.Position),
			IsDefault: r.IsDefault,
		})
	}
	return out, nil
}

// ResolvePriorityKey returns the numeric value for a key, or an error
// if the key is unknown for that project.
func ResolvePriorityKey(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) (int, error) {
	v, err := dbq.New(pool).GetPriorityValue(ctx, dbq.GetPriorityValueParams{
		ProjectID: projectID, Key: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("unknown priority key %q for this project", key)
	}
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// DefaultPriorityValue returns the value the project would like new
// tasks created without an explicit priority to use. It prefers the
// bucket whose key is "medium" (the seeded default). If that key
// doesn't exist (admin renamed/removed it), the bucket whose value
// is closest to 50 wins, with ties broken in favour of the higher
// value. If the project has no priorities at all (shouldn't happen
// post-seeding), returns 50 so the DB default still applies.
func DefaultPriorityValue(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) (int, error) {
	q := dbq.New(pool)
	v, err := q.GetPriorityValue(ctx, dbq.GetPriorityValueParams{ProjectID: projectID, Key: "medium"})
	if err == nil {
		return int(v), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	v, err = q.GetPriorityClosestTo50(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 50, nil
	}
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// UpsertPriority creates or updates a priority bucket.
func UpsertPriority(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string, value, position int) (*Priority, error) {
	if key == "" {
		return nil, errors.New("priority key is required")
	}
	if value < 0 || value > 1000 {
		return nil, errors.New("priority value must be between 0 and 1000")
	}
	row, err := dbq.New(pool).UpsertProjectPriority(ctx, dbq.UpsertProjectPriorityParams{
		ProjectID: projectID,
		Key:       key,
		Value:     int32(value),
		Position:  int32(position),
	})
	if err != nil {
		return nil, err
	}
	return &Priority{
		ProjectID: row.ProjectID,
		Key:       row.Key,
		Value:     int(row.Value),
		Position:  int(row.Position),
		IsDefault: row.IsDefault,
	}, nil
}

// RemovePriority deletes a priority bucket. Tasks already using its
// numeric value keep their integer priority unchanged; the bucket
// only disappears from the project's vocabulary.
func RemovePriority(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) error {
	rows, err := dbq.New(pool).DeleteProjectPriority(ctx, dbq.DeleteProjectPriorityParams{
		ProjectID: projectID, Key: key,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("priority not found")
	}
	return nil
}

// seedDefaultPriorities is called from CreateProject so every new
// project starts with the default catalogue.
func seedDefaultPriorities(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) error {
	q := dbq.New(tx)
	for _, p := range DefaultPriorities {
		if err := q.SeedDefaultPriority(ctx, dbq.SeedDefaultPriorityParams{
			ProjectID: projectID,
			Key:       p.Key,
			Value:     int32(p.Value),
			Position:  int32(p.Position),
		}); err != nil {
			return fmt.Errorf("seed priority %s: %w", p.Key, err)
		}
	}
	return nil
}
