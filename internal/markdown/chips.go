package markdown

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
)

// resolveTaskChip resolves [[task:<short-or-full-id>]] against the
// project. The href format matches the kanban board's task-detail
// deep-link convention.
func resolveTaskChip(ctx context.Context, pool *pgxpool.Pool, ref string, projectID uuid.UUID) string {
	// Accept full UUID or short prefix; LIKE pattern is "prefix%".
	prefix := ref + "%"
	row, err := dbq.New(pool).GetTaskChipByShortID(ctx, dbq.GetTaskChipByShortIDParams{
		ProjectID: projectID,
		Prefix:    prefix,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return missingChip("task", ref, "no task with that id prefix in this project")
	}
	if err != nil {
		return missingChip("task", ref, "lookup error")
	}
	short := row.ID.String()
	if len(short) >= 8 {
		short = short[:8]
	}
	label := fmt.Sprintf("#%s · %s", short, row.Title)
	href := fmt.Sprintf("/projects/%s/board/kanban#task=%s",
		pathEscape(projectID.String()), pathEscape(row.ID.String()))
	return chipLink("task", href, ref, label, "chip-state-"+row.State)
}

// resolveDocChip resolves [[doc:<path>]] against the project's
// documents. The path is the same form users see in the docs reader
// (e.g. "projects/<id>/context/glossary.md").
func resolveDocChip(ctx context.Context, pool *pgxpool.Pool, ref string, projectID uuid.UUID) string {
	row, err := dbq.New(pool).GetDocChipByPath(ctx, dbq.GetDocChipByPathParams{
		ProjectID: projectID,
		Path:      ref,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return missingChip("doc", ref, "no document with that path in this project")
	}
	if err != nil {
		return missingChip("doc", ref, "lookup error")
	}
	title := row.Title
	if title == "" {
		title = ref
	}
	label := fmt.Sprintf("📄 %s", title)
	href := fmt.Sprintf("/projects/%s/docs#path=%s",
		pathEscape(projectID.String()), pathEscape(ref))
	return chipLink("doc", href, ref, label, "")
}

// resolveArchChip resolves [[arch:<slug>]] against the architecture
// graph.
func resolveArchChip(ctx context.Context, pool *pgxpool.Pool, ref string, projectID uuid.UUID) string {
	row, err := dbq.New(pool).GetArchChipBySlug(ctx, dbq.GetArchChipBySlugParams{
		ProjectID: projectID,
		Slug:      ref,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return missingChip("arch", ref, "no architecture node with that slug")
	}
	if err != nil {
		return missingChip("arch", ref, "lookup error")
	}
	label := fmt.Sprintf("◆ %s", row.Name)
	href := fmt.Sprintf("/projects/%s/arch/diagram#slug=%s",
		pathEscape(projectID.String()), pathEscape(ref))
	return chipLink("arch", href, ref, label, "")
}
