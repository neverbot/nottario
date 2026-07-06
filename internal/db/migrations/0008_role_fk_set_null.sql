-- +goose Up
-- +goose StatementBegin
-- api_tokens.default_role_id previously had a plain FK (no ON DELETE
-- clause) which defaults to NO ACTION / RESTRICT — deleting a role
-- that any token used as its default would fail with a foreign-key
-- violation and surface to the user as a generic "delete failed".
-- Bring it in line with tasks.target_role_id (SET NULL): the token
-- keeps working, it just loses its default and agents fall back to
-- the per-call role hint (or none).
ALTER TABLE api_tokens
  DROP CONSTRAINT IF EXISTS api_tokens_default_role_id_fkey;
ALTER TABLE api_tokens
  ADD CONSTRAINT api_tokens_default_role_id_fkey
    FOREIGN KEY (default_role_id) REFERENCES roles(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE api_tokens
  DROP CONSTRAINT IF EXISTS api_tokens_default_role_id_fkey;
ALTER TABLE api_tokens
  ADD CONSTRAINT api_tokens_default_role_id_fkey
    FOREIGN KEY (default_role_id) REFERENCES roles(id);
-- +goose StatementEnd
