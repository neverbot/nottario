-- name: GetUserByGithubID :one
SELECT id, github_login, github_id, display_name,
       COALESCE(avatar_url, '')::text AS avatar_url,
       is_admin, created_at, last_seen_at
FROM users
WHERE github_id = $1;

-- name: CountUsers :one
SELECT COUNT(*)::int FROM users;

-- name: InsertUser :one
INSERT INTO users (github_login, github_id, display_name, avatar_url, is_admin)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, github_login, github_id, display_name,
          COALESCE(avatar_url, '')::text AS avatar_url,
          is_admin, created_at, last_seen_at;

-- name: UpdateUserProfile :exec
UPDATE users
SET display_name = $2, avatar_url = $3, github_login = $4
WHERE id = $1;

-- name: GetUserByID :one
SELECT id, github_login, github_id, display_name,
       COALESCE(avatar_url, '')::text AS avatar_url,
       is_admin, created_at, last_seen_at
FROM users WHERE id = $1;

-- name: TouchUserLastSeen :exec
UPDATE users SET last_seen_at = now() WHERE id = $1;
