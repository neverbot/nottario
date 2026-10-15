package docs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// PathTakenError is returned by Move when the destination path already
// holds a document. Deleted is set when that document is soft-deleted:
// it still owns the path, and rewriting it would bring it back.
type PathTakenError struct {
	Path    string
	Deleted bool
}

func (e *PathTakenError) Error() string {
	if e.Deleted {
		return fmt.Sprintf("%s belongs to a deleted document; writing to it restores that document, so pick another path", e.Path)
	}
	return fmt.Sprintf("a document already exists at %s", e.Path)
}

// MoveParams carries the input to move a document to a new path.
type MoveParams struct {
	Scope           Scope
	ProjectID       *uuid.UUID
	Path            string
	NewPath         string
	Message         string
	ExpectedVersion *int
}

// Move gives a document a new path in place: same document, same
// history, plus one version that records the move. Architecture nodes
// that linked the old path are pointed at the new one in the same
// transaction, so nothing that referred to the document is left
// dangling. There is no redirect: the old path is free afterwards.
func Move(ctx context.Context, pool *pgxpool.Pool, p MoveParams, by Authorship) (*Document, error) {
	if err := validateScope(p.Scope, p.ProjectID); err != nil {
		return nil, err
	}
	p.NewPath = strings.TrimSpace(p.NewPath)
	if strings.TrimSpace(p.Path) == "" || p.NewPath == "" {
		return nil, ErrPathRequired
	}
	if p.NewPath == p.Path {
		return nil, errors.New("new_path is the document's current path")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	row, err := q.GetDocumentForMove(ctx, dbq.GetDocumentForMoveParams{
		Scope: string(p.Scope), ProjectID: p.ProjectID, Path: p.Path,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if p.ExpectedVersion != nil && *p.ExpectedVersion != int(row.CurrentVersion) {
		return nil, &VersionConflictError{CurrentVersion: int(row.CurrentVersion)}
	}

	holder, err := q.GetDocumentPathHolder(ctx, dbq.GetDocumentPathHolderParams{
		Scope: string(p.Scope), ProjectID: p.ProjectID, Path: p.NewPath,
	})
	switch {
	case err == nil:
		return nil, &PathTakenError{Path: p.NewPath, Deleted: holder.Deleted}
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	// A document without a title of its own shows its path; keep it
	// showing the path it now has.
	title := row.Title
	if title == p.Path {
		title = p.NewPath
	}
	message := p.Message
	if message == "" {
		message = "moved from " + p.Path
	}
	newVersion := row.CurrentVersion + 1

	moved, err := q.MoveDocument(ctx, dbq.MoveDocumentParams{
		ID:               row.ID,
		NewPath:          p.NewPath,
		Title:            title,
		CurrentVersion:   newVersion,
		UpdatedByUserID:  by.UserID,
		UpdatedByTokenID: by.TokenID,
	})
	if err != nil {
		// A concurrent write or move claimed new_path after the check.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, &PathTakenError{Path: p.NewPath}
		}
		return nil, err
	}
	if err := q.InsertDocumentVersion(ctx, dbq.InsertDocumentVersionParams{
		DocumentID:    row.ID,
		Version:       newVersion,
		Title:         title,
		Description:   row.Description,
		ContentMd:     row.ContentMd,
		Frontmatter:   row.Frontmatter,
		Message:       message,
		AuthorUserID:  by.UserID,
		AuthorTokenID: by.TokenID,
	}); err != nil {
		return nil, err
	}

	// Architecture links name project documents by path. This is a
	// reference fix-up, not an edit of the graph, so it does not take
	// the architecture session lock.
	if p.Scope == ScopeProject {
		if err := q.RepathArchDocLinks(ctx, dbq.RepathArchDocLinksParams{
			ProjectID: *p.ProjectID, OldPath: p.Path, NewPath: p.NewPath,
		}); err != nil {
			return nil, err
		}
		if err := q.DeleteArchDocLinksTo(ctx, dbq.DeleteArchDocLinksToParams{
			ProjectID: *p.ProjectID, Path: p.Path,
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	d := documentFromUpdateRow(dbq.UpdateDocumentRow(moved))
	if err := enrichDocViaMCP(ctx, pool, []*Document{&d}); err != nil {
		return nil, err
	}
	return &d, nil
}
