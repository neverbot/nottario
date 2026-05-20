-- name: ListProjectPriorities :many
SELECT project_id, key, value, position, is_default
FROM project_priorities
WHERE project_id = $1
ORDER BY position, value DESC;

-- name: GetPriorityValue :one
SELECT value FROM project_priorities WHERE project_id = $1 AND key = $2;

-- name: GetPriorityClosestTo50 :one
SELECT value FROM project_priorities
WHERE project_id = $1
ORDER BY abs(value - 50) ASC, value DESC
LIMIT 1;

-- name: UpsertProjectPriority :one
INSERT INTO project_priorities (project_id, key, value, position, is_default)
VALUES ($1, $2, $3, $4, false)
ON CONFLICT (project_id, key) DO UPDATE
  SET value = EXCLUDED.value, position = EXCLUDED.position
RETURNING project_id, key, value, position, is_default;

-- name: DeleteProjectPriority :execrows
DELETE FROM project_priorities WHERE project_id = $1 AND key = $2;

-- name: SeedDefaultPriority :exec
INSERT INTO project_priorities (project_id, key, value, position, is_default)
VALUES ($1, $2, $3, $4, true)
ON CONFLICT DO NOTHING;
