-- name: GetTask :one
SELECT id, project_id, parent_task_id, type, title, description_md,
       state, priority, assignee_user_id, target_role_id,
       actual_start, actual_end,
       created_by_user_id, created_by_token_id,
       created_at, updated_at
FROM tasks
WHERE id = $1;

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = $1;

-- name: LockTaskTypeAndParent :one
SELECT type, parent_task_id FROM tasks WHERE id = $1 FOR UPDATE;

-- name: ListUnresolvedPreconditions :many
SELECT t.id, t.title, t.state
FROM task_dependencies td
JOIN tasks t ON t.id = td.depends_on_id
WHERE td.task_id = $1 AND t.state <> 'done'
ORDER BY t.actual_end NULLS LAST, t.created_at;

-- name: SetTaskTodo :exec
UPDATE tasks SET
  state = 'todo',
  actual_start = NULL,
  actual_end = NULL,
  updated_at = now()
WHERE id = $1;

-- name: SetTaskDoing :exec
UPDATE tasks SET
  state = 'doing',
  actual_start = COALESCE(actual_start, now()),
  actual_end = NULL,
  updated_at = now()
WHERE id = $1;

-- name: SetTaskDone :exec
UPDATE tasks SET
  state = 'done',
  actual_start = COALESCE(actual_start, now()),
  actual_end = now(),
  updated_at = now()
WHERE id = $1;

-- name: GetParentStateAndGrandparent :one
SELECT state, parent_task_id FROM tasks WHERE id = $1 FOR UPDATE;

-- name: CountNonDoneChildren :one
SELECT COUNT(*)::int FROM tasks
WHERE parent_task_id = $1 AND state <> 'done';

-- name: RoleExistsInProject :one
SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1 AND project_id = $2)::bool;

-- name: UserBelongsToProjectOrIsAdmin :one
SELECT (
  EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_admin = true)
  OR EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND project_id = $2)
)::bool;

-- name: InsertTask :one
INSERT INTO tasks (
  project_id, parent_task_id, type, title, description_md,
  state, priority, assignee_user_id, target_role_id,
  created_by_user_id, created_by_token_id
)
VALUES ($1, $2, $3, $4, $5, 'todo', $6, $7, $8, $9, $10)
RETURNING id, project_id, parent_task_id, type, title, description_md,
          state, priority, assignee_user_id, target_role_id,
          actual_start, actual_end,
          created_by_user_id, created_by_token_id,
          created_at, updated_at;

