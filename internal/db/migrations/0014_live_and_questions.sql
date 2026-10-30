-- +goose Up
-- Agent sessions: one row per agent conversation, keyed by an id the
-- client supplies (Claude Code: $CLAUDE_CODE_SESSION_ID), so the UI can
-- say which conversation holds a task and how to get back to it.
CREATE TABLE public.agent_sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   uuid NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
    token_id     uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    user_id      uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    session_key  text NOT NULL CHECK (char_length(session_key) BETWEEN 1 AND 128),
    label        text NOT NULL DEFAULT '' CHECK (char_length(label) <= 80),
    client       text NOT NULL DEFAULT '' CHECK (char_length(client) <= 80),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_sessions_key_unique UNIQUE (project_id, user_id, session_key)
);

ALTER TABLE public.tasks
    ADD COLUMN active_session_id uuid REFERENCES public.agent_sessions(id) ON DELETE SET NULL;
CREATE INDEX tasks_active_session_idx ON public.tasks (active_session_id)
    WHERE active_session_id IS NOT NULL;

-- Last activity per task lives in its own table so that writes to steps,
-- comments, commits and questions never touch the tasks row (which would
-- bump tasks.updated_at, the conflict token of task edits, and fire its
-- triggers).
CREATE TABLE public.task_activity (
    task_id          uuid PRIMARY KEY REFERENCES public.tasks(id) ON DELETE CASCADE,
    last_activity_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO public.task_activity (task_id, last_activity_at)
    SELECT id, updated_at FROM public.tasks;

ALTER TABLE public.task_steps
    ADD COLUMN started_at timestamptz,
    ADD COLUMN started_session_id uuid REFERENCES public.agent_sessions(id) ON DELETE SET NULL,
    ADD COLUMN started_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL;
CREATE INDEX task_steps_started_session_idx ON public.task_steps (started_session_id)
    WHERE started_session_id IS NOT NULL;

-- project_id and step_id must agree with task_id (same project, step of
-- that task); the application enforces it, the schema does not.
CREATE TABLE public.task_questions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          uuid NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
    task_id             uuid NOT NULL REFERENCES public.tasks(id) ON DELETE CASCADE,
    step_id             uuid REFERENCES public.task_steps(id) ON DELETE SET NULL,
    session_id          uuid REFERENCES public.agent_sessions(id) ON DELETE SET NULL,
    asked_by_user_id    uuid REFERENCES public.users(id) ON DELETE SET NULL,
    asked_by_token_id   uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    recipient_user_id   uuid REFERENCES public.users(id) ON DELETE SET NULL,
    kind                text NOT NULL CHECK (kind IN ('text', 'choice')),
    body_md             text NOT NULL CHECK (char_length(body_md) BETWEEN 1 AND 4000),
    options             jsonb,
    multi               boolean NOT NULL DEFAULT false,
    recommended_key     text,
    recommended_reason  text NOT NULL DEFAULT '' CHECK (char_length(recommended_reason) <= 300),
    state               text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'answered', 'withdrawn')),
    answer_keys         text[] NOT NULL DEFAULT '{}',
    answer_text         text NOT NULL DEFAULT '' CHECK (char_length(answer_text) <= 8000),
    answer_note         text NOT NULL DEFAULT '' CHECK (char_length(answer_note) <= 8000),
    answered_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL,
    -- Set when the asking agent recorded an answer the human gave it
    -- outside Nottario (answer_channel says where); null for answers
    -- given in the web UI.
    answered_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    answer_channel      text NOT NULL DEFAULT 'web' CHECK (answer_channel IN ('web', 'chat', 'other')),
    answered_at         timestamptz,
    delivered_at        timestamptz,
    withdrawn_at        timestamptz,
    withdrawn_reason    text NOT NULL DEFAULT '' CHECK (char_length(withdrawn_reason) <= 300),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT task_questions_choice_has_options CHECK ((kind = 'choice') = (options IS NOT NULL)),
    CONSTRAINT task_questions_options_array CHECK (options IS NULL OR jsonb_typeof(options) = 'array'),
    CONSTRAINT task_questions_withdrawn CHECK ((state = 'withdrawn') = (withdrawn_at IS NOT NULL)),
    CONSTRAINT task_questions_answer_keys_max CHECK (cardinality(answer_keys) <= 9),
    CONSTRAINT task_questions_answered CHECK ((state = 'answered') = (answered_at IS NOT NULL)),
    CONSTRAINT task_questions_delivered CHECK (delivered_at IS NULL OR state = 'answered')
);
CREATE INDEX task_questions_recipient_idx ON public.task_questions (recipient_user_id, state, created_at);
CREATE INDEX task_questions_project_idx ON public.task_questions (project_id, state, created_at);
CREATE INDEX task_questions_task_idx ON public.task_questions (task_id);
CREATE INDEX task_questions_step_idx ON public.task_questions (step_id) WHERE step_id IS NOT NULL;
CREATE INDEX task_questions_undelivered_idx ON public.task_questions (session_id)
    WHERE state = 'answered' AND delivered_at IS NULL;

