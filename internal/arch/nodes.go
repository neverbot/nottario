package arch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by node operations.
var (
	ErrNodeNotFound  = errors.New("node not found")
	ErrInvalidKind   = errors.New("unknown kind for this project")
	ErrSlugRequired  = errors.New("slug is required")
	ErrNameRequired  = errors.New("name is required")
	ErrParentMissing = errors.New("parent node not found")
	ErrCycle         = errors.New("would create a cycle in the node tree")
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// UpsertParams is the input to UpsertNode. When ID is zero a new node
// is created (keyed by slug); when ID is set the existing node is
// updated.
type UpsertParams struct {
	Slug          string
	ParentSlug    string // empty for a root node
	Kind          string
	Name          string
	DescriptionMD string
	Metadata      map[string]any
	LinkedRepo    string // pass "" to clear
	LinkedPath    string // pass "" to clear
	Position      *int
}

// UpsertNode creates or updates a node keyed by (project_id, slug).
// It validates the kind against the project's catalogue and checks
// that the optional parent_slug resolves and does not create a cycle.
func UpsertNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, p UpsertParams) (*Node, error) {
	if err := EnsureDefaultKinds(ctx, pool, projectID); err != nil {
		return nil, err
	}
	if p.Slug == "" {
		return nil, ErrSlugRequired
	}
	if !slugRe.MatchString(p.Slug) {
		return nil, errors.New("slug must match [a-z0-9][a-z0-9._-]*")
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, ErrNameRequired
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ok, err := kindExists(ctx, tx, projectID, p.Kind)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidKind, p.Kind)
	}

	var parentID *uuid.UUID
	if p.ParentSlug != "" {
		var pid uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, p.ParentSlug).Scan(&pid)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrParentMissing
		}
		if err != nil {
			return nil, err
		}
		parentID = &pid
	}

	if p.Metadata == nil {
		p.Metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	linkedRepo := nullableString(p.LinkedRepo)
	linkedPath := nullableString(p.LinkedPath)
	position := 0
	if p.Position != nil {
		position = *p.Position
	}

	// Detect an existing row for this slug.
	var existingID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, p.Slug).Scan(&existingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// create
		var n Node
		err := tx.QueryRow(ctx, `
			INSERT INTO arch_nodes (project_id, slug, parent_id, kind, name, description_md,
			                        metadata, linked_repo, linked_path, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, project_id, slug, parent_id, kind, name, description_md,
			          metadata, linked_repo, linked_path, position,
			          created_at, updated_at
		`, projectID, p.Slug, parentID, p.Kind, p.Name, p.DescriptionMD,
			metadataJSON, linkedRepo, linkedPath, position,
		).Scan(
			&n.ID, &n.ProjectID, &n.Slug, &n.ParentID, &n.Kind, &n.Name, &n.DescriptionMD,
			&metadataScanner{m: &n.Metadata}, &n.LinkedRepo, &n.LinkedPath, &n.Position,
			&n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &n, nil
	case err != nil:
		return nil, err
	}

	// update — guard against parent loop.
	if parentID != nil && *parentID == existingID {
		return nil, ErrCycle
	}
	if parentID != nil {
		cycle, err := wouldCreateCycle(ctx, tx, projectID, existingID, *parentID)
		if err != nil {
			return nil, err
		}
		if cycle {
			return nil, ErrCycle
		}
	}
	var n Node
	err = tx.QueryRow(ctx, `
		UPDATE arch_nodes
		SET parent_id = $2,
		    kind = $3,
		    name = $4,
		    description_md = $5,
		    metadata = $6,
		    linked_repo = $7,
		    linked_path = $8,
		    position = $9
		WHERE id = $1
		RETURNING id, project_id, slug, parent_id, kind, name, description_md,
		          metadata, linked_repo, linked_path, position,
		          created_at, updated_at
	`, existingID, parentID, p.Kind, p.Name, p.DescriptionMD,
		metadataJSON, linkedRepo, linkedPath, position,
	).Scan(
		&n.ID, &n.ProjectID, &n.Slug, &n.ParentID, &n.Kind, &n.Name, &n.DescriptionMD,
		&metadataScanner{m: &n.Metadata}, &n.LinkedRepo, &n.LinkedPath, &n.Position,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNode fetches a node by slug.
func GetNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, slug string) (*Node, error) {
	var n Node
	err := pool.QueryRow(ctx, `
		SELECT id, project_id, slug, parent_id, kind, name, description_md,
		       metadata, linked_repo, linked_path, position, created_at, updated_at
		FROM arch_nodes
		WHERE project_id = $1 AND slug = $2
	`, projectID, slug).Scan(
		&n.ID, &n.ProjectID, &n.Slug, &n.ParentID, &n.Kind, &n.Name, &n.DescriptionMD,
		&metadataScanner{m: &n.Metadata}, &n.LinkedRepo, &n.LinkedPath, &n.Position,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListNodes returns every node of a project. Optionally restrict to
// the direct children of one parent (pass parentSlug "" together with
// rootOnly=true to get root nodes).
func ListNodes(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, parentSlug string, rootOnly bool) ([]Node, error) {
	q := `
		SELECT id, project_id, slug, parent_id, kind, name, description_md,
		       metadata, linked_repo, linked_path, position, created_at, updated_at
		FROM arch_nodes
		WHERE project_id = $1
	`
	args := []any{projectID}
	if parentSlug != "" {
		q += ` AND parent_id = (SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2)`
		args = append(args, parentSlug)
	} else if rootOnly {
		q += ` AND parent_id IS NULL`
	}
	q += ` ORDER BY position, slug`

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.Slug, &n.ParentID, &n.Kind, &n.Name, &n.DescriptionMD,
			&metadataScanner{m: &n.Metadata}, &n.LinkedRepo, &n.LinkedPath, &n.Position,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// RemoveNode deletes the node (children cascade via FK). When cascade
// is false and the node has any children, the call is rejected.
func RemoveNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, slug string, cascade bool) error {
	n, err := GetNode(ctx, pool, projectID, slug)
	if err != nil {
		return err
	}
	if !cascade {
		var children int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM arch_nodes WHERE parent_id = $1`, n.ID).Scan(&children); err != nil {
			return err
		}
		if children > 0 {
			return fmt.Errorf("node has %d children — pass cascade=true to delete the subtree", children)
		}
	}
	_, err = pool.Exec(ctx, `DELETE FROM arch_nodes WHERE id = $1`, n.ID)
	return err
}

// MoveNode reparents a node. Use parentSlug = "" to make it a root.
func MoveNode(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, slug, parentSlug string) (*Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var nodeID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, slug).Scan(&nodeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}

	var parentID *uuid.UUID
	if parentSlug != "" {
		var pid uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2`, projectID, parentSlug).Scan(&pid)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrParentMissing
		}
		if err != nil {
			return nil, err
		}
		if pid == nodeID {
			return nil, ErrCycle
		}
		cycle, err := wouldCreateCycle(ctx, tx, projectID, nodeID, pid)
		if err != nil {
			return nil, err
		}
		if cycle {
			return nil, ErrCycle
		}
		parentID = &pid
	}

	var n Node
	err = tx.QueryRow(ctx, `
		UPDATE arch_nodes SET parent_id = $2 WHERE id = $1
		RETURNING id, project_id, slug, parent_id, kind, name, description_md,
		          metadata, linked_repo, linked_path, position, created_at, updated_at
	`, nodeID, parentID).Scan(
		&n.ID, &n.ProjectID, &n.Slug, &n.ParentID, &n.Kind, &n.Name, &n.DescriptionMD,
		&metadataScanner{m: &n.Metadata}, &n.LinkedRepo, &n.LinkedPath, &n.Position,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &n, nil
}

// wouldCreateCycle returns true when making `nodeID`'s parent point
// to `newParentID` would close a cycle (e.g. parent is a descendant
// of the node itself).
func wouldCreateCycle(ctx context.Context, q pgx.Tx, projectID, nodeID, newParentID uuid.UUID) (bool, error) {
	var hit bool
	err := q.QueryRow(ctx, `
		WITH RECURSIVE ancestors(id) AS (
			SELECT id FROM arch_nodes WHERE id = $1
			UNION
			SELECT a.parent_id FROM arch_nodes a
			JOIN ancestors r ON r.id = a.id
			WHERE a.parent_id IS NOT NULL
		)
		SELECT EXISTS (
			SELECT 1
			FROM ancestors anc
			WHERE anc.id IN (
				WITH RECURSIVE descendants(id) AS (
					SELECT id FROM arch_nodes WHERE id = $2
					UNION
					SELECT a.id FROM arch_nodes a
					JOIN descendants d ON d.id = a.parent_id
					WHERE a.project_id = $3
				)
				SELECT id FROM descendants
			)
		)
	`, newParentID, nodeID, projectID).Scan(&hit)
	if err != nil {
		return false, err
	}
	return hit, nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// metadataScanner decodes a jsonb column into a Go map.
type metadataScanner struct {
	m *map[string]any
}

func (s *metadataScanner) Scan(v any) error {
	if v == nil {
		*s.m = map[string]any{}
		return nil
	}
	var raw []byte
	switch x := v.(type) {
	case []byte:
		raw = x
	case string:
		raw = []byte(x)
	default:
		return fmt.Errorf("unexpected jsonb scan type %T", v)
	}
	out := map[string]any{}
	if len(raw) == 0 {
		*s.m = out
		return nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*s.m = out
	return nil
}
