-- name: InsertAPIToken :one
INSERT INTO api_tokens (user_id, project_id, name, token_hash, prefix, default_role_id)
VALUES (
    sqlc.arg('user_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('token_hash')::bytea,
    sqlc.arg('prefix')::text,
    sqlc.narg('default_role_id')::uuid
)
RETURNING id, user_id, project_id, name, prefix, default_role_id,
          created_at, last_used_at, revoked_at;

-- name: LookupAPIToken :one
SELECT t.id AS token_id, t.user_id, t.project_id, t.name AS token_name,
       t.prefix, t.default_role_id,
       t.created_at AS token_created_at,
       t.last_used_at, t.revoked_at,
       u.id AS user_id_full, u.github_login, u.github_id, u.display_name,
       COALESCE(u.avatar_url, '')::text AS avatar_url,
       u.is_admin, u.created_at AS user_created_at, u.last_seen_at
FROM api_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL;

-- name: TouchTokenLastUsed :exec
UPDATE api_tokens SET last_used_at = now() WHERE id = $1;

-- name: ListProjectTokens :many
SELECT id, user_id, project_id, name, prefix, default_role_id,
       created_at, last_used_at, revoked_at
FROM api_tokens
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: ListUserTokens :many
-- Every token the given user has issued, across every project they
-- belong to. Joined to projects so the /me page can render a
-- project-name column without a second round-trip. Revoked tokens
-- are included so the audit view is honest.
SELECT t.id, t.user_id, t.project_id, t.name, t.prefix,
       t.default_role_id, t.created_at, t.last_used_at, t.revoked_at,
       p.name AS project_name, p.slug AS project_slug
FROM api_tokens t
JOIN projects p ON p.id = t.project_id
WHERE t.user_id = sqlc.arg('user_id')::uuid
ORDER BY p.name ASC, t.created_at DESC;

-- name: GetAPIToken :one
SELECT id, user_id, project_id, name, prefix, default_role_id,
       created_at, last_used_at, revoked_at
FROM api_tokens
WHERE id = $1;

-- Look up the human-readable names of one or more api_tokens by id.
-- Used to enrich rows recorded with a token_id (comments, task
-- creation, doc versions, cycle closes) so the UI can render an
-- "agent of <user> via <token name>" badge without leaking the
-- token's UUID.
-- name: ListTokenNamesByIDs :many
SELECT id, name FROM api_tokens
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: RevokeAPIToken :execrows
UPDATE api_tokens
SET revoked_at = now()
WHERE id = $1
  AND revoked_at IS NULL
  AND (sqlc.arg('is_admin')::bool OR user_id = sqlc.arg('requester_user_id')::uuid);
