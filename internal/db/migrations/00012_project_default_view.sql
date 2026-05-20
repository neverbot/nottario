-- +goose Up
-- Per-project default view: which surface (`board/kanban`, `board/gantt`,
-- `docs`, `arch/diagram`, …) the project list cards navigate to when the
-- whole card is clicked. The set of valid view keys is the source of truth
-- in the frontend's view registry (internal/web/static/views.js); the
-- server-side CHECK below mirrors that allowlist defensively. When we add
-- a new view, extend both the registry and this constraint via a follow-up
-- migration.

ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS default_view text NOT NULL DEFAULT 'board/kanban'
    CHECK (default_view IN ('board/kanban','board/gantt','docs','arch/diagram','arch/tree'));

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS default_view;
