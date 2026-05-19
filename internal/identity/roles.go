package identity

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ListRoles returns the role catalogue of a project.
func ListRoles(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Role, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, project_id, key, label, COALESCE(color, ''), created_at
		FROM roles
		WHERE project_id = $1
		ORDER BY label
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRole adds a role to a project's catalogue.
func CreateRole(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key, label, color string) (*Role, error) {
	var r Role
	err := pool.QueryRow(ctx, `
		INSERT INTO roles (project_id, key, label, color)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, project_id, key, label, COALESCE(color, ''), created_at
	`, projectID, key, label, color).Scan(
		&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdateRole edits the label and color of a role.
func UpdateRole(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, label, color string) (*Role, error) {
	var r Role
	err := pool.QueryRow(ctx, `
		UPDATE roles SET label = $2, color = NULLIF($3, '')
		WHERE id = $1
		RETURNING id, project_id, key, label, COALESCE(color, ''), created_at
	`, id, label, color).Scan(
		&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteRole removes a role. Memberships referencing it cascade.
func DeleteRole(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}
