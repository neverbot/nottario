package docs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by this package.
var (
	ErrNotFound        = errors.New("document not found")
	ErrVersionConflict = errors.New("expected_version does not match current_version")
	ErrInvalidScope    = errors.New("invalid scope")
	ErrInvalidKind     = errors.New("invalid kind")
	ErrPathRequired    = errors.New("path is required")
)

// Authorship records who performed an operation.
type Authorship struct {
	UserID  *uuid.UUID
	TokenID *uuid.UUID
}

// WriteParams carries the input to create or update a document.
type WriteParams struct {
	Scope     Scope
	ProjectID *uuid.UUID
	Path      string
	// ContentMD is the markdown including its frontmatter (if any).
	// The server splits, parses and stores them separately while
	// keeping the body verbatim.
	ContentMD string
	// Kind defaults to the value parsed from frontmatter, then to
	// 'context' when neither is present. Pass it explicitly to
	// override.
	Kind Kind
	// Message is a short explanation stored on the version row.
	Message string
	// ExpectedVersion enforces optimistic concurrency on updates.
	// When nil, the write proceeds without a check (used for
	// creation).
	ExpectedVersion *int
}

// Write creates a new document or updates an existing one keyed by
// (scope, project_id, path). It maintains the document_versions
// history and runs the optimistic concurrency check.
func Write(ctx context.Context, pool *pgxpool.Pool, p WriteParams, by Authorship) (*Document, error) {
	if err := validateScope(p.Scope, p.ProjectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Path) == "" {
		return nil, ErrPathRequired
	}

	frontmatter, body, err := SplitFrontmatter(p.ContentMD)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if frontmatter == nil {
		frontmatter = map[string]any{}
	}
	kind := p.Kind
	if kind == "" {
		kind = KindFromFrontmatter(frontmatter)
	}
	if kind == "" {
		kind = KindContext
	}
	if !ValidKind(kind) {
		return nil, ErrInvalidKind
	}
	title := TitleFromFrontmatter(frontmatter)
	if title == "" {
		title = deriveTitleFromBody(body)
	}
	if title == "" {
		title = p.Path
	}
	description := DescriptionFromFrontmatter(frontmatter)
	fmJSON, err := json.Marshal(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Look up the existing row, if any.
	var existing struct {
		id             uuid.UUID
		currentVersion int
	}
	row := tx.QueryRow(ctx, `
		SELECT id, current_version
		FROM documents
		WHERE scope = $1 AND project_id IS NOT DISTINCT FROM $2 AND path = $3
	`, string(p.Scope), p.ProjectID, p.Path)
	err = row.Scan(&existing.id, &existing.currentVersion)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Create — expected_version (if provided) must equal 0.
		if p.ExpectedVersion != nil && *p.ExpectedVersion != 0 {
			return nil, ErrVersionConflict
		}
		var d Document
		err := tx.QueryRow(ctx, `
			INSERT INTO documents (
				scope, project_id, path, kind, title, description, content_md, frontmatter,
				current_version, created_by_user_id, created_by_token_id,
				updated_by_user_id, updated_by_token_id
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10, $9, $10)
			RETURNING id, scope, project_id, path, kind, title, description, content_md,
			          frontmatter, current_version, deleted_at,
			          created_by_user_id, created_by_token_id,
			          updated_by_user_id, updated_by_token_id,
			          created_at, updated_at
		`, string(p.Scope), p.ProjectID, p.Path, string(kind), title, description,
			body, fmJSON, by.UserID, by.TokenID,
		).Scan(
			&d.ID, &d.Scope, &d.ProjectID, &d.Path, &d.Kind, &d.Title, &d.Description, &d.ContentMD,
			&fmJSONBytes{m: &d.Frontmatter}, &d.CurrentVersion, &d.DeletedAt,
			&d.CreatedByUserID, &d.CreatedByTokenID,
			&d.UpdatedByUserID, &d.UpdatedByTokenID,
			&d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if err := insertVersion(ctx, tx, d.ID, 1, title, description, body, fmJSON, p.Message, by); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &d, nil

	case err != nil:
		return nil, err
	}

	// Update path.
	if p.ExpectedVersion != nil && *p.ExpectedVersion != existing.currentVersion {
		return nil, ErrVersionConflict
	}
	newVersion := existing.currentVersion + 1

	var d Document
	err = tx.QueryRow(ctx, `
		UPDATE documents
		SET kind = $2, title = $3, description = $4, content_md = $5, frontmatter = $6,
		    current_version = $7,
		    updated_by_user_id = $8, updated_by_token_id = $9,
		    deleted_at = NULL
		WHERE id = $1
		RETURNING id, scope, project_id, path, kind, title, description, content_md,
		          frontmatter, current_version, deleted_at,
		          created_by_user_id, created_by_token_id,
		          updated_by_user_id, updated_by_token_id,
		          created_at, updated_at
	`, existing.id, string(kind), title, description, body, fmJSON, newVersion, by.UserID, by.TokenID).Scan(
		&d.ID, &d.Scope, &d.ProjectID, &d.Path, &d.Kind, &d.Title, &d.Description, &d.ContentMD,
		&fmJSONBytes{m: &d.Frontmatter}, &d.CurrentVersion, &d.DeletedAt,
		&d.CreatedByUserID, &d.CreatedByTokenID,
		&d.UpdatedByUserID, &d.UpdatedByTokenID,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := insertVersion(ctx, tx, d.ID, newVersion, title, description, body, fmJSON, p.Message, by); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &d, nil
}

// Read fetches one document by (scope, project_id, path).
func Read(ctx context.Context, pool *pgxpool.Pool, scope Scope, projectID *uuid.UUID, path string) (*Document, error) {
	if err := validateScope(scope, projectID); err != nil {
		return nil, err
	}
	var d Document
	err := pool.QueryRow(ctx, `
		SELECT id, scope, project_id, path, kind, title, description, content_md,
		       frontmatter, current_version, deleted_at,
		       created_by_user_id, created_by_token_id,
		       updated_by_user_id, updated_by_token_id,
		       created_at, updated_at
		FROM documents
		WHERE scope = $1 AND project_id IS NOT DISTINCT FROM $2 AND path = $3
		  AND deleted_at IS NULL
	`, string(scope), projectID, path).Scan(
		&d.ID, &d.Scope, &d.ProjectID, &d.Path, &d.Kind, &d.Title, &d.Description, &d.ContentMD,
		&fmJSONBytes{m: &d.Frontmatter}, &d.CurrentVersion, &d.DeletedAt,
		&d.CreatedByUserID, &d.CreatedByTokenID,
		&d.UpdatedByUserID, &d.UpdatedByTokenID,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListFilter narrows a List call. ProjectID is required for scope =
// project; ignored for scope = global.
type ListFilter struct {
	Scope      Scope
	ProjectID  *uuid.UUID
	PathPrefix string
	Kind       Kind
}

// List returns documents matching the filter, lightweight rows (no
// content_md, no frontmatter — those need a Read call).
type Summary struct {
	ID               uuid.UUID
	Scope            Scope
	ProjectID        *uuid.UUID
	Path             string
	Kind             Kind
	Title            string
	Description      string
	CurrentVersion   int
	UpdatedByUserID  *uuid.UUID
	UpdatedByTokenID *uuid.UUID
	UpdatedAt        interface{}
}

// List returns lightweight document summaries.
func List(ctx context.Context, pool *pgxpool.Pool, f ListFilter) ([]Summary, error) {
	if err := validateScope(f.Scope, f.ProjectID); err != nil {
		return nil, err
	}
	q := `
		SELECT id, scope, project_id, path, kind, title, description, current_version,
		       updated_by_user_id, updated_by_token_id, updated_at
		FROM documents
		WHERE scope = $1 AND project_id IS NOT DISTINCT FROM $2 AND deleted_at IS NULL
	`
	args := []any{string(f.Scope), f.ProjectID}
	idx := 3
	if f.PathPrefix != "" {
		q += fmt.Sprintf(" AND path LIKE $%d", idx)
		args = append(args, f.PathPrefix+"%")
		idx++
	}
	if f.Kind != "" {
		q += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, string(f.Kind))
	}
	q += " ORDER BY path"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var s Summary
		if err := rows.Scan(
			&s.ID, &s.Scope, &s.ProjectID, &s.Path, &s.Kind, &s.Title, &s.Description,
			&s.CurrentVersion, &s.UpdatedByUserID, &s.UpdatedByTokenID, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SearchFilter narrows a Search call. Same semantics as ListFilter.
type SearchFilter struct {
	Scope     Scope
	ProjectID *uuid.UUID
	Kind      Kind
}

// SearchHit is a search result with its rank, suitable for display.
type SearchHit struct {
	Summary
	Rank float32
}

// Search returns documents matching the FTS query, ordered by rank.
// The query is parsed via plainto_tsquery for tolerance to user input.
func Search(ctx context.Context, pool *pgxpool.Pool, query string, f SearchFilter) ([]SearchHit, error) {
	if err := validateScope(f.Scope, f.ProjectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	q := `
		SELECT id, scope, project_id, path, kind, title, description, current_version,
		       updated_by_user_id, updated_by_token_id, updated_at,
		       ts_rank(search_vector, plainto_tsquery('simple', $3)) AS rank
		FROM documents
		WHERE scope = $1 AND project_id IS NOT DISTINCT FROM $2 AND deleted_at IS NULL
		  AND search_vector @@ plainto_tsquery('simple', $3)
	`
	args := []any{string(f.Scope), f.ProjectID, query}
	idx := 4
	if f.Kind != "" {
		q += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, string(f.Kind))
	}
	q += " ORDER BY rank DESC, path"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		var s SearchHit
		if err := rows.Scan(
			&s.ID, &s.Scope, &s.ProjectID, &s.Path, &s.Kind, &s.Title, &s.Description,
			&s.CurrentVersion, &s.UpdatedByUserID, &s.UpdatedByTokenID, &s.UpdatedAt,
			&s.Rank,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// History returns the version metadata for a document, newest first.
// Bodies are omitted; use ReadVersion to fetch one.
type VersionSummary struct {
	Version       int
	Title         string
	Message       string
	AuthorUserID  *uuid.UUID
	AuthorTokenID *uuid.UUID
	CreatedAt     interface{}
}

// History returns the version metadata for a document, newest first.
func History(ctx context.Context, pool *pgxpool.Pool, documentID uuid.UUID) ([]VersionSummary, error) {
	rows, err := pool.Query(ctx, `
		SELECT version, title, message, author_user_id, author_token_id, created_at
		FROM document_versions
		WHERE document_id = $1
		ORDER BY version DESC
	`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VersionSummary{}
	for rows.Next() {
		var v VersionSummary
		if err := rows.Scan(&v.Version, &v.Title, &v.Message, &v.AuthorUserID, &v.AuthorTokenID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReadVersion returns a specific historical version of a document.
func ReadVersion(ctx context.Context, pool *pgxpool.Pool, documentID uuid.UUID, version int) (*Version, error) {
	var v Version
	err := pool.QueryRow(ctx, `
		SELECT id, document_id, version, title, description, content_md, frontmatter, message,
		       author_user_id, author_token_id, created_at
		FROM document_versions
		WHERE document_id = $1 AND version = $2
	`, documentID, version).Scan(
		&v.ID, &v.DocumentID, &v.Version, &v.Title, &v.Description, &v.ContentMD,
		&fmJSONBytes{m: &v.Frontmatter}, &v.Message,
		&v.AuthorUserID, &v.AuthorTokenID, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Delete soft-deletes the document (rows in document_versions stay).
// Re-writing the same path resurrects it.
func Delete(ctx context.Context, pool *pgxpool.Pool, scope Scope, projectID *uuid.UUID, path, message string, by Authorship) error {
	if err := validateScope(scope, projectID); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var doc Document
	err = tx.QueryRow(ctx, `
		SELECT id, current_version, title, description, content_md, frontmatter
		FROM documents
		WHERE scope = $1 AND project_id IS NOT DISTINCT FROM $2 AND path = $3
		  AND deleted_at IS NULL
	`, string(scope), projectID, path).Scan(
		&doc.ID, &doc.CurrentVersion, &doc.Title, &doc.Description, &doc.ContentMD,
		&fmJSONBytes{m: &doc.Frontmatter},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	newVersion := doc.CurrentVersion + 1
	_, err = tx.Exec(ctx, `
		UPDATE documents
		SET deleted_at = now(),
		    current_version = $2,
		    updated_by_user_id = $3,
		    updated_by_token_id = $4
		WHERE id = $1
	`, doc.ID, newVersion, by.UserID, by.TokenID)
	if err != nil {
		return err
	}

	fmJSON, _ := json.Marshal(doc.Frontmatter)
	if err := insertVersion(ctx, tx, doc.ID, newVersion, doc.Title, doc.Description, doc.ContentMD, fmJSON, message, by); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateScope(s Scope, projectID *uuid.UUID) error {
	if !ValidScope(s) {
		return ErrInvalidScope
	}
	if s == ScopeProject && projectID == nil {
		return errors.New("project_id is required when scope=project")
	}
	if s == ScopeGlobal && projectID != nil {
		return errors.New("project_id must be empty when scope=global")
	}
	return nil
}

func insertVersion(ctx context.Context, tx pgx.Tx, documentID uuid.UUID, version int, title, description, body string, fmJSON []byte, message string, by Authorship) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO document_versions (
			document_id, version, title, description, content_md, frontmatter,
			message, author_user_id, author_token_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, documentID, version, title, description, body, fmJSON, message, by.UserID, by.TokenID)
	return err
}

func deriveTitleFromBody(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		if line != "" {
			break
		}
	}
	return ""
}

// fmJSONBytes is a pgx Scanner that decodes a jsonb column into a map.
type fmJSONBytes struct {
	m *map[string]any
}

func (s *fmJSONBytes) Scan(v any) error {
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
