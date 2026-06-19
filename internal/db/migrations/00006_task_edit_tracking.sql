-- +goose Up
-- +goose StatementBegin
--
-- Per-row edit-tracking metadata for tasks and task_comments. Lets the
-- UI render a GitHub-style "(edited Xh ago by @user)" badge after a
-- human edits the title/description of a task or the body of a
-- comment. Also powers optimistic-concurrency on PATCH: the client
-- echoes the `updated_at` it saw, the server rejects with 409 if the
-- row has moved on since.
--
-- We deliberately don't add a history table. Tasks and comments churn
-- a lot during refinement; a versioned snapshot would balloon the DB
-- with little payoff. If "show prior versions" is ever asked for, a
-- `task_edits` history table can be added additively without breaking
-- anything.

ALTER TABLE public.tasks
    ADD COLUMN edited_at         timestamptz,
    ADD COLUMN edited_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE public.task_comments
    ADD COLUMN edited_at         timestamptz,
    ADD COLUMN edited_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL,
    ADD COLUMN updated_at        timestamptz NOT NULL DEFAULT now();

-- Backfill updated_at on pre-existing rows so the optimistic-concurrency
-- check has a usable value from day one.
UPDATE public.task_comments SET updated_at = created_at WHERE updated_at IS NULL OR updated_at = now();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.task_comments
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS edited_by_user_id,
    DROP COLUMN IF EXISTS edited_at;

ALTER TABLE public.tasks
    DROP COLUMN IF EXISTS edited_by_user_id,
    DROP COLUMN IF EXISTS edited_at;
-- +goose StatementEnd