ALTER TABLE public.projects
    ADD COLUMN live_idle_minutes integer NOT NULL DEFAULT 30
        CHECK (live_idle_minutes BETWEEN 5 AND 1440);
ALTER TABLE public.projects DROP CONSTRAINT projects_default_view_check;
ALTER TABLE public.projects ADD CONSTRAINT projects_default_view_check CHECK (default_view = ANY (ARRAY[
    'board/kanban', 'board/gantt', 'live', 'questions', 'docs', 'arch/diagram', 'arch/tree']));

-- Activity: any write on a task or on what hangs off it moves the task
-- to the top of Live.
--
-- The trigger never waits on a task_activity row. Writers take their
-- locks in different orders (closing a task locks the task, then its
-- questions; answering locks the question; a close with a comment
-- inserts the comment before locking the task), so a trigger that
-- waited on the activity row would close a cycle and deadlock. Every
-- task has its row (backfilled above, inserted by the tasks INSERT
-- path with DO NOTHING, which takes no lock on a conflict), and a bump
-- skips a row another transaction holds: that transaction is bumping
-- it anyway, to the same moment or near enough. The bump is also
-- skipped when the row already carries now() (many writes in one
-- transaction). A bump for a task being deleted finds no row and does
-- nothing.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.task_touch_activity() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    tid uuid;
BEGIN
    IF TG_TABLE_NAME = 'tasks' AND TG_OP = 'INSERT' THEN
        INSERT INTO public.task_activity (task_id, last_activity_at)
            VALUES (NEW.id, now())
        ON CONFLICT (task_id) DO NOTHING;
        RETURN NEW;
    END IF;
    IF TG_TABLE_NAME = 'tasks' THEN
        tid := NEW.id;
    ELSIF TG_OP = 'DELETE' THEN
        tid := OLD.task_id;
    ELSE
        tid := NEW.task_id;
    END IF;
    UPDATE public.task_activity SET last_activity_at = now()
    WHERE task_id = (
        SELECT a.task_id FROM public.task_activity a
        WHERE a.task_id = tid AND a.last_activity_at IS DISTINCT FROM now()
        FOR UPDATE SKIP LOCKED);
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER task_steps_touch_activity AFTER INSERT OR UPDATE OR DELETE ON public.task_steps
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();
CREATE TRIGGER task_comments_touch_activity AFTER INSERT ON public.task_comments
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();
CREATE TRIGGER task_commits_touch_activity AFTER INSERT ON public.task_commits
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();
CREATE TRIGGER task_questions_touch_activity AFTER INSERT OR UPDATE ON public.task_questions
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();
CREATE TRIGGER tasks_touch_activity_insert AFTER INSERT ON public.tasks
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();
-- Column-scoped so cycle_id-only bulk moves and similar bookkeeping do not
-- count as activity.
CREATE TRIGGER tasks_touch_activity_update
    AFTER UPDATE OF state, assignee_user_id, title, description_md, type, priority, target_role_id, parent_task_id
    ON public.tasks
    FOR EACH ROW EXECUTE FUNCTION public.task_touch_activity();

-- Question events reach SSE like step events (routed through the owning
-- task, see notify_event below).
CREATE TRIGGER task_questions_notify_insert AFTER INSERT ON public.task_questions
    FOR EACH ROW EXECUTE FUNCTION public.notify_event('question.created');
CREATE TRIGGER task_questions_notify_update AFTER UPDATE ON public.task_questions
    FOR EACH ROW EXECUTE FUNCTION public.notify_event('question.updated');

-- notify_event learns task_questions (copy of 0012 with one more table
-- in the task-child branch).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.notify_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    payload jsonb;
    pid     uuid;
    tid     uuid;
