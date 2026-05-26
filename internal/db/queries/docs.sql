-- name: GetDocumentByPath :one
SELECT id, current_version
FROM documents
WHERE scope = sqlc.arg('scope')::text
  AND project_id IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND path = sqlc.arg('path')::text;

-- name: InsertDocument :one
INSERT INTO documents (
    scope, project_id, path, kind, title, description, content_md, frontmatter,
    current_version, created_by_user_id, created_by_token_id,
    updated_by_user_id, updated_by_token_id
)
VALUES (
    sqlc.arg('scope')::text,
    sqlc.narg('project_id')::uuid,
    sqlc.arg('path')::text,
    sqlc.arg('kind')::text,
    sqlc.arg('title')::text,
    sqlc.arg('description')::text,
    sqlc.arg('content_md')::text,
    sqlc.arg('frontmatter')::jsonb,
    1,
    sqlc.narg('created_by_user_id')::uuid,
    sqlc.narg('created_by_token_id')::uuid,
    sqlc.narg('created_by_user_id')::uuid,
    sqlc.narg('created_by_token_id')::uuid
)
RETURNING id, scope, project_id, path, kind, title, description, content_md,
          frontmatter, current_version, deleted_at,
          created_by_user_id, created_by_token_id,
          updated_by_user_id, updated_by_token_id,
          created_at, updated_at;

-- name: UpdateDocument :one
UPDATE documents
SET kind = sqlc.arg('kind')::text,
    title = sqlc.arg('title')::text,
    description = sqlc.arg('description')::text,
    content_md = sqlc.arg('content_md')::text,
    frontmatter = sqlc.arg('frontmatter')::jsonb,
    current_version = sqlc.arg('current_version')::int,
    updated_by_user_id = sqlc.narg('updated_by_user_id')::uuid,
    updated_by_token_id = sqlc.narg('updated_by_token_id')::uuid,
    deleted_at = NULL
WHERE id = sqlc.arg('id')::uuid
RETURNING id, scope, project_id, path, kind, title, description, content_md,
          frontmatter, current_version, deleted_at,
          created_by_user_id, created_by_token_id,
          updated_by_user_id, updated_by_token_id,
          created_at, updated_at;

-- name: ReadDocument :one
SELECT id, scope, project_id, path, kind, title, description, content_md,
       frontmatter, current_version, deleted_at,
       created_by_user_id, created_by_token_id,
       updated_by_user_id, updated_by_token_id,
       created_at, updated_at
FROM documents
WHERE scope = sqlc.arg('scope')::text
  AND project_id IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND path = sqlc.arg('path')::text
  AND deleted_at IS NULL;

-- name: ListDocuments :many
SELECT id, scope, project_id, path, kind, title, description, current_version,
       updated_by_user_id, updated_by_token_id, updated_at
FROM documents
WHERE scope = sqlc.arg('scope')::text
  AND project_id IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('path_prefix')::text IS NULL OR path LIKE sqlc.narg('path_prefix')::text)
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
ORDER BY path;

-- name: SearchDocuments :many
-- Multi-config tsquery: simple || english || spanish, matching the
-- documents.search_vector definition in migrations/00013.
WITH q AS (
    SELECT (
        plainto_tsquery('simple',  sqlc.arg('query')::text) ||
        plainto_tsquery('english', sqlc.arg('query')::text) ||
        plainto_tsquery('spanish', sqlc.arg('query')::text)
    ) AS tsq
)
SELECT id, scope, project_id, path, kind, title, description, current_version,
       updated_by_user_id, updated_by_token_id, updated_at,
       ts_rank(search_vector, (SELECT tsq FROM q))::real AS rank
FROM documents
WHERE scope = sqlc.arg('scope')::text
  AND project_id IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND deleted_at IS NULL
  AND search_vector @@ (SELECT tsq FROM q)
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
ORDER BY rank DESC, path;

-- name: ListDocumentVersions :many
SELECT version, title, message, author_user_id, author_token_id, created_at
FROM document_versions
WHERE document_id = $1
ORDER BY version DESC;

-- name: GetDocumentVersion :one
SELECT id, document_id, version, title, description, content_md, frontmatter,
       message, author_user_id, author_token_id, created_at
FROM document_versions
WHERE document_id = $1 AND version = $2;

-- name: GetDocumentForDelete :one
SELECT id, current_version, title, description, content_md, frontmatter
FROM documents
WHERE scope = sqlc.arg('scope')::text
  AND project_id IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND path = sqlc.arg('path')::text
  AND deleted_at IS NULL;

-- name: SoftDeleteDocument :exec
UPDATE documents
SET deleted_at = now(),
    current_version = sqlc.arg('current_version')::int,
    updated_by_user_id = sqlc.narg('updated_by_user_id')::uuid,
    updated_by_token_id = sqlc.narg('updated_by_token_id')::uuid
WHERE id = sqlc.arg('id')::uuid;

-- name: InsertDocumentVersion :exec
INSERT INTO document_versions (
    document_id, version, title, description, content_md, frontmatter,
    message, author_user_id, author_token_id
)
VALUES (
    sqlc.arg('document_id')::uuid,
    sqlc.arg('version')::int,
    sqlc.arg('title')::text,
    sqlc.arg('description')::text,
    sqlc.arg('content_md')::text,
    sqlc.arg('frontmatter')::jsonb,
    sqlc.arg('message')::text,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid
);
