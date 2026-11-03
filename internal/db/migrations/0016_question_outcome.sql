-- +goose Up
-- The decision an answered question led to: one formal line the asking
-- agent writes once it has the answer, read in place of the raw reply.
ALTER TABLE public.task_questions
    ADD COLUMN outcome text NOT NULL DEFAULT ''
        CHECK (char_length(outcome) <= 300),
    ADD COLUMN outcome_at timestamptz;

-- +goose Down
ALTER TABLE public.task_questions
    DROP COLUMN outcome_at,
    DROP COLUMN outcome;
