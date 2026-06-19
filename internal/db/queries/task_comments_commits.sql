-- name: InsertTaskComment :one
INSERT INTO task_comments (task_id, author_user_id, author_token_id, body_md)
VALUES ($1, $2, $3, $4)
RETURNING id, task_id, author_user_id, author_token_id, body_md,
          created_at, updated_at, edited_at, edited_by_user_id;

-- name: ListTaskComments :many
SELECT id, task_id, author_user_id, author_token_id, body_md,
       created_at, updated_at, edited_at, edited_by_user_id
FROM task_comments
WHERE task_id = $1
ORDER BY created_at ASC;

-- name: GetTaskComment :one
SELECT id, task_id, author_user_id, author_token_id, body_md,
       created_at, updated_at, edited_at, edited_by_user_id
FROM task_comments
WHERE id = $1;

-- name: UpdateTaskComment :one
-- Edits a comment body with optimistic concurrency. Returns the updated
-- row when the expected `updated_at` matches the row's current value;
-- returns no rows otherwise so the caller can answer 409.
--
-- Sets edited_at / edited_by_user_id whenever the body actually
-- changes (a no-op edit doesn't paint the row as edited).
UPDATE task_comments SET
  body_md = sqlc.arg('body_md')::text,
  edited_at = CASE WHEN sqlc.arg('body_md')::text <> body_md THEN now() ELSE edited_at END,
  edited_by_user_id = CASE WHEN sqlc.arg('body_md')::text <> body_md
                           THEN sqlc.arg('caller_user_id')::uuid
                           ELSE edited_by_user_id END,
  updated_at = now()
WHERE id = $1
  AND (sqlc.narg('expected_updated_at')::timestamptz IS NULL
       OR updated_at = sqlc.narg('expected_updated_at')::timestamptz)
RETURNING id, task_id, author_user_id, author_token_id, body_md,
          created_at, updated_at, edited_at, edited_by_user_id;

-- name: DeleteTaskComment :execrows
DELETE FROM task_comments WHERE id = $1;

-- name: UpsertTaskCommit :exec
INSERT INTO task_commits (task_id, repo, sha, message, added_by_user_id, added_by_token_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (task_id, repo, sha) DO UPDATE SET message = EXCLUDED.message;

-- name: DeleteTaskCommit :exec
DELETE FROM task_commits WHERE task_id = $1 AND repo = $2 AND sha = $3;

-- name: ListTaskCommits :many
SELECT task_id, repo, sha, message, added_at
FROM task_commits
WHERE task_id = $1
ORDER BY added_at DESC;
