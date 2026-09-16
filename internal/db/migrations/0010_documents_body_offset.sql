-- +goose Up
-- +goose StatementBegin
-- Documents are about to be stored whole: content_md keeps the
-- frontmatter block instead of having it split off (see the Go
-- migration 0011). The search index must still ignore frontmatter, so
-- it indexes content_md from body_offset on.
--
-- body_offset is the number of characters before the body starts,
-- computed by the application's frontmatter parser at write time
-- (docs.BodyOffset). Postgres never parses markdown; it only takes a
-- substring. Characters, not bytes, because substr() counts
-- characters.
--
-- Every existing row gets 0, which is correct today: until 0011 runs,
-- content_md holds the body only.
ALTER TABLE documents
    ADD COLUMN body_offset integer NOT NULL DEFAULT 0
    CONSTRAINT documents_body_offset_check CHECK (body_offset >= 0);

-- A generated column's expression cannot be altered in place; drop
-- and recreate it (the GIN index goes with the column).
ALTER TABLE documents DROP COLUMN search_vector;
ALTER TABLE documents ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple'::regconfig,  COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('simple'::regconfig,  COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('simple'::regconfig,  COALESCE(substr(content_md, body_offset + 1), ''::text)), 'C'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(substr(content_md, body_offset + 1), ''::text)), 'C'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(substr(content_md, body_offset + 1), ''::text)), 'C'::"char")
) STORED;
CREATE INDEX documents_search_idx ON documents USING gin (search_vector);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE documents DROP COLUMN search_vector;
ALTER TABLE documents ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple'::regconfig,  COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(title, ''::text)), 'A'::"char") ||
    setweight(to_tsvector('simple'::regconfig,  COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(description, ''::text)), 'B'::"char") ||
    setweight(to_tsvector('simple'::regconfig,  COALESCE(content_md, ''::text)), 'C'::"char") ||
    setweight(to_tsvector('english'::regconfig, COALESCE(content_md, ''::text)), 'C'::"char") ||
    setweight(to_tsvector('spanish'::regconfig, COALESCE(content_md, ''::text)), 'C'::"char")
) STORED;
CREATE INDEX documents_search_idx ON documents USING gin (search_vector);
ALTER TABLE documents DROP COLUMN body_offset;
-- +goose StatementEnd
