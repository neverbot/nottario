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

-- +goose Down
DROP INDEX IF EXISTS cycles_one_active_per_project;
DROP TABLE IF EXISTS cycles;
