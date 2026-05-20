package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ListRoles returns the role catalogue of a project, ordered by
// the admin-defined position (then by label as a tiebreaker).
func ListRoles(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]Role, error) {
	rows, err := dbq.New(pool).ListProjectRoles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Role, 0, len(rows))
	for _, r := range rows {
		out = append(out, Role{
			ID:        r.ID,
			ProjectID: r.ProjectID,
			Key:       r.Key,
			Label:     r.Label,
			Color:     r.Color,
			Position:  int(r.Position),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// CreateRole appends a role to a project's catalogue. The new role
// receives the highest position so it lands at the bottom of the list.
func CreateRole(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key, label, color string) (*Role, error) {
	row, err := dbq.New(pool).InsertProjectRole(ctx, dbq.InsertProjectRoleParams{
		ProjectID: projectID,
		Key:       key,
		Label:     label,
		Color:     color,
	})
	if err != nil {
		return nil, err
	}
	return &Role{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Key:       row.Key,
		Label:     row.Label,
		Color:     row.Color,
		Position:  int(row.Position),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// UpdateRole edits the label and color of a role.
func UpdateRole(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, label, color string) (*Role, error) {
	row, err := dbq.New(pool).UpdateProjectRole(ctx, dbq.UpdateProjectRoleParams{
		ID: id, Label: label, Color: color,
	})
	if err != nil {
		return nil, err
	}
	return &Role{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Key:       row.Key,
		Label:     row.Label,
		Color:     row.Color,
		Position:  int(row.Position),
		CreatedAt: row.CreatedAt.Time,
	}, nil
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
	q := dbq.New(tx)

	allIDs, err := q.ListProjectRoleIDs(ctx, projectID)
	if err != nil {
		return err
	}
	known := make(map[uuid.UUID]bool, len(allIDs))
	for _, id := range allIDs {
		known[id] = true
	}

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
		if err := q.SetRolePosition(ctx, dbq.SetRolePositionParams{ID: id, Position: int32(pos)}); err != nil {
			return err
		}
		pos++
	}
	// Anything not mentioned keeps trailing slots in creation order so a
	// partial reorder doesn't drop roles off the bottom.
	for _, id := range allIDs {
		if seen[id] {
			continue
		}
		if err := q.SetRolePosition(ctx, dbq.SetRolePositionParams{ID: id, Position: int32(pos)}); err != nil {
			return err
		}
		pos++
	}

	return tx.Commit(ctx)
}

// DeleteRole removes a role. Memberships referencing it cascade.
func DeleteRole(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	return dbq.New(pool).DeleteProjectRole(ctx, id)
}
