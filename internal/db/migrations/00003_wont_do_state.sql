-- +goose Up
-- Extend the task lifecycle with a `wont_do` state for tasks we
-- deliberately decide NOT to do.
--
-- The DB value is `wont_do` (no apostrophe) so it slots cleanly into
-- the existing snake_case set (`todo`, `doing`, `done`) and never
-- needs SQL-string escaping; the UI label renders it as "Won't do".
ALTER TABLE tasks DROP CONSTRAINT tasks_state_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_state_check
    CHECK (state = ANY (ARRAY['todo'::text, 'doing'::text, 'done'::text, 'wont_do'::text]));

-- +goose Down
ALTER TABLE tasks DROP CONSTRAINT tasks_state_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_state_check
    CHECK (state = ANY (ARRAY['todo'::text, 'doing'::text, 'done'::text]));
