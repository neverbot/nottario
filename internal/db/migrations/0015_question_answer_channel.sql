-- +goose Up
-- Where an answer was given: in the web UI, or to the asking agent in
-- the chat (or another medium) and recorded by it through its token.
ALTER TABLE public.task_questions
    ADD COLUMN answered_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    ADD COLUMN answer_channel text NOT NULL DEFAULT 'web'
        CHECK (answer_channel IN ('web', 'chat', 'other'));

-- +goose Down
ALTER TABLE public.task_questions
    DROP COLUMN answer_channel,
    DROP COLUMN answered_by_token_id;
