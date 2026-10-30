-- name: LiveWorkingTasks :many
-- Tasks in doing across the given projects, most recently active
-- first. A task without a task_activity row falls back to updated_at.
SELECT t.id, t.project_id, t.title, t.type, t.target_role_id, t.assignee_user_id,
       t.actual_start, COALESCE(a.last_activity_at, t.updated_at)::timestamptz AS last_activity_at,
       t.active_session_id
FROM tasks t
LEFT JOIN task_activity a ON a.task_id = t.id
WHERE t.project_id = ANY(sqlc.arg('project_ids')::uuid[]) AND t.state = 'doing'
ORDER BY 8 DESC, t.id
LIMIT 201;

-- name: LiveFinishedTasks :many
-- Caps are one over what Build keeps, so it can tell a block was cut.
SELECT t.id, t.project_id, t.title, t.type, t.state, t.assignee_user_id, t.actual_end
FROM tasks t
WHERE t.project_id = ANY(sqlc.arg('project_ids')::uuid[])
  AND t.state IN ('done', 'wont_do') AND t.actual_end >= sqlc.arg('since')::timestamptz
ORDER BY t.actual_end DESC
LIMIT 101;

-- name: LiveInProgressSteps :many
SELECT id, task_id, position, text, started_at, started_session_id
FROM task_steps
WHERE task_id = ANY(sqlc.arg('ids')::uuid[]) AND started_at IS NOT NULL AND NOT done
ORDER BY started_at DESC;

-- name: ProjectsLiveSettings :many
SELECT id, name, slug, live_idle_minutes FROM projects
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetRolesBrief :many
SELECT id, key, label FROM roles WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: SetTaskLastActivityForTest :exec
-- Test only: backdates a task's activity to exercise the idle marker.
UPDATE task_activity SET last_activity_at = sqlc.arg('at')::timestamptz
WHERE task_id = sqlc.arg('task_id')::uuid;
