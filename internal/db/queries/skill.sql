-- name: ListSkillOverridePaths :many
-- Lists every global-scope skill document path. The skill bundle
-- consults this list to union embedded files with user-provided
-- overrides; returned paths still include the 'global/skills/' prefix
-- and the caller trims it.
SELECT path FROM documents
WHERE scope = 'global'
  AND kind = 'skill'
  AND deleted_at IS NULL
  AND path LIKE sqlc.arg('path_prefix')::text;

-- name: GetSkillOverride :one
-- Reads the body and frontmatter of a single skill-override document
-- keyed by its full path (e.g. 'global/skills/domains/tasks.md').
-- Returns ErrNoRows when there is no override for that path.
SELECT content_md, frontmatter FROM documents
WHERE scope = 'global'
  AND kind = 'skill'
  AND deleted_at IS NULL
  AND path = sqlc.arg('path')::text;
