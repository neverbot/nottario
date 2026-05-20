-- +goose Up
-- Per-project MCP pagination page size and the index that backs the
-- keyset cursor on tasks.

ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS mcp_page_size integer NOT NULL DEFAULT 50
    CHECK (mcp_page_size BETWEEN 1 AND 500);

CREATE INDEX IF NOT EXISTS tasks_pagination_idx
  ON tasks (project_id, priority DESC, created_at, id);

-- +goose Down
DROP INDEX IF EXISTS tasks_pagination_idx;
ALTER TABLE projects DROP COLUMN IF EXISTS mcp_page_size;
