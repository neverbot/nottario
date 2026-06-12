-- name: ListCycles :many
SELECT id, project_id, name, position, opened_at, closed_at,
       closed_by_user_id, closed_by_token_id
FROM cycles
WHERE project_id = $1
ORDER BY position DESC;

-- name: GetCycle :one
SELECT id, project_id, name, position, opened_at, closed_at,
       closed_by_user_id, closed_by_token_id
FROM cycles WHERE id = $1;

-- name: GetActiveCycle :one
SELECT id, project_id, name, position, opened_at, closed_at,
       closed_by_user_id, closed_by_token_id
FROM cycles WHERE project_id = $1 AND closed_at IS NULL;

-- name: NextCyclePosition :one
SELECT COALESCE(MAX(position), 0) + 1 FROM cycles WHERE project_id = $1;

-- name: GetProjectCycleLabel :one
SELECT cycle_label FROM projects WHERE id = $1;

-- name: InsertCycle :one
INSERT INTO cycles (project_id, name, position)
VALUES (sqlc.arg('project_id')::uuid,
        sqlc.arg('name')::text,
        sqlc.arg('position')::int)
RETURNING id, project_id, name, position, opened_at, closed_at,
          closed_by_user_id, closed_by_token_id;

-- name: CloseCycle :exec
UPDATE cycles
SET closed_at = now(),
    closed_by_user_id = sqlc.narg('closed_by_user_id')::uuid,
    closed_by_token_id = sqlc.narg('closed_by_token_id')::uuid
WHERE id = sqlc.arg('id')::uuid;

-- name: AcquireCycleLock :exec
SELECT pg_advisory_xact_lock(
    hashtext('cycles'), hashtext(sqlc.arg('project_id')::text)
);

-- name: LockActiveCycle :one
SELECT id, project_id, name, position, opened_at, closed_at,
       closed_by_user_id, closed_by_token_id
FROM cycles WHERE project_id = $1 AND closed_at IS NULL
FOR UPDATE;

-- name: MovePartialFeatureSubtrees :execrows
-- For every feature in `from_cycle` that isn't done, move it AND
-- all its recursive descendants to `to_cycle`. Done children of
-- those features get re-stamped (they had cycle_id = from_cycle).
WITH RECURSIVE partial_features AS (
    SELECT id FROM tasks
    WHERE cycle_id = sqlc.arg('from_cycle')::uuid
      AND type = 'feature'
      AND state NOT IN ('done', 'wont_do')
), subtree AS (
    SELECT id FROM partial_features
    UNION
    SELECT t.id FROM tasks t
    INNER JOIN subtree s ON t.parent_task_id = s.id
)
UPDATE tasks SET cycle_id = sqlc.arg('to_cycle')::uuid
WHERE id IN (SELECT id FROM subtree);

-- name: MoveStandaloneNonDone :execrows
-- Move any task in `from_cycle` that is not yet done AND was not
-- already moved as part of a feature subtree (i.e. its cycle_id is
-- still the closing cycle).
UPDATE tasks SET cycle_id = sqlc.arg('to_cycle')::uuid
WHERE cycle_id = sqlc.arg('from_cycle')::uuid
  AND state NOT IN ('done', 'wont_do');
