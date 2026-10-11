-- name: ListTaskSteps :many
SELECT id, task_id, position, text, done, done_at, done_by_user_id, created_at
FROM task_steps
WHERE task_id = $1
ORDER BY position;

-- name: CountTaskSteps :one
SELECT count(*)::int AS total FROM task_steps WHERE task_id = $1;

-- name: InsertTaskStep :one
INSERT INTO task_steps (task_id, position, text, created_by_user_id, created_by_token_id)
VALUES (sqlc.arg('task_id')::uuid, sqlc.arg('position')::int, sqlc.arg('text')::text,
        sqlc.narg('created_by_user_id')::uuid, sqlc.narg('created_by_token_id')::uuid)
RETURNING id, task_id, position, text, done, done_at, done_by_user_id, created_at;

-- name: GetTaskStep :one
SELECT id, task_id, position, text, done, done_at, done_by_user_id, created_at
FROM task_steps
WHERE id = $1 AND task_id = $2;

-- name: UpdateTaskStep :one
-- Changes the text, the done flag, or both. Ticking stamps done_at and
-- done_by_user_id; unticking clears them; re-ticking a done step keeps
-- the original stamp.
UPDATE task_steps SET
  text = COALESCE(sqlc.narg('text')::text, text),
  done = COALESCE(sqlc.narg('done')::boolean, done),
  done_at = CASE
    WHEN sqlc.narg('done')::boolean IS NULL THEN done_at
    WHEN sqlc.narg('done')::boolean AND done THEN done_at
    WHEN sqlc.narg('done')::boolean THEN now()
    ELSE NULL END,
  done_by_user_id = CASE
    WHEN sqlc.narg('done')::boolean IS NULL THEN done_by_user_id
    WHEN sqlc.narg('done')::boolean AND done THEN done_by_user_id
    WHEN sqlc.narg('done')::boolean THEN sqlc.narg('actor_user_id')::uuid
    ELSE NULL END,
  updated_at = now()
WHERE id = sqlc.arg('id')::uuid AND task_id = sqlc.arg('task_id')::uuid
RETURNING id, task_id, position, text, done, done_at, done_by_user_id, created_at;

-- name: DeleteTaskStep :one
DELETE FROM task_steps WHERE id = $1 AND task_id = $2
RETURNING position;

-- name: ShiftTaskSteps :exec
-- Adds delta to the position of every step of the task whose position
-- lies in [lo, hi]. Used to close the gap a removed step leaves and to
-- make room for a moved one. Callers hold the task row lock.
UPDATE task_steps SET position = position + sqlc.arg('delta')::int, updated_at = now()
WHERE task_id = sqlc.arg('task_id')::uuid
  AND position BETWEEN sqlc.arg('lo')::int AND sqlc.arg('hi')::int;

-- name: SetTaskStepPosition :exec
UPDATE task_steps SET position = sqlc.arg('position')::int, updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: TaskStepProgress :many
-- Done / total step counts for a page of tasks. Tasks without steps
-- produce no row.
SELECT task_id,
       (count(*) FILTER (WHERE done))::int AS done,
       count(*)::int AS total
FROM task_steps
WHERE task_id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY task_id;
