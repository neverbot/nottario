-- +goose Up
-- Expand the FTS index to cover `simple` (verbatim, preserves
-- non-English words and slugs), `english` (Porter stemmer) and
-- `spanish`. Searching for "task" should match "tasks" and vice
-- versa, while a Spanish word like "tareas" still matches "tarea".
-- Each generated column unions the three tsvectors; the query side
-- (see internal/db/queries/search.sql) does the same with tsquery.

-- Tasks
ALTER TABLE tasks DROP COLUMN IF EXISTS search_vector;
ALTER TABLE tasks
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple',  coalesce(title, '')),         'A') ||
        setweight(to_tsvector('english', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('spanish', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('simple',  coalesce(description_md, '')),'B') ||
        setweight(to_tsvector('english', coalesce(description_md, '')),'B') ||
        setweight(to_tsvector('spanish', coalesce(description_md, '')),'B')
    ) STORED;
CREATE INDEX tasks_search_idx ON tasks USING GIN (search_vector);

-- Documents
ALTER TABLE documents DROP COLUMN IF EXISTS search_vector;
ALTER TABLE documents
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple',  coalesce(title, '')),       'A') ||
        setweight(to_tsvector('english', coalesce(title, '')),       'A') ||
        setweight(to_tsvector('spanish', coalesce(title, '')),       'A') ||
        setweight(to_tsvector('simple',  coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('spanish', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple',  coalesce(content_md, '')),  'C') ||
        setweight(to_tsvector('english', coalesce(content_md, '')),  'C') ||
        setweight(to_tsvector('spanish', coalesce(content_md, '')),  'C')
    ) STORED;
CREATE INDEX documents_search_idx ON documents USING GIN (search_vector);

-- Architecture nodes
ALTER TABLE arch_nodes DROP COLUMN IF EXISTS search_vector;
ALTER TABLE arch_nodes
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple',  coalesce(slug, '')),           'A') ||
        setweight(to_tsvector('simple',  coalesce(name, '')),           'A') ||
        setweight(to_tsvector('english', coalesce(name, '')),           'A') ||
        setweight(to_tsvector('spanish', coalesce(name, '')),           'A') ||
        setweight(to_tsvector('simple',  coalesce(description_md, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(description_md, '')), 'B') ||
        setweight(to_tsvector('spanish', coalesce(description_md, '')), 'B')
    ) STORED;
CREATE INDEX arch_nodes_search_idx ON arch_nodes USING GIN (search_vector);


-- +goose Down
-- Revert each table to the original `simple`-only vector.

DROP INDEX IF EXISTS tasks_search_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS search_vector;
ALTER TABLE tasks
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('simple', coalesce(description_md,'')), 'B')
    ) STORED;
CREATE INDEX tasks_search_idx ON tasks USING GIN (search_vector);

DROP INDEX IF EXISTS documents_search_idx;
ALTER TABLE documents DROP COLUMN IF EXISTS search_vector;
ALTER TABLE documents
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(content_md, '')), 'C')
    ) STORED;
CREATE INDEX documents_search_idx ON documents USING GIN (search_vector);

DROP INDEX IF EXISTS arch_nodes_search_idx;
ALTER TABLE arch_nodes DROP COLUMN IF EXISTS search_vector;
ALTER TABLE arch_nodes
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(slug, '')),          'A') ||
        setweight(to_tsvector('simple', coalesce(name, '')),          'A') ||
        setweight(to_tsvector('simple', coalesce(description_md,'')), 'B')
    ) STORED;
CREATE INDEX arch_nodes_search_idx ON arch_nodes USING GIN (search_vector);
