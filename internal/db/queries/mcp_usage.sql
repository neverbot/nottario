-- name: RecordMcpUsage :exec
INSERT INTO mcp_usage_daily (day, project_id, token_id, tool, calls, errors, bytes_in, bytes_out)
VALUES (
    (now() AT TIME ZONE 'UTC')::date,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('token_id')::uuid,
    sqlc.arg('tool')::text,
    1,
    sqlc.arg('errors')::bigint,
    sqlc.arg('bytes_in')::bigint,
    sqlc.arg('bytes_out')::bigint
)
ON CONFLICT (project_id, day, token_id, tool) DO UPDATE SET
    calls     = mcp_usage_daily.calls + 1,
    errors    = mcp_usage_daily.errors + EXCLUDED.errors,
    bytes_in  = mcp_usage_daily.bytes_in + EXCLUDED.bytes_in,
    bytes_out = mcp_usage_daily.bytes_out + EXCLUDED.bytes_out;

-- name: ListMcpUsageRows :many
-- Per token and tool over the window, with the token's name and owner,
-- so the web layer can show per-tool totals and per-token rows.
SELECT u.token_id,
       t.name AS token_name,
       t.user_id AS token_user_id,
       (t.revoked_at IS NOT NULL)::bool AS token_revoked,
       u.tool,
       sum(u.calls)::bigint AS calls,
       sum(u.errors)::bigint AS errors,
       sum(u.bytes_in)::bigint AS bytes_in,
       sum(u.bytes_out)::bigint AS bytes_out
FROM mcp_usage_daily u
JOIN api_tokens t ON t.id = u.token_id
WHERE u.project_id = sqlc.arg('project_id')::uuid
  AND u.day >= sqlc.arg('since')::date
GROUP BY u.token_id, t.name, t.user_id, t.revoked_at, u.tool
ORDER BY sum(u.bytes_out) DESC;

-- name: DeleteMcpUsageBefore :execrows
DELETE FROM mcp_usage_daily WHERE day < sqlc.arg('before')::date;
