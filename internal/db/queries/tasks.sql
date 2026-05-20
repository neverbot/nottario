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

