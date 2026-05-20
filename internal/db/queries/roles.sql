-- name: ListProjectRoles :many
SELECT id, project_id, key, label, COALESCE(color, '')::text AS color, position, created_at
FROM roles
WHERE project_id = $1
ORDER BY position, label;

-- name: InsertProjectRole :one
INSERT INTO roles (project_id, key, label, color, position)
VALUES (
  $1, $2, $3, NULLIF(sqlc.arg('color')::text, ''),
  COALESCE((SELECT MAX(position) + 1 FROM roles WHERE project_id = $1), 0)
)
RETURNING id, project_id, key, label, COALESCE(color, '')::text AS color, position, created_at;

-- name: UpdateProjectRole :one
UPDATE roles
SET label = $2, color = NULLIF(sqlc.arg('color')::text, '')
WHERE id = $1
RETURNING id, project_id, key, label, COALESCE(color, '')::text AS color, position, created_at;

-- name: ListProjectRoleIDs :many
SELECT id FROM roles WHERE project_id = $1 ORDER BY created_at, id;

-- name: SetRolePosition :exec
UPDATE roles SET position = $2 WHERE id = $1;

-- name: DeleteProjectRole :exec
DELETE FROM roles WHERE id = $1;

-- name: InsertSeedRole :exec
INSERT INTO roles (project_id, key, label, color, position)
VALUES ($1, $2, $3, $4, $5);
