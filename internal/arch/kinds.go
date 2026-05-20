package arch

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ErrKindInUse is returned by DeleteKind when nodes still reference it.
var ErrKindInUse = errors.New("kind in use by one or more nodes")

// EnsureDefaultKinds seeds the default kind catalogue if the project
// has no kinds at all. Safe to call repeatedly.
func EnsureDefaultKinds(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) error {
	q := dbq.New(pool)
	count, err := q.CountArchKinds(ctx, projectID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := dbq.New(tx)
	for _, k := range DefaultKinds {
		if err := tq.InsertDefaultArchKind(ctx, dbq.InsertDefaultArchKindParams{
			ProjectID:   projectID,
			Key:         k.Key,
			Label:       k.Label,
			Icon:        k.Icon,
			Color:       k.Color,
			Description: k.Description,
		}); err != nil {
			return fmt.Errorf("seed kind %s: %w", k.Key, err)
		}
	}
	return tx.Commit(ctx)
}

// ListKinds returns every kind defined for the project.
func ListKinds(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Kind, error) {
	if err := EnsureDefaultKinds(ctx, pool, projectID); err != nil {
		return nil, err
	}
	rows, err := dbq.New(pool).ListArchKinds(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Kind, 0, len(rows))
	for _, r := range rows {
		out = append(out, Kind{
			ProjectID:   r.ProjectID,
			Key:         r.Key,
			Label:       r.Label,
			Icon:        r.Icon,
			Color:       r.Color,
			Description: r.Description,
			IsDefault:   r.IsDefault,
			CreatedAt:   r.CreatedAt.Time,
		})
	}
	return out, nil
}

// UpsertKind creates or updates a kind.
func UpsertKind(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, k Kind) (*Kind, error) {
	if k.Key == "" || k.Label == "" {
		return nil, errors.New("key and label are required")
	}
	row, err := dbq.New(pool).UpsertArchKind(ctx, dbq.UpsertArchKindParams{
		ProjectID:   projectID,
		Key:         k.Key,
		Label:       k.Label,
		Icon:        k.Icon,
		Color:       k.Color,
		Description: k.Description,
	})
	if err != nil {
		return nil, err
	}
	return &Kind{
		ProjectID:   row.ProjectID,
		Key:         row.Key,
		Label:       row.Label,
		Icon:        row.Icon,
		Color:       row.Color,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		CreatedAt:   row.CreatedAt.Time,
	}, nil
}

// DeleteKind removes a kind, refusing if any node still uses it.
func DeleteKind(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) error {
	q := dbq.New(pool)
	count, err := q.CountNodesByKind(ctx, dbq.CountNodesByKindParams{ProjectID: projectID, Kind: key})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrKindInUse
	}
	rows, err := q.DeleteArchKind(ctx, dbq.DeleteArchKindParams{ProjectID: projectID, Key: key})
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("kind not found")
	}
	return nil
}

// kindExistsTx reports whether a kind key is known for the project,
// using the supplied dbq.Queries (typically bound to an open tx).
func kindExistsTx(ctx context.Context, q *dbq.Queries, projectID uuid.UUID, key string) (bool, error) {
	return q.ArchKindExists(ctx, dbq.ArchKindExistsParams{ProjectID: projectID, Key: key})
}
