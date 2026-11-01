-- name: InsertNotification :one
INSERT INTO notifications (
  user_id, project_id, kind, task_id, actor_user_id, body
) VALUES (
  sqlc.arg('user_id')::uuid,
  sqlc.arg('project_id')::uuid,
  sqlc.arg('kind')::text,
  sqlc.narg('task_id')::uuid,
  sqlc.narg('actor_user_id')::uuid,
  sqlc.arg('body')::text
)
RETURNING id, user_id, project_id, kind, task_id, actor_user_id, body, created_at, read_at;

-- name: ListNotifications :many
-- Keyset paginated by (created_at DESC, id ASC). Pass after_created_at
-- and after_id from a previous page's tail row; NULL on the first page.
-- Requests limit+1 so the caller can detect has_more (last row dropped).
SELECT id, user_id, project_id, kind, task_id, actor_user_id, body, created_at, read_at
FROM notifications
WHERE user_id = sqlc.arg('user_id')::uuid
  AND (
    sqlc.narg('after_created_at')::timestamptz IS NULL
    OR created_at < sqlc.narg('after_created_at')::timestamptz
    OR (created_at = sqlc.narg('after_created_at')::timestamptz
        AND id > sqlc.narg('after_id')::uuid)
  )
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')::int;

-- name: CountUnread :one
SELECT COUNT(*)::int AS unread
FROM notifications
WHERE user_id = sqlc.arg('user_id')::uuid
  AND read_at IS NULL;

-- name: MarkRead :execrows
UPDATE notifications
SET read_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid
  AND id = ANY(sqlc.arg('ids')::uuid[])
  AND read_at IS NULL;

-- name: MarkQuestionNotificationsRead :execrows
-- Clears a user's question_asked bell entries for a task once none of
-- the task's questions addressed to them is still open.
UPDATE notifications
SET read_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid
  AND task_id = sqlc.arg('task_id')::uuid
  AND kind = 'question_asked'
  AND read_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM task_questions
    WHERE task_id = sqlc.arg('task_id')::uuid
      AND recipient_user_id = sqlc.arg('user_id')::uuid
      AND state = 'open'
  );

-- name: MarkAllRead :execrows
UPDATE notifications
SET read_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid
  AND read_at IS NULL;

-- name: GetPreferences :one
SELECT notification_preferences
FROM users
WHERE id = sqlc.arg('user_id')::uuid;

-- name: UpdatePreferences :exec
UPDATE users
SET notification_preferences = sqlc.arg('prefs')::jsonb
WHERE id = sqlc.arg('user_id')::uuid;
