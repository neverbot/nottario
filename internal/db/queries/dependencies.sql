-- name: InsertDependency :execrows
INSERT INTO task_dependencies (task_id, depends_on_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveDependency :execrows
DELETE FROM task_dependencies WHERE task_id = $1 AND depends_on_id = $2;

-- name: ListProjectDependencies :many
SELECT d.task_id, d.depends_on_id
FROM task_dependencies d
JOIN tasks t ON t.id = d.task_id
WHERE t.project_id = $1;

-- name: ListDependsOn :many
SELECT depends_on_id FROM task_dependencies WHERE task_id = $1;

-- name: ListDependents :many
SELECT task_id FROM task_dependencies WHERE depends_on_id = $1;

-- name: WouldCreateCycle :one
WITH RECURSIVE reachable AS (
  SELECT depends_on_id AS rid FROM task_dependencies WHERE task_id = sqlc.arg('start')::uuid
  UNION
  SELECT d.depends_on_id AS rid
  FROM task_dependencies d
  JOIN reachable r ON r.rid = d.task_id
)
SELECT (EXISTS (SELECT 1 FROM reachable WHERE rid = sqlc.arg('target')::uuid)
        OR sqlc.arg('start')::uuid = sqlc.arg('target')::uuid) AS hit;

-- name: LockTaskRow :exec
SELECT id FROM tasks WHERE id = $1 FOR UPDATE;

-- name: LockTwoTaskRows :many
SELECT id FROM tasks
WHERE id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY id
FOR UPDATE;

-- name: ProjectIDForTask :one
SELECT project_id FROM tasks WHERE id = $1;

-- name: AcquireDepLock :exec
SELECT pg_advisory_xact_lock(sqlc.arg('namespace')::int, hashtext(sqlc.arg('project_id')::text));
