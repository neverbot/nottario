-- name: InsertProject :one
INSERT INTO projects (slug, name, description, primary_language, project_type, created_by_user_id)
VALUES ($1, $2, $3, NULLIF(sqlc.arg('primary_language')::text, ''),
        NULLIF(sqlc.arg('project_type')::text, ''), $4)
RETURNING id, slug, name, description,
          COALESCE(primary_language, '')::text AS primary_language,
          COALESCE(project_type, '')::text AS project_type,
          mcp_page_size, default_view,
          created_by_user_id, created_at, updated_at;

-- name: ListProjectsAdmin :many
SELECT id, slug, name, description,
       COALESCE(primary_language, '')::text AS primary_language,
       COALESCE(project_type, '')::text AS project_type,
       mcp_page_size, default_view,
       created_by_user_id, created_at, updated_at
FROM projects
ORDER BY name;

-- name: ListProjectsForUser :many
SELECT DISTINCT p.id, p.slug, p.name, p.description,
       COALESCE(p.primary_language, '')::text AS primary_language,
       COALESCE(p.project_type, '')::text AS project_type,
       p.mcp_page_size, p.default_view,
       p.created_by_user_id, p.created_at, p.updated_at
FROM projects p
JOIN memberships m ON m.project_id = p.id
WHERE m.user_id = $1
ORDER BY p.name;

-- name: GetProjectByIDOrSlug :one
SELECT id, slug, name, description,
       COALESCE(primary_language, '')::text AS primary_language,
       COALESCE(project_type, '')::text AS project_type,
       mcp_page_size, default_view,
       created_by_user_id, created_at, updated_at
FROM projects
WHERE id::text = sqlc.arg('id_or_slug')::text
   OR slug      = sqlc.arg('id_or_slug')::text;

-- name: UpdateProjectFields :exec
UPDATE projects
SET name = $2,
    description = $3,
    primary_language = NULLIF(sqlc.arg('primary_language')::text, ''),
    project_type = NULLIF(sqlc.arg('project_type')::text, ''),
    updated_at = now()
WHERE id = $1;

-- name: UpdateProjectMCPPageSize :exec
UPDATE projects SET mcp_page_size = $2, updated_at = now() WHERE id = $1;

-- name: UpdateProjectDefaultView :exec
UPDATE projects SET default_view = sqlc.arg('default_view')::text, updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: DeleteProjectByID :exec
DELETE FROM projects WHERE id = $1;

-- name: ProjectSlugExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE slug = $1)::bool;

-- name: ListProjectRepos :many
SELECT repo FROM project_repos WHERE project_id = $1 ORDER BY repo;

-- name: ClearProjectRepos :exec
DELETE FROM project_repos WHERE project_id = $1;

-- name: InsertProjectRepo :exec
INSERT INTO project_repos (project_id, repo)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;
