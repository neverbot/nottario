-- name: UpsertAgentSession :one
-- Creates the session on first use and refreshes it after: the label
-- and client are replaced only when a new one is given, last_seen_at
-- always moves.
INSERT INTO agent_sessions (project_id, token_id, user_id, session_key, label, client)
VALUES (sqlc.arg('project_id')::uuid, sqlc.narg('token_id')::uuid, sqlc.arg('user_id')::uuid,
        sqlc.arg('session_key')::text, sqlc.arg('label')::text, sqlc.arg('client')::text)
ON CONFLICT (project_id, user_id, session_key) DO UPDATE SET
  label = CASE WHEN EXCLUDED.label <> '' THEN EXCLUDED.label ELSE agent_sessions.label END,
  client = CASE WHEN EXCLUDED.client <> '' THEN EXCLUDED.client ELSE agent_sessions.client END,
  token_id = COALESCE(EXCLUDED.token_id, agent_sessions.token_id),
  last_seen_at = now()
RETURNING id, project_id, token_id, user_id, session_key, label, client, created_at, last_seen_at;

-- name: GetAgentSessionsByIDs :many
SELECT id, project_id, token_id, user_id, session_key, label, client, created_at, last_seen_at
FROM agent_sessions WHERE id = ANY(sqlc.arg('ids')::uuid[]);
