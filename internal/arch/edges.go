package arch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
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
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	fromID, err := resolveSlugQ(ctx, q, projectID, p.FromSlug)
	if err != nil {
		return nil, err
	}
	toID, err := resolveSlugQ(ctx, q, projectID, p.ToSlug)
	if err != nil {
		return nil, err
	}
	row, err := q.UpsertArchEdge(ctx, dbq.UpsertArchEdgeParams{
		ProjectID:     projectID,
		FromNodeID:    fromID,
		ToNodeID:      toID,
		Kind:          p.Kind,
		Label:         p.Label,
		DescriptionMd: p.DescriptionMD,
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Edge{
		ID: row.ID, ProjectID: row.ProjectID,
		FromNodeID: row.FromNodeID, ToNodeID: row.ToNodeID,
		Kind: row.Kind, Label: row.Label, DescriptionMD: row.DescriptionMd,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// RemoveEdge deletes an edge by id.
func RemoveEdge(ctx context.Context, pool *pgxpool.Pool, projectID, edgeID uuid.UUID) error {
	rows, err := dbq.New(pool).DeleteArchEdge(ctx, dbq.DeleteArchEdgeParams{ProjectID: projectID, ID: edgeID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

// EdgeView is the shape returned by ListEdges.
type EdgeView struct {
	Edge
	FromSlug string `json:"from_slug"`
	FromName string `json:"from_name"`
	ToSlug   string `json:"to_slug"`
	ToName   string `json:"to_name"`
}

// EdgeFilter narrows ListEdges.
type EdgeFilter struct {
	NodeSlug  string
	Direction string
	Kind      string
}

// ListEdges returns edges of a project.
func ListEdges(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, f EdgeFilter) ([]EdgeView, error) {
	q := dbq.New(pool)
	var kind pgtype.Text
	if f.Kind != "" {
		kind = pgtype.Text{String: f.Kind, Valid: true}
	}
	var nodeID *uuid.UUID
	if f.NodeSlug != "" {
		id, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: f.NodeSlug})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("node not found: " + f.NodeSlug)
		}
		if err != nil {
			return nil, err
		}
		nodeID = &id
	}
	rows, err := q.ListArchEdges(ctx, dbq.ListArchEdgesParams{
		ProjectID: projectID,
		Kind:      kind,
		NodeID:    nodeID,
		Direction: f.Direction,
	})
	if err != nil {
		return nil, err
	}
	out := make([]EdgeView, 0, len(rows))
	for _, r := range rows {
		out = append(out, EdgeView{
			Edge: Edge{
				ID: r.ID, ProjectID: r.ProjectID,
				FromNodeID: r.FromNodeID, ToNodeID: r.ToNodeID,
				Kind: r.Kind, Label: r.Label, DescriptionMD: r.DescriptionMd,
				CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
			},
			FromSlug: r.FromSlug, FromName: r.FromName,
			ToSlug: r.ToSlug, ToName: r.ToName,
		})
	}
	return out, nil
}

func resolveSlugQ(ctx context.Context, q *dbq.Queries, projectID uuid.UUID, slug string) (uuid.UUID, error) {
	id, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: slug})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("node not found: " + slug)
	}
	return id, err
}
