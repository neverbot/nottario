package arch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

var (
	ErrNodeNotFound  = errors.New("node not found")
	ErrInvalidKind   = errors.New("unknown kind for this project")
	ErrSlugRequired  = errors.New("slug is required")
	ErrNameRequired  = errors.New("name is required")
	ErrParentMissing = errors.New("parent node not found")
	ErrCycle         = errors.New("would create a cycle in the node tree")
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// UpsertParams is the input to UpsertNode.
type UpsertParams struct {
	Slug          string
	ParentSlug    string
	Kind          string
	Name          string
	DescriptionMD string
	Metadata      map[string]any
	LinkedRepo    string
	LinkedPath    string
	Position      *int
}

// UpsertNode creates or updates a node keyed by (project_id, slug).
// The write happens inside an arch session: it acquires (or extends)
// the per-project lock for `by.UserID` and records authorship on the
// row. Returns *LockedError when another user owns the active session.
func UpsertNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, p UpsertParams) (*Node, error) {
	if err := EnsureDefaultKinds(ctx, pool, projectID); err != nil {
		return nil, err
	}
	// Defensive: agents occasionally send HTML-entity-encoded strings
	// (`Pages &amp; Router`) because they are mentally building a
	// markup payload. Stored verbatim, those entities then get
	// re-escaped by the Lit renderer and show up to users as
	// `Pages &amp; Router` literal. Decode at the boundary so the row
	// holds plain UTF-8. Names that genuinely need the literal token
	// `&amp;` are vanishingly rare in architecture-node labels.
	p.Name = html.UnescapeString(p.Name)
	p.DescriptionMD = html.UnescapeString(p.DescriptionMD)
	if p.Slug == "" {
		return nil, ErrSlugRequired
	}
	if !slugRe.MatchString(p.Slug) {
		return nil, errors.New("slug must match [a-z0-9][a-z0-9._-]*")
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, ErrNameRequired
	}

	if p.Metadata == nil {
		p.Metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	position := int32(0)
	if p.Position != nil {
		position = int32(*p.Position)
	}

	var out *Node
	err = withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		ok, err := kindExistsTx(ctx, q, projectID, p.Kind)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %q", ErrInvalidKind, p.Kind)
		}

		var parentID *uuid.UUID
		if p.ParentSlug != "" {
			pid, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: p.ParentSlug})
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrParentMissing
			}
			if err != nil {
				return err
			}
			parentID = &pid
		}

		existingID, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: p.Slug})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			row, err := q.InsertArchNode(ctx, dbq.InsertArchNodeParams{
				ProjectID:     projectID,
				Slug:          p.Slug,
				ParentID:      parentID,
				Kind:          p.Kind,
				Name:          p.Name,
				DescriptionMd: p.DescriptionMD,
				Metadata:      metadataJSON,
				LinkedRepo:    textOrNull(p.LinkedRepo),
				LinkedPath:    textOrNull(p.LinkedPath),
				Position:      position,
				AuthorUserID:  &by.UserID,
				AuthorTokenID: by.TokenID,
			})
			if err != nil {
				return err
			}
			n := nodeFromInsertRow(row)
			out = &n
			return nil
		case err != nil:
			return err
		}

		if parentID != nil && *parentID == existingID {
			return ErrCycle
		}
		if parentID != nil {
			cycle, err := q.ArchNodeCycleCheck(ctx, dbq.ArchNodeCycleCheckParams{
				NewParentID: *parentID,
				NodeID:      existingID,
				ProjectID:   projectID,
			})
			if err != nil {
				return err
			}
			if cycle {
				return ErrCycle
			}
		}
		row, err := q.UpdateArchNode(ctx, dbq.UpdateArchNodeParams{
			ID:            existingID,
			ParentID:      parentID,
			Kind:          p.Kind,
			Name:          p.Name,
			DescriptionMd: p.DescriptionMD,
			Metadata:      metadataJSON,
			LinkedRepo:    textOrNull(p.LinkedRepo),
			LinkedPath:    textOrNull(p.LinkedPath),
			Position:      position,
			AuthorUserID:  &by.UserID,
			AuthorTokenID: by.TokenID,
		})
		if err != nil {
			return err
		}
		n := nodeFromUpdateRow(row)
		out = &n
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetNode fetches a node by slug.
func GetNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, slug string) (*Node, error) {
	row, err := dbq.New(pool).GetArchNodeBySlug(ctx, dbq.GetArchNodeBySlugParams{ProjectID: projectID, Slug: slug})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, err
	}
	n := nodeFromGetRow(row)
	return &n, nil
}

