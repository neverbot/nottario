package arch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// LinkDoc associates a document path with a node. Runs inside an arch session.
func LinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, nodeSlug, docPath string) error {
	if docPath == "" {
		return errors.New("doc_path is required")
	}
	return withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
		if err != nil {
			return err
		}
		return q.InsertArchNodeLink(ctx, dbq.InsertArchNodeLinkParams{
			ProjectID: projectID, NodeID: nodeID, LinkType: "doc", TargetID: docPath,
		})
	})
}

// UnlinkDoc removes the association. Runs inside an arch session.
func UnlinkDoc(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, nodeSlug, docPath string) error {
	return withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
		if err != nil {
			return err
		}
		return q.DeleteArchNodeLink(ctx, dbq.DeleteArchNodeLinkParams{
			ProjectID: projectID, NodeID: nodeID, LinkType: "doc", TargetID: docPath,
		})
	})
}

// LinkTask attaches a task uuid to a node. Runs inside an arch session.
func LinkTask(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, taskID uuid.UUID, nodeSlug string) error {
	return withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
		if err != nil {
			return err
		}
		return q.InsertArchNodeLink(ctx, dbq.InsertArchNodeLinkParams{
			ProjectID: projectID, NodeID: nodeID, LinkType: "task", TargetID: taskID.String(),
		})
	})
}

// UnlinkTask removes the association. Runs inside an arch session.
func UnlinkTask(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, nodeSlug string, taskID uuid.UUID) error {
	return withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		nodeID, err := resolveSlugQ(ctx, q, projectID, nodeSlug)
		if err != nil {
			return err
		}
		return q.DeleteArchNodeLink(ctx, dbq.DeleteArchNodeLinkParams{
			ProjectID: projectID, NodeID: nodeID, LinkType: "task", TargetID: taskID.String(),
		})
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
