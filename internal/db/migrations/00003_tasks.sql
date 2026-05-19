-- +goose Up
CREATE TABLE tasks (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id            uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_task_id        uuid REFERENCES tasks(id) ON DELETE CASCADE,
    type                  text NOT NULL DEFAULT 'task',
    title                 text NOT NULL,
    description_md        text NOT NULL DEFAULT '',
    state                 text NOT NULL DEFAULT 'todo',
    priority              integer NOT NULL DEFAULT 50,
    assignee_user_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    target_role_id        uuid REFERENCES roles(id) ON DELETE SET NULL,
    actual_start          timestamptz,
    actual_end            timestamptz,
    created_by_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by_token_id   uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CHECK (type   IN ('task', 'bug', 'chore', 'spike', 'feature')),
    CHECK (state  IN ('todo', 'doing', 'done'))
);

CREATE INDEX tasks_project_idx       ON tasks(project_id);
CREATE INDEX tasks_state_idx         ON tasks(project_id, state);
CREATE INDEX tasks_assignee_idx      ON tasks(project_id, assignee_user_id);
CREATE INDEX tasks_target_role_idx   ON tasks(project_id, target_role_id);
CREATE INDEX tasks_parent_idx        ON tasks(parent_task_id);

CREATE TABLE task_dependencies (
    task_id        uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_id  uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, depends_on_id),
    CHECK (task_id <> depends_on_id)
);
CREATE INDEX task_dependencies_dep_idx ON task_dependencies(depends_on_id);

CREATE TABLE task_commits (
    task_id              uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    repo                 text NOT NULL,
    sha                  text NOT NULL,
    message              text NOT NULL DEFAULT '',
    added_at             timestamptz NOT NULL DEFAULT now(),
    added_by_user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    added_by_token_id    uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    PRIMARY KEY (task_id, repo, sha)
);

CREATE TABLE task_comments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    author_token_id    uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    body_md            text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX task_comments_task_idx ON task_comments(task_id);

-- Trigger to bump updated_at on task changes.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_task_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER tasks_touch_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION touch_task_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS tasks_touch_updated_at ON tasks;
DROP FUNCTION IF EXISTS touch_task_updated_at();
DROP TABLE IF EXISTS task_comments;
DROP TABLE IF EXISTS task_commits;
DROP TABLE IF EXISTS task_dependencies;
DROP TABLE IF EXISTS tasks;
