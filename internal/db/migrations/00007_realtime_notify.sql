-- +goose Up
-- A single trigger function that publishes one notification per row
-- mutation across every domain. The first trigger argument carries
-- the event type (`task.created`, `doc.updated`, etc.); the payload
-- always includes `project_id` for client-side fan-out filtering.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_event() RETURNS trigger AS $$
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

    -- pg_notify accepts at most 8000 bytes for the payload; ours is far smaller.
    PERFORM pg_notify('nottario_events', payload::text);

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Tasks
CREATE TRIGGER tasks_notify_insert AFTER INSERT ON tasks
    FOR EACH ROW EXECUTE FUNCTION notify_event('task.created');
CREATE TRIGGER tasks_notify_update AFTER UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION notify_event('task.updated');
CREATE TRIGGER tasks_notify_delete AFTER DELETE ON tasks
    FOR EACH ROW EXECUTE FUNCTION notify_event('task.deleted');

-- Documents
CREATE TRIGGER documents_notify_insert AFTER INSERT ON documents
    FOR EACH ROW EXECUTE FUNCTION notify_event('doc.created');
CREATE TRIGGER documents_notify_update AFTER UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION notify_event('doc.updated');
CREATE TRIGGER documents_notify_delete AFTER DELETE ON documents
    FOR EACH ROW EXECUTE FUNCTION notify_event('doc.deleted');

-- Architecture nodes
CREATE TRIGGER arch_nodes_notify_insert AFTER INSERT ON arch_nodes
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.node.created');
CREATE TRIGGER arch_nodes_notify_update AFTER UPDATE ON arch_nodes
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.node.updated');
CREATE TRIGGER arch_nodes_notify_delete AFTER DELETE ON arch_nodes
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.node.deleted');

-- Architecture edges
CREATE TRIGGER arch_edges_notify_insert AFTER INSERT ON arch_edges
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.edge.created');
CREATE TRIGGER arch_edges_notify_update AFTER UPDATE ON arch_edges
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.edge.updated');
CREATE TRIGGER arch_edges_notify_delete AFTER DELETE ON arch_edges
    FOR EACH ROW EXECUTE FUNCTION notify_event('arch.edge.deleted');

-- +goose Down
DROP TRIGGER IF EXISTS arch_edges_notify_delete ON arch_edges;
DROP TRIGGER IF EXISTS arch_edges_notify_update ON arch_edges;
DROP TRIGGER IF EXISTS arch_edges_notify_insert ON arch_edges;
DROP TRIGGER IF EXISTS arch_nodes_notify_delete ON arch_nodes;
DROP TRIGGER IF EXISTS arch_nodes_notify_update ON arch_nodes;
DROP TRIGGER IF EXISTS arch_nodes_notify_insert ON arch_nodes;
DROP TRIGGER IF EXISTS documents_notify_delete ON documents;
DROP TRIGGER IF EXISTS documents_notify_update ON documents;
DROP TRIGGER IF EXISTS documents_notify_insert ON documents;
DROP TRIGGER IF EXISTS tasks_notify_delete ON tasks;
DROP TRIGGER IF EXISTS tasks_notify_update ON tasks;
DROP TRIGGER IF EXISTS tasks_notify_insert ON tasks;
DROP FUNCTION IF EXISTS notify_event();
