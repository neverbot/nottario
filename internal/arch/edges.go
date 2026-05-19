package arch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEdgeNotFound is returned by lookups that find no row.
var ErrEdgeNotFound = errors.New("edge not found")

// EdgeUpsertParams is the input to UpsertEdge.
type EdgeUpsertParams struct {
	FromSlug      string
	ToSlug        string
	Kind          string
	Label         string
	DescriptionMD string
}

// UpsertEdge creates or updates an edge keyed by (from, to, kind).
// Self-loops are rejected. Slugs are resolved to node uuids inside
// the same transaction.
func UpsertEdge(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, p EdgeUpsertParams) (*Edge, error) {
	if p.FromSlug == "" || p.ToSlug == "" {
		return nil, errors.New("from and to slugs are required")
	}
	if p.Kind == "" {
		return nil, errors.New("kind is required")
	}
	if p.FromSlug == p.ToSlug {
		return nil, errors.New("self-loops are not allowed")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	fromID, err := resolveSlug(ctx, tx, projectID, p.FromSlug)
	if err != nil {
		return nil, err
	}
	toID, err := resolveSlug(ctx, tx, projectID, p.ToSlug)
	if err != nil {
		return nil, err
	}
	var e Edge
	err = tx.QueryRow(ctx, `
		INSERT INTO arch_edges (project_id, from_node_id, to_node_id, kind, label, description_md)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, from_node_id, to_node_id, kind) DO UPDATE
		SET label = EXCLUDED.label, description_md = EXCLUDED.description_md
		RETURNING id, project_id, from_node_id, to_node_id, kind, label, description_md, created_at, updated_at
	`, projectID, fromID, toID, p.Kind, p.Label, p.DescriptionMD).Scan(
		&e.ID, &e.ProjectID, &e.FromNodeID, &e.ToNodeID, &e.Kind, &e.Label, &e.DescriptionMD, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &e, nil
}

// RemoveEdge deletes an edge by id.
func RemoveEdge(ctx context.Context, pool *pgxpool.Pool, projectID, edgeID uuid.UUID) error {
	cmd, err := pool.Exec(ctx, `DELETE FROM arch_edges WHERE project_id = $1 AND id = $2`, projectID, edgeID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

// EdgeView is the shape returned by ListEdges: it includes the
// endpoints' slugs and names for convenient display.
type EdgeView struct {
	Edge
	FromSlug string
	FromName string
	ToSlug   string
	ToName   string
}

// EdgeFilter narrows ListEdges. NodeSlug, when set, returns edges
// incident to that node (controlled by Direction).
type EdgeFilter struct {
	NodeSlug  string
	Direction string // "out", "in", or "" for both
	Kind      string
}

// ListEdges returns edges of a project. Joined with arch_nodes to
// hydrate FromSlug / ToSlug / FromName / ToName.
func ListEdges(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, f EdgeFilter) ([]EdgeView, error) {
	q := `
		SELECT e.id, e.project_id, e.from_node_id, e.to_node_id, e.kind, e.label, e.description_md,
		       e.created_at, e.updated_at,
		       a.slug, a.name, b.slug, b.name
		FROM arch_edges e
		JOIN arch_nodes a ON a.id = e.from_node_id
		JOIN arch_nodes b ON b.id = e.to_node_id
		WHERE e.project_id = $1
	`
	args := []any{projectID}
	idx := 2
	if f.Kind != "" {
		q += " AND e.kind = $2"
		args = append(args, f.Kind)
		idx = 3
	}
	if f.NodeSlug != "" {
		nodeID, err := resolveSlugPool(ctx, pool, projectID, f.NodeSlug)
		if err != nil {
			return nil, err
		}
		switch f.Direction {
		case "out":
			q += " AND e.from_node_id = $" + itoa(idx)
			args = append(args, nodeID)
		case "in":
			q += " AND e.to_node_id = $" + itoa(idx)
			args = append(args, nodeID)
		default:
			q += " AND (e.from_node_id = $" + itoa(idx) + " OR e.to_node_id = $" + itoa(idx) + ")"
			args = append(args, nodeID)
		}
	}
	q += " ORDER BY e.kind, a.slug, b.slug"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EdgeView{}
	for rows.Next() {
		var v EdgeView
		if err := rows.Scan(
			&v.ID, &v.ProjectID, &v.FromNodeID, &v.ToNodeID, &v.Kind, &v.Label, &v.DescriptionMD,
			&v.CreatedAt, &v.UpdatedAt,
			&v.FromSlug, &v.FromName, &v.ToSlug, &v.ToName,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// resolveSlug returns the uuid for (project, slug) within an open tx.
func resolveSlug(ctx context.Context, q pgx.Tx, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("node not found: " + slug)
	}
	return id, err
}

// resolveSlugPool is the pool-bound twin of resolveSlug.
func resolveSlugPool(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("node not found: " + slug)
	}
	return id, err
}

// itoa avoids strconv import for the few small ints used in dynamic
// query construction here.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	if n < 100 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return "" // never used past 99 in this package
}
