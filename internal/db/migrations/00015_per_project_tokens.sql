-- +goose Up
--
-- Per-project API tokens — phase 1 of 2.
--
-- Adds api_tokens.project_id (nullable for now) and backfills existing
-- non-revoked tokens deterministically. The column is left nullable
-- here on purpose: the follow-up Go/sqlc rework (task 83e27358) is
-- what flips it to NOT NULL once the query layer and every caller
-- pass project_id explicitly. Splitting the change this way keeps the
-- build green between commits and avoids a long-running PR.

ALTER TABLE api_tokens
    ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE CASCADE;

CREATE INDEX api_tokens_project_idx ON api_tokens(project_id);

-- Backfill: each non-revoked token gets pinned to its user's
-- earliest-joined project membership. The pick is deterministic
-- (memberships.created_at ASC, then project_id ASC as tie-break) so
-- re-running the migration on a snapshot produces the same mapping.
--
-- Tokens whose owning user has zero memberships keep project_id NULL.
-- This is intentional: deciding to revoke them is the operator's
-- call, not the migration's.
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
  AND t.revoked_at IS NULL
  AND t.project_id IS NULL;

-- Surface any non-revoked, unbackfilled tokens for the operator.
-- Not an error: the user just has no memberships yet, so we can't
-- safely guess a project for them.
-- +goose StatementBegin
DO $$
DECLARE
    n int;
BEGIN
    SELECT count(*) INTO n
    FROM api_tokens
    WHERE revoked_at IS NULL AND project_id IS NULL;
    IF n > 0 THEN
        RAISE NOTICE 'per-project-tokens: % non-revoked token(s) left without project_id (owners have no memberships). Revoke and re-issue them, or grant the owner a membership before the follow-up migration sets NOT NULL.', n;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS api_tokens_project_idx;
ALTER TABLE api_tokens DROP COLUMN IF EXISTS project_id;
