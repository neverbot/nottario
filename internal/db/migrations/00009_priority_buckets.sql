-- +goose Up
-- Per-project named priority buckets. Tasks still store an integer
-- in tasks.priority; this table provides a vocabulary so the team
-- (and agents) can refer to priorities by name. Buckets are
-- editable per project; defaults are seeded on project creation
-- and (here) backfilled into every existing project.
CREATE TABLE project_priorities (
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key         text NOT NULL,
    value       int  NOT NULL,
    position    int  NOT NULL DEFAULT 0,
    is_default  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, key)
);
CREATE INDEX project_priorities_value_idx ON project_priorities(project_id, value);

-- Backfill the four default buckets for every existing project. New
-- projects get them seeded by the application on creation.
INSERT INTO project_priorities (project_id, key, value, position, is_default)
SELECT p.id, x.key, x.value, x.pos, true
FROM projects p
CROSS JOIN (VALUES
    ('low',      30, 0),
    ('medium',   60, 1),
    ('high',     90, 2),
    ('critical', 100, 3)
) AS x(key, value, pos)
ON CONFLICT DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS project_priorities_value_idx;
DROP TABLE IF EXISTS project_priorities;
