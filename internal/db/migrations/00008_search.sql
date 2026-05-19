-- +goose Up
-- Add a generated tsvector to arch_nodes so cross-cutting search can
-- include architecture nodes alongside tasks and documents (both of
-- which already have a `search_vector` column).
ALTER TABLE arch_nodes
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(slug, '')),        'A') ||
        setweight(to_tsvector('simple', coalesce(name, '')),        'A') ||
        setweight(to_tsvector('simple', coalesce(description_md,'')), 'B')
    ) STORED;

CREATE INDEX arch_nodes_search_idx ON arch_nodes USING GIN (search_vector);

-- Tasks already exist without a search_vector; add one now so the
-- search package does not have to special-case absence.
ALTER TABLE tasks
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('simple', coalesce(description_md,'')), 'B')
    ) STORED;

CREATE INDEX tasks_search_idx ON tasks USING GIN (search_vector);

-- +goose Down
DROP INDEX IF EXISTS tasks_search_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS arch_nodes_search_idx;
ALTER TABLE arch_nodes DROP COLUMN IF EXISTS search_vector;
