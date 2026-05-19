package arch

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrKindInUse is returned by DeleteKind when nodes still reference it.
var ErrKindInUse = errors.New("kind in use by one or more nodes")

// EnsureDefaultKinds seeds the default kind catalogue if the project
// has no kinds at all. Safe to call repeatedly; it is a no-op once
// at least one kind exists for the project.
func EnsureDefaultKinds(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM arch_node_kinds WHERE project_id = $1`, projectID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, k := range DefaultKinds {
		_, err := tx.Exec(ctx, `
			INSERT INTO arch_node_kinds (project_id, key, label, icon, color, description, is_default)
			VALUES ($1, $2, $3, $4, $5, $6, true)
			ON CONFLICT DO NOTHING
		`, projectID, k.Key, k.Label, k.Icon, k.Color, k.Description)
		if err != nil {
			return fmt.Errorf("seed kind %s: %w", k.Key, err)
		}
	}
	return tx.Commit(ctx)
}

// ListKinds returns every kind defined for the project, seeding the
// defaults if the project has never had any.
func ListKinds(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Kind, error) {
	if err := EnsureDefaultKinds(ctx, pool, projectID); err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, key, label, icon, color, description, is_default, created_at
		FROM arch_node_kinds
		WHERE project_id = $1
		ORDER BY is_default DESC, label
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Kind{}
	for rows.Next() {
		var k Kind
		if err := rows.Scan(&k.ProjectID, &k.Key, &k.Label, &k.Icon, &k.Color, &k.Description, &k.IsDefault, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpsertKind creates or updates a kind. is_default is only set to
// true by EnsureDefaultKinds; user/agent additions are always false.
func UpsertKind(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, k Kind) (*Kind, error) {
	if k.Key == "" || k.Label == "" {
		return nil, errors.New("key and label are required")
	}
	var out Kind
	err := pool.QueryRow(ctx, `
		INSERT INTO arch_node_kinds (project_id, key, label, icon, color, description, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, false)
		ON CONFLICT (project_id, key) DO UPDATE
		SET label = EXCLUDED.label,
		    icon = EXCLUDED.icon,
		    color = EXCLUDED.color,
		    description = EXCLUDED.description
		RETURNING project_id, key, label, icon, color, description, is_default, created_at
	`, projectID, k.Key, k.Label, k.Icon, k.Color, k.Description).Scan(
		&out.ProjectID, &out.Key, &out.Label, &out.Icon, &out.Color, &out.Description, &out.IsDefault, &out.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteKind removes a kind. It refuses to delete a kind that is
// still used by at least one node.
func DeleteKind(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM arch_nodes WHERE project_id = $1 AND kind = $2`, projectID, key).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrKindInUse
	}
	cmd, err := pool.Exec(ctx, `DELETE FROM arch_node_kinds WHERE project_id = $1 AND key = $2`, projectID, key)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("kind not found")
	}
	return nil
}

// kindExists reports whether a kind key is known for the project.
func kindExists(ctx context.Context, q pgx.Tx, projectID uuid.UUID, key string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arch_node_kinds WHERE project_id = $1 AND key = $2)`, projectID, key).Scan(&ok)
	return ok, err
}
