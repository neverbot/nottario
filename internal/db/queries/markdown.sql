-- Queries used by the markdown renderer to resolve cross-domain chip
-- references ([[task:N]], [[doc:path]], [[arch:slug]]) into the
-- target's current title for inline display.

-- name: GetTaskChipByShortID :one
-- Resolves a task chip by the 8-char (or longer) UUID prefix the user
-- typed. We match prefix with LIKE on the text form of the UUID. The
-- query is scoped to a single project to keep the result deterministic
-- when the same prefix matches across projects.
SELECT id, title, state
FROM tasks
WHERE project_id = sqlc.arg('project_id')::uuid
  AND id::text LIKE sqlc.arg('prefix')::text
LIMIT 1;

-- name: GetDocChipByPath :one
-- Resolves a doc chip by its logical path within a project.
SELECT id, title
FROM documents
WHERE scope = 'project'
  AND project_id = sqlc.arg('project_id')::uuid
  AND path = sqlc.arg('path')::text
  AND deleted_at IS NULL;

-- name: GetArchChipBySlug :one
-- Resolves an architecture node chip by slug.
SELECT id, name
FROM arch_nodes
WHERE project_id = sqlc.arg('project_id')::uuid
  AND slug = sqlc.arg('slug')::text;
