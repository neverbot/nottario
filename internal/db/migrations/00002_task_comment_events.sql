-- +goose Up
-- Emit realtime events when task comments are inserted, updated or
-- deleted. Without these, an open task-detail dialog never sees a
-- new comment posted by a different agent / human and stays stale.
--
-- The `notify_event` function already handles `task_comments` at the
-- bottom of its body so we only need to attach the triggers. The
-- payload it produces carries `project_id` (resolved via a subquery
-- on the comment's owning task) and `task_id`, which is enough for
-- the SSE hub to fan the event out to project subscribers and for
-- the dialog to call `loadDetail(task_id)` on receipt.

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

CREATE TRIGGER task_comments_notify_insert
    AFTER INSERT ON public.task_comments
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.comment.created');

CREATE TRIGGER task_comments_notify_update
    AFTER UPDATE ON public.task_comments
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.comment.updated');

CREATE TRIGGER task_comments_notify_delete
    AFTER DELETE ON public.task_comments
    FOR EACH ROW
    EXECUTE FUNCTION public.notify_event('task.comment.deleted');

-- +goose Down
DROP TRIGGER IF EXISTS task_comments_notify_delete ON public.task_comments;
DROP TRIGGER IF EXISTS task_comments_notify_update ON public.task_comments;
DROP TRIGGER IF EXISTS task_comments_notify_insert ON public.task_comments;
-- Restore the original notify_event function (without the
-- task_comments branch) so a downgrade leaves the codebase
-- consistent with what 00001_init.sql defines.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.notify_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    payload jsonb;
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
    END IF;

    PERFORM pg_notify('nottario_events', payload::text);

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
