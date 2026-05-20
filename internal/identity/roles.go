package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ListRoles returns the role catalogue of a project, ordered by
// the admin-defined position (then by label as a tiebreaker).
func ListRoles(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Role, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, project_id, key, label, COALESCE(color, ''), position, created_at
		FROM roles
		WHERE project_id = $1
		ORDER BY position, label
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.Position, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRole appends a role to a project's catalogue. The new role
// receives the highest position so it lands at the bottom of the list.
func CreateRole(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key, label, color string) (*Role, error) {
	var r Role
	err := pool.QueryRow(ctx, `
		INSERT INTO roles (project_id, key, label, color, position)
		VALUES ($1, $2, $3, NULLIF($4, ''),
		        COALESCE((SELECT MAX(position) + 1 FROM roles WHERE project_id = $1), 0))
		RETURNING id, project_id, key, label, COALESCE(color, ''), position, created_at
	`, projectID, key, label, color).Scan(
		&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.Position, &r.CreatedAt,
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
		RETURNING id, project_id, key, label, COALESCE(color, ''), position, created_at
	`, id, label, color).Scan(
		&r.ID, &r.ProjectID, &r.Key, &r.Label, &r.Color, &r.Position, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// MoveRole rewrites the positions of every role in the project so the
// caller's full ordering is honoured atomically. orderedRoleIDs is the
// desired top-to-bottom sequence; any role belonging to the project
// that is missing from the list keeps its current relative position
// but is appended at the end.
func MoveRole(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, orderedRoleIDs []uuid.UUID) error {
	if len(orderedRoleIDs) == 0 {
		return errors.New("empty ordering")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Validate every id belongs to the project.
	rows, err := tx.Query(ctx, `SELECT id FROM roles WHERE project_id = $1`, projectID)
	if err != nil {
		return err
	}
	known := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		known[id] = true
	}
	rows.Close()

	seen := map[uuid.UUID]bool{}
	pos := 0
	for _, id := range orderedRoleIDs {
		if !known[id] {
			return errors.New("role does not belong to project: " + id.String())
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.Exec(ctx, `UPDATE roles SET position = $1 WHERE id = $2`, pos, id); err != nil {
			return err
		}
		pos++
	}
	// Anything not mentioned keeps trailing slots in creation order so a
	// partial reorder doesn't drop roles off the bottom.
	rows2, err := tx.Query(ctx, `
		SELECT id FROM roles
		WHERE project_id = $1
		ORDER BY created_at, id
	`, projectID)
	if err != nil {
		return err
	}
	var trailing []uuid.UUID
	for rows2.Next() {
		var id uuid.UUID
		if err := rows2.Scan(&id); err != nil {
			rows2.Close()
			return err
		}
		if !seen[id] {
			trailing = append(trailing, id)
		}
	}
	rows2.Close()
	for _, id := range trailing {
		if _, err := tx.Exec(ctx, `UPDATE roles SET position = $1 WHERE id = $2`, pos, id); err != nil {
			return err
		}
		pos++
	}

	return tx.Commit(ctx)
}

// DeleteRole removes a role. Memberships referencing it cascade.
func DeleteRole(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}
