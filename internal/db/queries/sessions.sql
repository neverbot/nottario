-- name: InsertSession :one
INSERT INTO sessions (user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, sqlc.narg('ip')::inet)
RETURNING id, user_id, created_at, last_seen_at, expires_at,
          COALESCE(user_agent, '')::text AS user_agent,
          COALESCE(host(ip), '')::text   AS ip;

-- name: GetActiveSession :one
SELECT id, user_id, created_at, last_seen_at, expires_at,
       COALESCE(user_agent, '')::text AS user_agent,
       COALESCE(host(ip), '')::text   AS ip
FROM sessions
WHERE id = $1 AND expires_at > now();

-- name: TouchSessionLastSeen :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: DeleteSessionByID :exec
DELETE FROM sessions WHERE id = $1;
