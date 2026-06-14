package docs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Errors returned by this package.
var (
	ErrNotFound        = errors.New("document not found")
	ErrVersionConflict = errors.New("expected_version does not match current_version")
	ErrInvalidScope    = errors.New("invalid scope")
	ErrInvalidKind     = errors.New("invalid kind")
	ErrPathRequired    = errors.New("path is required")
)

// VersionConflictError is returned by Write/Delete when the caller's
// expected_version does not match the row's current_version. It
// satisfies errors.Is(err, ErrVersionConflict) so existing callers
// continue to match, while exposing the actual current_version so
// API layers can return a structured 409 payload.
type VersionConflictError struct {
	CurrentVersion int `json:"current_version"`
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("expected_version does not match current_version (current=%d)", e.CurrentVersion)
}

func (e *VersionConflictError) Is(target error) bool {
	return target == ErrVersionConflict
}

// Authorship records who performed an operation.
type Authorship struct {
	UserID  *uuid.UUID
	TokenID *uuid.UUID
}

// WriteParams carries the input to create or update a document.
type WriteParams struct {
	Scope           Scope
	ProjectID       *uuid.UUID
	Path            string
	ContentMD       string
	Kind            Kind
	Message         string
	ExpectedVersion *int
}

// Write creates a new document or updates an existing one keyed by
// (scope, project_id, path).
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
	q := dbq.New(tx)

	existing, err := q.GetDocumentByPathForUpdate(ctx, dbq.GetDocumentByPathForUpdateParams{
		Scope:     string(p.Scope),
		ProjectID: p.ProjectID,
		Path:      p.Path,
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if p.ExpectedVersion != nil && *p.ExpectedVersion != 0 {
			return nil, &VersionConflictError{CurrentVersion: 0}
		}
		row, err := q.InsertDocument(ctx, dbq.InsertDocumentParams{
			Scope:            string(p.Scope),
			ProjectID:        p.ProjectID,
			Path:             p.Path,
			Kind:             string(kind),
			Title:            title,
			Description:      description,
			ContentMd:        body,
			Frontmatter:      fmJSON,
			CreatedByUserID:  by.UserID,
			CreatedByTokenID: by.TokenID,
		})
		if err != nil {
			return nil, err
		}
		d := documentFromInsertRow(row)
		if err := q.InsertDocumentVersion(ctx, dbq.InsertDocumentVersionParams{
			DocumentID:    d.ID,
			Version:       1,
			Title:         title,
			Description:   description,
			ContentMd:     body,
			Frontmatter:   fmJSON,
			Message:       p.Message,
			AuthorUserID:  by.UserID,
			AuthorTokenID: by.TokenID,
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &d, nil

	case err != nil:
		return nil, err
	}

	if p.ExpectedVersion != nil && *p.ExpectedVersion != int(existing.CurrentVersion) {
		return nil, &VersionConflictError{CurrentVersion: int(existing.CurrentVersion)}
	}
	newVersion := int(existing.CurrentVersion) + 1

	row, err := q.UpdateDocument(ctx, dbq.UpdateDocumentParams{
		ID:               existing.ID,
		Kind:             string(kind),
		Title:            title,
		Description:      description,
		ContentMd:        body,
		Frontmatter:      fmJSON,
		CurrentVersion:   int32(newVersion),
		UpdatedByUserID:  by.UserID,
		UpdatedByTokenID: by.TokenID,
	})
	if err != nil {
		return nil, err
	}
	d := documentFromUpdateRow(row)
	if err := q.InsertDocumentVersion(ctx, dbq.InsertDocumentVersionParams{
		DocumentID:    d.ID,
		Version:       int32(newVersion),
		Title:         title,
		Description:   description,
		ContentMd:     body,
		Frontmatter:   fmJSON,
		Message:       p.Message,
		AuthorUserID:  by.UserID,
		AuthorTokenID: by.TokenID,
	}); err != nil {
		// Belt-and-braces: the FOR UPDATE read above already
		// serialises writers, but if a future caller bypasses the
		// lock path the unique constraint on
		// (document_id, version) is the next line of defence —
		// translate it to ErrVersionConflict so the contract holds.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, &VersionConflictError{CurrentVersion: int(existing.CurrentVersion)}
		}
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
	row, err := dbq.New(pool).ReadDocument(ctx, dbq.ReadDocumentParams{
		Scope:     string(scope),
		ProjectID: projectID,
		Path:      path,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d := documentFromReadRow(row)
	return &d, nil
}

// ListFilter narrows a List call.
type ListFilter struct {
	Scope      Scope
	ProjectID  *uuid.UUID
	PathPrefix string
	Kind       Kind
}

// Summary is a lightweight document view returned by List.
type Summary struct {
	ID               uuid.UUID  `json:"id"`
	Scope            Scope      `json:"scope"`
	ProjectID        *uuid.UUID `json:"project_id"`
	Path             string     `json:"path"`
	Kind             Kind       `json:"kind"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	CurrentVersion   int        `json:"current_version"`
	UpdatedByUserID  *uuid.UUID `json:"updated_by_user_id"`
	UpdatedByTokenID *uuid.UUID `json:"-"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// List returns lightweight document summaries.
func List(ctx context.Context, pool *pgxpool.Pool, f ListFilter) ([]Summary, error) {
	if err := validateScope(f.Scope, f.ProjectID); err != nil {
		return nil, err
	}
	var pathPrefix pgtype.Text
	if f.PathPrefix != "" {
		pathPrefix = pgtype.Text{String: f.PathPrefix + "%", Valid: true}
	}
	var kind pgtype.Text
	if f.Kind != "" {
		kind = pgtype.Text{String: string(f.Kind), Valid: true}
	}
	rows, err := dbq.New(pool).ListDocuments(ctx, dbq.ListDocumentsParams{
		Scope:      string(f.Scope),
		ProjectID:  f.ProjectID,
		PathPrefix: pathPrefix,
		Kind:       kind,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, Summary{
			ID:               r.ID,
			Scope:            Scope(r.Scope),
			ProjectID:        r.ProjectID,
			Path:             r.Path,
			Kind:             Kind(r.Kind),
			Title:            r.Title,
			Description:      r.Description,
			CurrentVersion:   int(r.CurrentVersion),
			UpdatedByUserID:  r.UpdatedByUserID,
			UpdatedByTokenID: r.UpdatedByTokenID,
			UpdatedAt:        r.UpdatedAt.Time,
		})
	}
	return out, nil
}

// SearchFilter narrows a Search call.
type SearchFilter struct {
	Scope     Scope
	ProjectID *uuid.UUID
	Kind      Kind
}

// SearchHit is a search result with its rank.
type SearchHit struct {
	Summary
	Rank float32 `json:"rank"`
}

// Search returns documents matching the FTS query, ordered by rank.
func Search(ctx context.Context, pool *pgxpool.Pool, query string, f SearchFilter) ([]SearchHit, error) {
	if err := validateScope(f.Scope, f.ProjectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	var kind pgtype.Text
	if f.Kind != "" {
		kind = pgtype.Text{String: string(f.Kind), Valid: true}
	}
	rows, err := dbq.New(pool).SearchDocuments(ctx, dbq.SearchDocumentsParams{
		Query:     query,
		Scope:     string(f.Scope),
		ProjectID: f.ProjectID,
		Kind:      kind,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchHit{
			Summary: Summary{
				ID:               r.ID,
				Scope:            Scope(r.Scope),
				ProjectID:        r.ProjectID,
				Path:             r.Path,
				Kind:             Kind(r.Kind),
				Title:            r.Title,
				Description:      r.Description,
				CurrentVersion:   int(r.CurrentVersion),
				UpdatedByUserID:  r.UpdatedByUserID,
				UpdatedByTokenID: r.UpdatedByTokenID,
				UpdatedAt:        r.UpdatedAt.Time,
			},
			Rank: r.Rank,
		})
	}
	return out, nil
}

// VersionSummary is one version row without its body.
type VersionSummary struct {
	Version       int        `json:"version"`
	Title         string     `json:"title"`
	Message       string     `json:"message"`
	AuthorUserID  *uuid.UUID `json:"author_user_id"`
	AuthorTokenID *uuid.UUID `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
}

// History returns the version metadata for a document, newest first.
func History(ctx context.Context, pool *pgxpool.Pool, documentID uuid.UUID) ([]VersionSummary, error) {
	rows, err := dbq.New(pool).ListDocumentVersions(ctx, documentID)
	if err != nil {
		return nil, err
	}
	out := make([]VersionSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, VersionSummary{
			Version:       int(r.Version),
			Title:         r.Title,
			Message:       r.Message,
			AuthorUserID:  r.AuthorUserID,
			AuthorTokenID: r.AuthorTokenID,
			CreatedAt:     r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ReadVersion returns a specific historical version of a document.
func ReadVersion(ctx context.Context, pool *pgxpool.Pool, documentID uuid.UUID, version int) (*Version, error) {
	row, err := dbq.New(pool).GetDocumentVersion(ctx, dbq.GetDocumentVersionParams{
		DocumentID: documentID,
		Version:    int32(version),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fm, err := decodeFrontmatter(row.Frontmatter)
	if err != nil {
		return nil, err
	}
	return &Version{
		ID:            row.ID,
		DocumentID:    row.DocumentID,
		Version:       int(row.Version),
		Title:         row.Title,
		Description:   row.Description,
		ContentMD:     row.ContentMd,
		Frontmatter:   fm,
		Message:       row.Message,
		AuthorUserID:  row.AuthorUserID,
		AuthorTokenID: row.AuthorTokenID,
		CreatedAt:     row.CreatedAt.Time,
	}, nil
}

// DeleteParams carries the input to soft-delete a document. When
// ExpectedVersion is non-nil it must match the row's current_version
// before the delete proceeds, mirroring Write's optimistic
// concurrency check.
type DeleteParams struct {
	Scope           Scope
	ProjectID       *uuid.UUID
	Path            string
	Message         string
	ExpectedVersion *int
}

// Delete soft-deletes the document. Backwards-compatible wrapper
// kept for the few call sites that don't yet pass DeleteParams.
func Delete(ctx context.Context, pool *pgxpool.Pool, scope Scope, projectID *uuid.UUID, path, message string, by Authorship) error {
	return DeleteWithParams(ctx, pool, DeleteParams{
		Scope: scope, ProjectID: projectID, Path: path, Message: message,
	}, by)
}

// DeleteWithParams is the structured variant of Delete that supports
// optimistic concurrency via ExpectedVersion.
func DeleteWithParams(ctx context.Context, pool *pgxpool.Pool, p DeleteParams, by Authorship) error {
	if err := validateScope(p.Scope, p.ProjectID); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	row, err := q.GetDocumentForDelete(ctx, dbq.GetDocumentForDeleteParams{
		Scope:     string(p.Scope),
		ProjectID: p.ProjectID,
		Path:      p.Path,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if p.ExpectedVersion != nil && *p.ExpectedVersion != int(row.CurrentVersion) {
		return &VersionConflictError{CurrentVersion: int(row.CurrentVersion)}
	}

	message := p.Message
	newVersion := int(row.CurrentVersion) + 1
	if err := q.SoftDeleteDocument(ctx, dbq.SoftDeleteDocumentParams{
		ID:               row.ID,
		CurrentVersion:   int32(newVersion),
		UpdatedByUserID:  by.UserID,
		UpdatedByTokenID: by.TokenID,
	}); err != nil {
		return err
	}
	if err := q.InsertDocumentVersion(ctx, dbq.InsertDocumentVersionParams{
		DocumentID:    row.ID,
		Version:       int32(newVersion),
		Title:         row.Title,
		Description:   row.Description,
		ContentMd:     row.ContentMd,
		Frontmatter:   row.Frontmatter,
		Message:       message,
		AuthorUserID:  by.UserID,
		AuthorTokenID: by.TokenID,
	}); err != nil {
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

func decodeFrontmatter(raw []byte) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func documentFromInsertRow(r dbq.InsertDocumentRow) Document {
	fm, _ := decodeFrontmatter(r.Frontmatter)
	return Document{
		ID:               r.ID,
		Scope:            Scope(r.Scope),
		ProjectID:        r.ProjectID,
		Path:             r.Path,
		Kind:             Kind(r.Kind),
		Title:            r.Title,
		Description:      r.Description,
		ContentMD:        r.ContentMd,
		Frontmatter:      fm,
		CurrentVersion:   int(r.CurrentVersion),
		DeletedAt:        timestampPtr(r.DeletedAt),
		CreatedByUserID:  r.CreatedByUserID,
		CreatedByTokenID: r.CreatedByTokenID,
		UpdatedByUserID:  r.UpdatedByUserID,
		UpdatedByTokenID: r.UpdatedByTokenID,
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}

func documentFromUpdateRow(r dbq.UpdateDocumentRow) Document {
	fm, _ := decodeFrontmatter(r.Frontmatter)
	return Document{
		ID:               r.ID,
		Scope:            Scope(r.Scope),
		ProjectID:        r.ProjectID,
		Path:             r.Path,
		Kind:             Kind(r.Kind),
		Title:            r.Title,
		Description:      r.Description,
		ContentMD:        r.ContentMd,
		Frontmatter:      fm,
		CurrentVersion:   int(r.CurrentVersion),
		DeletedAt:        timestampPtr(r.DeletedAt),
		CreatedByUserID:  r.CreatedByUserID,
		CreatedByTokenID: r.CreatedByTokenID,
		UpdatedByUserID:  r.UpdatedByUserID,
		UpdatedByTokenID: r.UpdatedByTokenID,
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}

func documentFromReadRow(r dbq.ReadDocumentRow) Document {
	fm, _ := decodeFrontmatter(r.Frontmatter)
	return Document{
		ID:               r.ID,
		Scope:            Scope(r.Scope),
		ProjectID:        r.ProjectID,
		Path:             r.Path,
		Kind:             Kind(r.Kind),
		Title:            r.Title,
		Description:      r.Description,
		ContentMD:        r.ContentMd,
		Frontmatter:      fm,
		CurrentVersion:   int(r.CurrentVersion),
		DeletedAt:        timestampPtr(r.DeletedAt),
		CreatedByUserID:  r.CreatedByUserID,
		CreatedByTokenID: r.CreatedByTokenID,
		UpdatedByUserID:  r.UpdatedByUserID,
		UpdatedByTokenID: r.UpdatedByTokenID,
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}

func timestampPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	v := ts.Time
	return &v
}