BEGIN
    payload := jsonb_build_object(
        'type', TG_ARGV[0],
        'op',   TG_OP
    );

    IF TG_TABLE_NAME = 'tasks' THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id,
                'task_id',    OLD.id
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id,
                'task_id',    NEW.id
            );
        END IF;
    ELSIF TG_TABLE_NAME = 'documents' THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id,
                'scope',      OLD.scope,
                'path',       OLD.path
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id,
                'scope',      NEW.scope,
                'path',       NEW.path
            );
        END IF;
    ELSIF TG_TABLE_NAME IN ('arch_nodes', 'arch_edges') THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id
            );
        END IF;
    ELSIF TG_TABLE_NAME IN ('task_comments', 'task_steps', 'task_questions') THEN
        -- These tables have no project_id we can rely on here; resolve it
        -- from the owning task so the SSE hub can route the event.
        IF TG_OP = 'DELETE' THEN
            tid := OLD.task_id;
        ELSE
            tid := NEW.task_id;
        END IF;
        SELECT project_id INTO pid FROM tasks WHERE id = tid;
        payload := payload || jsonb_build_object(
            'project_id', pid,
            'task_id',    tid
        );
    END IF;

    PERFORM pg_notify('nottario_events', payload::text);

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS task_questions_notify_update ON public.task_questions;
DROP TRIGGER IF EXISTS task_questions_notify_insert ON public.task_questions;
DROP TRIGGER IF EXISTS tasks_touch_activity_update ON public.tasks;
DROP TRIGGER IF EXISTS tasks_touch_activity_insert ON public.tasks;
DROP TRIGGER IF EXISTS task_questions_touch_activity ON public.task_questions;
DROP TRIGGER IF EXISTS task_commits_touch_activity ON public.task_commits;
DROP TRIGGER IF EXISTS task_comments_touch_activity ON public.task_comments;
DROP TRIGGER IF EXISTS task_steps_touch_activity ON public.task_steps;
DROP FUNCTION IF EXISTS public.task_touch_activity();
DROP TABLE IF EXISTS public.task_questions;
ALTER TABLE public.task_steps
    DROP COLUMN started_by_user_id,
    DROP COLUMN started_session_id,
    DROP COLUMN started_at;
DROP TABLE IF EXISTS public.task_activity;
DROP INDEX IF EXISTS public.tasks_active_session_idx;
ALTER TABLE public.tasks
    DROP COLUMN active_session_id;
DROP TABLE IF EXISTS public.agent_sessions;
UPDATE public.projects SET default_view = 'board/kanban' WHERE default_view IN ('live', 'questions');
ALTER TABLE public.projects DROP CONSTRAINT projects_default_view_check;
ALTER TABLE public.projects ADD CONSTRAINT projects_default_view_check CHECK ((default_view = ANY (ARRAY['board/kanban'::text, 'board/gantt'::text, 'docs'::text, 'arch/diagram'::text, 'arch/tree'::text])));
ALTER TABLE public.projects DROP COLUMN live_idle_minutes;
-- Restore notify_event as 0012 defines it.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.notify_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    payload jsonb;
    pid     uuid;
    tid     uuid;
BEGIN
    payload := jsonb_build_object(
        'type', TG_ARGV[0],
        'op',   TG_OP
    );

    IF TG_TABLE_NAME = 'tasks' THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id,
                'task_id',    OLD.id
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id,
                'task_id',    NEW.id
            );
        END IF;
    ELSIF TG_TABLE_NAME = 'documents' THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id,
                'scope',      OLD.scope,
                'path',       OLD.path
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id,
                'scope',      NEW.scope,
                'path',       NEW.path
            );
        END IF;
    ELSIF TG_TABLE_NAME IN ('arch_nodes', 'arch_edges') THEN
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object(
                'project_id', OLD.project_id
            );
        ELSE
            payload := payload || jsonb_build_object(
                'project_id', NEW.project_id
            );
        END IF;
    ELSIF TG_TABLE_NAME IN ('task_comments', 'task_steps') THEN
        -- Neither table has a project_id column; resolve it from the
        -- owning task so the SSE hub can route the event correctly.
        IF TG_OP = 'DELETE' THEN
            tid := OLD.task_id;
        ELSE
            tid := NEW.task_id;
        END IF;
        SELECT project_id INTO pid FROM tasks WHERE id = tid;
        payload := payload || jsonb_build_object(
            'project_id', pid,
            'task_id',    tid
        );
    END IF;

    PERFORM pg_notify('nottario_events', payload::text);

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