// ListNodes returns nodes optionally filtered by parent.
func ListNodes(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, parentSlug string, rootOnly bool) ([]Node, error) {
	var parent pgtype.Text
	if parentSlug != "" {
		parent = pgtype.Text{String: parentSlug, Valid: true}
	}
	rows, err := dbq.New(pool).ListArchNodes(ctx, dbq.ListArchNodesParams{
		ProjectID:  projectID,
		ParentSlug: parent,
		RootOnly:   rootOnly,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(rows))
	for _, r := range rows {
		out = append(out, nodeFromListRow(r))
	}
	return out, nil
}

// RemoveNode deletes a node. Runs inside an arch session.
func RemoveNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, slug string, cascade bool) error {
	return withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		row, err := q.GetArchNodeBySlug(ctx, dbq.GetArchNodeBySlugParams{ProjectID: projectID, Slug: slug})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNodeNotFound
		}
		if err != nil {
			return err
		}
		if !cascade {
			children, err := q.CountArchNodeChildren(ctx, &row.ID)
			if err != nil {
				return err
			}
			if children > 0 {
				return fmt.Errorf("node has %d children — pass cascade=true to delete the subtree", children)
			}
		}
		return q.DeleteArchNode(ctx, row.ID)
	})
}

// MoveNode reparents a node. Runs inside an arch session.
func MoveNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, by Authorship, slug, parentSlug string) (*Node, error) {
	var out *Node
	err := withSession(ctx, pool, projectID, by, func(tx pgx.Tx, q *dbq.Queries) error {
		nodeID, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: slug})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNodeNotFound
		}
		if err != nil {
			return err
		}

		var parentID *uuid.UUID
		if parentSlug != "" {
			pid, err := q.GetArchNodeIDBySlug(ctx, dbq.GetArchNodeIDBySlugParams{ProjectID: projectID, Slug: parentSlug})
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrParentMissing
			}
			if err != nil {
				return err
			}
			if pid == nodeID {
				return ErrCycle
			}
			cycle, err := q.ArchNodeCycleCheck(ctx, dbq.ArchNodeCycleCheckParams{
				NewParentID: pid,
				NodeID:      nodeID,
				ProjectID:   projectID,
			})
			if err != nil {
				return err
			}
			if cycle {
				return ErrCycle
			}
			parentID = &pid
		}

		row, err := q.MoveArchNode(ctx, dbq.MoveArchNodeParams{
			ID:            nodeID,
			ParentID:      parentID,
			AuthorUserID:  &by.UserID,
			AuthorTokenID: by.TokenID,
		})
		if err != nil {
			return err
		}
		n := nodeFromMoveRow(row)
		out = &n
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func decodeMetadata(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func nodeFromInsertRow(r dbq.InsertArchNodeRow) Node {
	return Node{
		ID: r.ID, ProjectID: r.ProjectID, Slug: r.Slug, ParentID: r.ParentID,
		Kind: r.Kind, Name: r.Name, DescriptionMD: r.DescriptionMd,
		Metadata:   decodeMetadata(r.Metadata),
		LinkedRepo: textPtr(r.LinkedRepo), LinkedPath: textPtr(r.LinkedPath),
		Position:  int(r.Position),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func nodeFromUpdateRow(r dbq.UpdateArchNodeRow) Node {
	return Node{
		ID: r.ID, ProjectID: r.ProjectID, Slug: r.Slug, ParentID: r.ParentID,
		Kind: r.Kind, Name: r.Name, DescriptionMD: r.DescriptionMd,
		Metadata:   decodeMetadata(r.Metadata),
		LinkedRepo: textPtr(r.LinkedRepo), LinkedPath: textPtr(r.LinkedPath),
		Position:  int(r.Position),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func nodeFromGetRow(r dbq.GetArchNodeBySlugRow) Node {
	return Node{
		ID: r.ID, ProjectID: r.ProjectID, Slug: r.Slug, ParentID: r.ParentID,
		Kind: r.Kind, Name: r.Name, DescriptionMD: r.DescriptionMd,
		Metadata:   decodeMetadata(r.Metadata),
		LinkedRepo: textPtr(r.LinkedRepo), LinkedPath: textPtr(r.LinkedPath),
		Position:  int(r.Position),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func nodeFromListRow(r dbq.ListArchNodesRow) Node {
	return Node{
		ID: r.ID, ProjectID: r.ProjectID, Slug: r.Slug, ParentID: r.ParentID,
		Kind: r.Kind, Name: r.Name, DescriptionMD: r.DescriptionMd,
		Metadata:   decodeMetadata(r.Metadata),
		LinkedRepo: textPtr(r.LinkedRepo), LinkedPath: textPtr(r.LinkedPath),
		Position:  int(r.Position),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func nodeFromMoveRow(r dbq.MoveArchNodeRow) Node {
	return Node{
		ID: r.ID, ProjectID: r.ProjectID, Slug: r.Slug, ParentID: r.ParentID,
		Kind: r.Kind, Name: r.Name, DescriptionMD: r.DescriptionMd,
		Metadata:   decodeMetadata(r.Metadata),
		LinkedRepo: textPtr(r.LinkedRepo), LinkedPath: textPtr(r.LinkedPath),
		Position:  int(r.Position),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}
