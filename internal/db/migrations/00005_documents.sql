-- +goose Up
CREATE TABLE documents (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope                 text NOT NULL,
    project_id            uuid REFERENCES projects(id) ON DELETE CASCADE,
    path                  text NOT NULL,
    kind                  text NOT NULL DEFAULT 'context',
    title                 text NOT NULL DEFAULT '',
    description           text NOT NULL DEFAULT '',
    content_md            text NOT NULL DEFAULT '',
    frontmatter           jsonb NOT NULL DEFAULT '{}'::jsonb,
    current_version       int NOT NULL DEFAULT 1,
    deleted_at            timestamptz,
    created_by_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by_token_id   uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    updated_by_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_by_token_id   uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    search_vector         tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(content_md, '')), 'C')
    ) STORED,
    CHECK (scope IN ('project', 'global')),
    CHECK (kind  IN ('skill', 'context', 'note')),
    CHECK ((scope = 'global') = (project_id IS NULL)),
    UNIQUE (scope, project_id, path)
);

CREATE INDEX documents_search_idx       ON documents USING GIN (search_vector);
CREATE INDEX documents_project_kind_idx ON documents(project_id, kind);
CREATE INDEX documents_scope_path_idx   ON documents(scope, project_id, path);

-- Bump updated_at on document changes (mirroring tasks pattern).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_document_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER documents_touch_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION touch_document_updated_at();

CREATE TABLE document_versions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id       uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version           int NOT NULL,
    title             text NOT NULL DEFAULT '',
    description       text NOT NULL DEFAULT '',
    content_md        text NOT NULL DEFAULT '',
    frontmatter       jsonb NOT NULL DEFAULT '{}'::jsonb,
    message           text NOT NULL DEFAULT '',
    author_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    author_token_id   uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_id, version)
);
CREATE INDEX document_versions_doc_idx ON document_versions(document_id);

-- +goose Down
DROP TABLE IF EXISTS document_versions;
DROP TRIGGER IF EXISTS documents_touch_updated_at ON documents;
DROP FUNCTION IF EXISTS touch_document_updated_at();
DROP TABLE IF EXISTS documents;
