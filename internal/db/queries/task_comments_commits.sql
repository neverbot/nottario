-- name: InsertTaskComment :one
INSERT INTO task_comments (task_id, author_user_id, author_token_id, body_md)
VALUES ($1, $2, $3, $4)
RETURNING id, task_id, author_user_id, author_token_id, body_md, created_at;

-- name: ListTaskComments :many
SELECT id, task_id, author_user_id, author_token_id, body_md, created_at
FROM task_comments
WHERE task_id = $1
ORDER BY created_at ASC;

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
