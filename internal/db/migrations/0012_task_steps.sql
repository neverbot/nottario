-- +goose Up
-- Steps: a short ordered checklist on a task. Each step is a row, not
-- an element of an array on tasks, so two agents can tick different
-- steps at once and every step has a stable id to address it by.
--
-- position is dense 0..n-1 per task; the application rewrites it with
-- the task row locked, so it carries no unique constraint (a shift
-- would trip it mid-statement).
CREATE TABLE public.task_steps (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id             uuid NOT NULL REFERENCES public.tasks(id) ON DELETE CASCADE,
    position            integer NOT NULL CHECK (position >= 0),
    text                text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 200),
    done                boolean NOT NULL DEFAULT false,
    done_at             timestamptz,
    done_by_user_id     uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_by_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT task_steps_done_at CHECK (done = (done_at IS NOT NULL))
);
CREATE INDEX task_steps_task_position_idx ON public.task_steps (task_id, position);

-- Step events route like comment events: project_id resolved from the
-- owning task.
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

CREATE TRIGGER task_steps_notify_insert
    AFTER INSERT ON public.task_steps
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.step.created');

CREATE TRIGGER task_steps_notify_update
    AFTER UPDATE ON public.task_steps
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.step.updated');

CREATE TRIGGER task_steps_notify_delete
    AFTER DELETE ON public.task_steps
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.step.deleted');

-- +goose Down
DROP TABLE IF EXISTS public.task_steps;
-- Restore notify_event as 00002 defines it.
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
    ELSIF TG_TABLE_NAME = 'task_comments' THEN
        -- task_comments has no project_id column; resolve it from the
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
