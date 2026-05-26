-- +goose Up
CREATE TABLE cycles (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name               text NOT NULL,
    position           int  NOT NULL,
    opened_at          timestamptz NOT NULL DEFAULT now(),
    closed_at          timestamptz,
    closed_by_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    closed_by_token_id uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    UNIQUE (project_id, position),
    UNIQUE (project_id, name)
);

CREATE UNIQUE INDEX cycles_one_active_per_project
    ON cycles(project_id) WHERE closed_at IS NULL;

ALTER TABLE projects
    ADD COLUMN cycle_label text NOT NULL DEFAULT 'sprint',
    ADD COLUMN owner_user_id uuid REFERENCES users(id);

-- Backfill: owner = creator. created_by_user_id is nullable but
-- every real project has it set; defensive default to first admin
-- if absent (instance always has at least one admin after first
-- login).
UPDATE projects
SET owner_user_id = COALESCE(
        created_by_user_id,
        (SELECT id FROM users WHERE is_admin ORDER BY created_at LIMIT 1)
    );

ALTER TABLE projects ALTER COLUMN owner_user_id SET NOT NULL;

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE projects DROP COLUMN IF EXISTS cycle_label;
DROP INDEX IF EXISTS cycles_one_active_per_project;
DROP TABLE IF EXISTS cycles;
