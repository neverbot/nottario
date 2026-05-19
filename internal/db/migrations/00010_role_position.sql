-- +goose Up
-- Add an explicit `position` to roles so admins can reorder them and
-- so the Gantt can render lanes in a predictable top-to-bottom order.

ALTER TABLE roles
  ADD COLUMN IF NOT EXISTS position integer NOT NULL DEFAULT 0;

-- Backfill: each project's existing roles get a position derived from
-- their creation order so the current visible ordering is preserved.
WITH ranked AS (
  SELECT id,
         row_number() OVER (PARTITION BY project_id ORDER BY created_at, id) - 1 AS pos
    FROM roles
)
UPDATE roles r
   SET position = ranked.pos
  FROM ranked
 WHERE r.id = ranked.id
   AND r.position = 0;

-- +goose Down
ALTER TABLE roles DROP COLUMN IF EXISTS position;
