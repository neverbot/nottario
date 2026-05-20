package arch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// LinkDoc associates a document path with a node.
func LinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug, docPath string) error {
	if docPath == "" {
		return errors.New("doc_path is required")
	}
	q := dbq.New(pool)
	nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
	if err != nil {
		return err
	}
	return q.InsertArchNodeLink(ctx, dbq.InsertArchNodeLinkParams{
		ProjectID: projectID, NodeID: nodeID, LinkType: "doc", TargetID: docPath,
	})
}

// UnlinkDoc removes the association.
func UnlinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug, docPath string) error {
	q := dbq.New(pool)
	nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
	if err != nil {
		return err
	}
	return q.DeleteArchNodeLink(ctx, dbq.DeleteArchNodeLinkParams{
		ProjectID: projectID, NodeID: nodeID, LinkType: "doc", TargetID: docPath,
	})
}

// LinkTask attaches a task uuid to a node.
func LinkTask(ctx context.Context, pool *pgxpool.Pool, projectID, nodeIDOrSlug, taskID uuid.UUID, nodeSlug string) error {
	q := dbq.New(pool)
	nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
	if err != nil {
		return err
	}
	return q.InsertArchNodeLink(ctx, dbq.InsertArchNodeLinkParams{
		ProjectID: projectID, NodeID: nodeID, LinkType: "task", TargetID: taskID.String(),
	})
}

// UnlinkTask removes the association.
func UnlinkTask(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug string, taskID uuid.UUID) error {
	q := dbq.New(pool)
	nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
	if err != nil {
		return err
	}
	return q.DeleteArchNodeLink(ctx, dbq.DeleteArchNodeLinkParams{
		ProjectID: projectID, NodeID: nodeID, LinkType: "task", TargetID: taskID.String(),
	})
}

// ListLinks returns every link of a node.
func ListLinks(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, nodeSlug string) ([]NodeLink, error) {
	q := dbq.New(pool)
	nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListArchNodeLinks(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]NodeLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, NodeLink{
			ProjectID: r.ProjectID,
			NodeID:    r.NodeID,
			LinkType:  r.LinkType,
			TargetID:  r.TargetID,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}
