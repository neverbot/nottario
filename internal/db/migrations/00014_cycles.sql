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

-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM projects WHERE owner_user_id IS NULL) THEN
        RAISE EXCEPTION 'cannot backfill projects.owner_user_id: % project(s) have no owner candidate (no created_by_user_id and no admin users to fall back to)',
            (SELECT count(*) FROM projects WHERE owner_user_id IS NULL);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE projects ALTER COLUMN owner_user_id SET NOT NULL;

-- One sprint-1 per project, position 1, open.
INSERT INTO cycles (project_id, name, position)
SELECT id, 'sprint-1', 1 FROM projects;

ALTER TABLE tasks
    ADD COLUMN cycle_id uuid REFERENCES cycles(id) ON DELETE RESTRICT;

UPDATE tasks t
SET cycle_id = c.id
FROM cycles c
WHERE c.project_id = t.project_id AND c.position = 1;

ALTER TABLE tasks ALTER COLUMN cycle_id SET NOT NULL;
CREATE INDEX tasks_cycle_idx ON tasks(cycle_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tasks_enforce_cycle_cascade() RETURNS trigger AS $$
BEGIN
    IF NEW.parent_task_id IS NOT NULL THEN
        SELECT cycle_id INTO NEW.cycle_id
        FROM tasks WHERE id = NEW.parent_task_id;
        IF NEW.cycle_id IS NULL THEN
            RAISE EXCEPTION 'parent task % has no cycle_id', NEW.parent_task_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER tasks_cycle_cascade
    BEFORE INSERT OR UPDATE OF parent_task_id, cycle_id ON tasks
    FOR EACH ROW EXECUTE FUNCTION tasks_enforce_cycle_cascade();

-- +goose Down
DROP TRIGGER IF EXISTS tasks_cycle_cascade ON tasks;
DROP FUNCTION IF EXISTS tasks_enforce_cycle_cascade();
DROP INDEX IF EXISTS tasks_cycle_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS cycle_id;
ALTER TABLE projects DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE projects DROP COLUMN IF EXISTS cycle_label;
DROP INDEX IF EXISTS cycles_one_active_per_project;
DROP TABLE IF EXISTS cycles;
