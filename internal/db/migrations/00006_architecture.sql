-- +goose Up
CREATE TABLE arch_node_kinds (
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key          text NOT NULL,
    label        text NOT NULL,
    icon         text NOT NULL DEFAULT '',
    color        text NOT NULL DEFAULT '',
    description  text NOT NULL DEFAULT '',
    is_default   boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, key)
);

CREATE TABLE arch_nodes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    slug            text NOT NULL,
    parent_id       uuid REFERENCES arch_nodes(id) ON DELETE CASCADE,
    kind            text NOT NULL,
    name            text NOT NULL,
    description_md  text NOT NULL DEFAULT '',
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    linked_repo     text,
    linked_path     text,
    position        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);
CREATE INDEX arch_nodes_project_idx ON arch_nodes(project_id);
CREATE INDEX arch_nodes_parent_idx  ON arch_nodes(parent_id);

CREATE TABLE arch_edges (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    from_node_id    uuid NOT NULL REFERENCES arch_nodes(id) ON DELETE CASCADE,
    to_node_id      uuid NOT NULL REFERENCES arch_nodes(id) ON DELETE CASCADE,
    kind            text NOT NULL,
    label           text NOT NULL DEFAULT '',
    description_md  text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (from_node_id <> to_node_id)
);
CREATE INDEX arch_edges_from_idx ON arch_edges(project_id, from_node_id);
CREATE INDEX arch_edges_to_idx   ON arch_edges(project_id, to_node_id);
CREATE UNIQUE INDEX arch_edges_unique_idx ON arch_edges(project_id, from_node_id, to_node_id, kind);

CREATE TABLE arch_node_links (
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    node_id     uuid NOT NULL REFERENCES arch_nodes(id) ON DELETE CASCADE,
    link_type   text NOT NULL,
    target_id   text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (node_id, link_type, target_id),
    CHECK (link_type IN ('doc', 'task'))
);
CREATE INDEX arch_node_links_target_idx ON arch_node_links(link_type, target_id);

-- Bump updated_at on node and edge changes.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_arch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER arch_nodes_touch_updated_at
    BEFORE UPDATE ON arch_nodes
    FOR EACH ROW EXECUTE FUNCTION touch_arch_updated_at();

CREATE TRIGGER arch_edges_touch_updated_at
    BEFORE UPDATE ON arch_edges
    FOR EACH ROW EXECUTE FUNCTION touch_arch_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS arch_edges_touch_updated_at ON arch_edges;
DROP TRIGGER IF EXISTS arch_nodes_touch_updated_at ON arch_nodes;
DROP FUNCTION IF EXISTS touch_arch_updated_at();
DROP TABLE IF EXISTS arch_node_links;
DROP TABLE IF EXISTS arch_edges;
DROP TABLE IF EXISTS arch_nodes;
DROP TABLE IF EXISTS arch_node_kinds;
