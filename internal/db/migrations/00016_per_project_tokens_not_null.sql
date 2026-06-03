-- +goose Up
--
-- Per-project API tokens — phase 2 of 2.
--
-- Migration 00015 added api_tokens.project_id as nullable and
-- backfilled non-revoked rows. The Go layer (sqlc queries, identity
-- helpers, REST routes) now always supplies project_id explicitly, so
-- this migration finalises the schema:
--
--   1. Backfill revoked tokens too (00015 deliberately skipped them;
--      ALTER … SET NOT NULL checks every row, so we need a value or
--      a delete for each one).
--   2. Drop the orphans that even the wider backfill can't pin to a
--      project — these are revoked tokens whose owner has zero
--      memberships. Safe to drop: revoked tokens are non-functional.
--   3. ALTER … SET NOT NULL.
--   4. UNIQUE (project_id, name): token names are now scoped per
--      project; the frontend nudges in this direction already.
--
-- The api_tokens_user_idx single-column index is kept: lookup/revoke
-- still touch user_id alone in the RevokeAPIToken WHERE clause, and
-- the index is cheap.

WITH first_membership AS (
    SELECT DISTINCT ON (m.user_id)
        m.user_id, m.project_id
    FROM memberships m
    ORDER BY m.user_id, m.created_at ASC, m.project_id ASC
)
UPDATE api_tokens t
SET project_id = fm.project_id
FROM first_membership fm
WHERE t.user_id = fm.user_id
  AND t.project_id IS NULL;

-- Drop orphans: revoked, owner has no memberships, no way to pin a
-- project. These cannot be used (revoked_at is set) and the schema
-- would otherwise reject the NOT NULL flip.
DELETE FROM api_tokens WHERE project_id IS NULL;

ALTER TABLE api_tokens ALTER COLUMN project_id SET NOT NULL;

ALTER TABLE api_tokens ADD CONSTRAINT api_tokens_project_name_unique UNIQUE (project_id, name);

-- +goose Down
ALTER TABLE api_tokens DROP CONSTRAINT IF EXISTS api_tokens_project_name_unique;
ALTER TABLE api_tokens ALTER COLUMN project_id DROP NOT NULL;
