package arch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LinkDoc associates a document path with a node.
func LinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug, docPath string) error {
	nodeID, err := resolveSlugPool(ctx, pool, projectID, nodeSlug)
	if err != nil {
		return err
	}
	if docPath == "" {
		return errors.New("doc_path is required")
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO arch_node_links (project_id, node_id, link_type, target_id)
		VALUES ($1, $2, 'doc', $3) ON CONFLICT DO NOTHING
	`, projectID, nodeID, docPath)
	return err
}

// UnlinkDoc removes the association.
func UnlinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug, docPath string) error {
	nodeID, err := resolveSlugPool(ctx, pool, projectID, nodeSlug)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		DELETE FROM arch_node_links
		WHERE project_id = $1 AND node_id = $2 AND link_type = 'doc' AND target_id = $3
	`, projectID, nodeID, docPath)
	return err
}

// LinkTask attaches a task uuid to a node.
func LinkTask(ctx context.Context, pool *pgxpool.Pool, projectID, nodeIDOrSlug, taskID uuid.UUID, nodeSlug string) error {
	nodeID, err := resolveSlugPool(ctx, pool, projectID, nodeSlug)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO arch_node_links (project_id, node_id, link_type, target_id)
		VALUES ($1, $2, 'task', $3) ON CONFLICT DO NOTHING
	`, projectID, nodeID, taskID.String())
	return err
}

// UnlinkTask removes the association.
func UnlinkTask(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug string, taskID uuid.UUID) error {
	nodeID, err := resolveSlugPool(ctx, pool, projectID, nodeSlug)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		DELETE FROM arch_node_links
		WHERE project_id = $1 AND node_id = $2 AND link_type = 'task' AND target_id = $3
	`, projectID, nodeID, taskID.String())
	return err
}

// ListLinks returns every link of a node.
func ListLinks(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug string) ([]NodeLink, error) {
	nodeID, err := resolveSlugPool(ctx, pool, projectID, nodeSlug)
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, node_id, link_type, target_id, created_at
		FROM arch_node_links WHERE node_id = $1
		ORDER BY link_type, target_id
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeLink{}
	for rows.Next() {
		var l NodeLink
		if err := rows.Scan(&l.ProjectID, &l.NodeID, &l.LinkType, &l.TargetID, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
