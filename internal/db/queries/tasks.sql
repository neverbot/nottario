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

-- name: ListTasks :many
-- Optional filters: state, type, assignee, target_role, parent_task.
-- include_children=false (default) restricts to parent IS NULL UNLESS
-- parent_task_id is explicitly set.
SELECT id, project_id, parent_task_id, type, title, description_md,
       state, priority, assignee_user_id, target_role_id,
       actual_start, actual_end,
       created_by_user_id, created_by_token_id,
       created_at, updated_at
FROM tasks
WHERE project_id = $1
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type')::text)
  AND (sqlc.narg('assignee_user_id')::uuid IS NULL OR assignee_user_id = sqlc.narg('assignee_user_id')::uuid)
  AND (sqlc.narg('target_role_id')::uuid IS NULL OR target_role_id = sqlc.narg('target_role_id')::uuid)
  AND (
    CASE
      WHEN sqlc.narg('parent_task_id')::uuid IS NOT NULL THEN parent_task_id = sqlc.narg('parent_task_id')::uuid
      WHEN sqlc.arg('include_children')::bool THEN TRUE
      ELSE parent_task_id IS NULL
    END
  )
ORDER BY priority DESC, created_at ASC;

-- name: ListTasksPaginated :many
-- Same filters as ListTasks plus keyset cursor on (priority DESC,
-- created_at ASC, id ASC). The cursor params are all-or-nothing:
-- pass them together or all NULL for the first page. The caller
-- requests limit+1 rows to detect has_more.
SELECT id, project_id, parent_task_id, type, title, description_md,
       state, priority, assignee_user_id, target_role_id,
       actual_start, actual_end,
       created_by_user_id, created_by_token_id,
       created_at, updated_at
FROM tasks
WHERE project_id = $1
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type')::text)
  AND (sqlc.narg('assignee_user_id')::uuid IS NULL OR assignee_user_id = sqlc.narg('assignee_user_id')::uuid)
  AND (sqlc.narg('target_role_id')::uuid IS NULL OR target_role_id = sqlc.narg('target_role_id')::uuid)
  AND (
    CASE
      WHEN sqlc.narg('parent_task_id')::uuid IS NOT NULL THEN parent_task_id = sqlc.narg('parent_task_id')::uuid
      WHEN sqlc.arg('include_children')::bool THEN TRUE
      ELSE parent_task_id IS NULL
    END
  )
  AND (
    sqlc.narg('cursor_priority')::int IS NULL
    OR priority < sqlc.narg('cursor_priority')::int
    OR (priority = sqlc.narg('cursor_priority')::int AND created_at > sqlc.narg('cursor_created_at')::timestamptz)
    OR (priority = sqlc.narg('cursor_priority')::int
        AND created_at = sqlc.narg('cursor_created_at')::timestamptz
        AND id > sqlc.narg('cursor_id')::uuid)
  )
ORDER BY priority DESC, created_at ASC, id ASC
LIMIT sqlc.arg('page_limit')::int;

-- name: UpdateTaskFields :one
-- Optional fields: title, description, type, priority, assignee_user_id,
-- target_role_id. assignee/target_role have an explicit "unset"
-- boolean because COALESCE alone can't distinguish "leave alone" from
-- "set to NULL".
UPDATE tasks SET
  title = COALESCE(sqlc.narg('title')::text, title),
  description_md = COALESCE(sqlc.narg('description_md')::text, description_md),
  type = COALESCE(sqlc.narg('type')::text, type),
  priority = COALESCE(sqlc.narg('priority')::int, priority),
  assignee_user_id = CASE
    WHEN sqlc.arg('unset_assignee')::bool THEN NULL
    WHEN sqlc.narg('assignee_user_id')::uuid IS NOT NULL THEN sqlc.narg('assignee_user_id')::uuid
    ELSE assignee_user_id
  END,
  target_role_id = CASE
    WHEN sqlc.arg('unset_target_role')::bool THEN NULL
    WHEN sqlc.narg('target_role_id')::uuid IS NOT NULL THEN sqlc.narg('target_role_id')::uuid
    ELSE target_role_id
  END,
  updated_at = now()
WHERE id = $1
RETURNING id, project_id, parent_task_id, type, title, description_md,
          state, priority, assignee_user_id, target_role_id,
          actual_start, actual_end,
          created_by_user_id, created_by_token_id,
          created_at, updated_at;

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

