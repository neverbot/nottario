-- name: CountArchKinds :one
SELECT COUNT(*)::int AS n FROM arch_node_kinds WHERE project_id = $1;

-- name: InsertDefaultArchKind :exec
INSERT INTO arch_node_kinds (project_id, key, label, icon, color, description, is_default)
VALUES ($1, $2, $3, $4, $5, $6, true)
ON CONFLICT DO NOTHING;

-- name: ListArchKinds :many
SELECT project_id, key, label, icon, color, description, is_default, created_at
FROM arch_node_kinds
WHERE project_id = $1
ORDER BY is_default DESC, label;

-- name: UpsertArchKind :one
INSERT INTO arch_node_kinds (project_id, key, label, icon, color, description, is_default)
VALUES ($1, $2, $3, $4, $5, $6, false)
ON CONFLICT (project_id, key) DO UPDATE
SET label = EXCLUDED.label,
    icon = EXCLUDED.icon,
    color = EXCLUDED.color,
    description = EXCLUDED.description
RETURNING project_id, key, label, icon, color, description, is_default, created_at;

-- name: CountNodesByKind :one
SELECT COUNT(*)::int AS n FROM arch_nodes WHERE project_id = $1 AND kind = $2;

-- name: DeleteArchKind :execrows
DELETE FROM arch_node_kinds WHERE project_id = $1 AND key = $2;

-- name: ArchKindExists :one
SELECT EXISTS (SELECT 1 FROM arch_node_kinds WHERE project_id = $1 AND key = $2) AS ok;

-- name: GetArchNodeIDBySlug :one
SELECT id FROM arch_nodes WHERE project_id = $1 AND slug = $2;

-- name: InsertArchNode :one
INSERT INTO arch_nodes (project_id, slug, parent_id, kind, name, description_md,
                        metadata, linked_repo, linked_path, position,
                        created_by_user_id, created_by_token_id,
                        updated_by_user_id, updated_by_token_id)
VALUES (
    sqlc.arg('project_id')::uuid,
    sqlc.arg('slug')::text,
    sqlc.narg('parent_id')::uuid,
    sqlc.arg('kind')::text,
    sqlc.arg('name')::text,
    sqlc.arg('description_md')::text,
    sqlc.arg('metadata')::jsonb,
    sqlc.narg('linked_repo')::text,
    sqlc.narg('linked_path')::text,
    sqlc.arg('position')::int,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid
)
RETURNING id, project_id, slug, parent_id, kind, name, description_md,
          metadata, linked_repo, linked_path, position,
          created_at, updated_at;

-- name: UpdateArchNode :one
UPDATE arch_nodes
SET parent_id = sqlc.narg('parent_id')::uuid,
    kind = sqlc.arg('kind')::text,
    name = sqlc.arg('name')::text,
    description_md = sqlc.arg('description_md')::text,
    metadata = sqlc.arg('metadata')::jsonb,
    linked_repo = sqlc.narg('linked_repo')::text,
    linked_path = sqlc.narg('linked_path')::text,
    position = sqlc.arg('position')::int,
    updated_by_user_id = sqlc.narg('author_user_id')::uuid,
    updated_by_token_id = sqlc.narg('author_token_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, project_id, slug, parent_id, kind, name, description_md,
          metadata, linked_repo, linked_path, position,
          created_at, updated_at;

-- name: GetArchNodeBySlug :one
SELECT id, project_id, slug, parent_id, kind, name, description_md,
       metadata, linked_repo, linked_path, position, created_at, updated_at
FROM arch_nodes
WHERE project_id = $1 AND slug = $2;

-- name: ListArchNodes :many
SELECT n.id, n.project_id, n.slug, n.parent_id, n.kind, n.name, n.description_md,
       n.metadata, n.linked_repo, n.linked_path, n.position, n.created_at, n.updated_at
FROM arch_nodes n
WHERE n.project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('parent_slug')::text IS NULL OR n.parent_id = (
       SELECT p.id FROM arch_nodes p
       WHERE p.project_id = sqlc.arg('project_id')::uuid
         AND p.slug = sqlc.narg('parent_slug')::text))
  AND (NOT sqlc.arg('root_only')::bool OR n.parent_id IS NULL)
ORDER BY n.position, n.slug;

-- name: CountArchNodeChildren :one
SELECT COUNT(*)::int AS n FROM arch_nodes WHERE parent_id = $1;

-- name: DeleteArchNode :exec
DELETE FROM arch_nodes WHERE id = $1;

-- name: MoveArchNode :one
UPDATE arch_nodes
SET parent_id = sqlc.narg('parent_id')::uuid,
    updated_by_user_id = sqlc.narg('author_user_id')::uuid,
    updated_by_token_id = sqlc.narg('author_token_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, project_id, slug, parent_id, kind, name, description_md,
          metadata, linked_repo, linked_path, position, created_at, updated_at;

-- name: ArchNodeCycleCheck :one
WITH RECURSIVE
ancestors(id) AS (
    SELECT id FROM arch_nodes WHERE id = sqlc.arg('new_parent_id')::uuid
    UNION
    SELECT a.parent_id FROM arch_nodes a
    JOIN ancestors r ON r.id = a.id
    WHERE a.parent_id IS NOT NULL
),
descendants(id) AS (
    SELECT id FROM arch_nodes WHERE id = sqlc.arg('node_id')::uuid
    UNION
    SELECT a.id FROM arch_nodes a
    JOIN descendants d ON d.id = a.parent_id
    WHERE a.project_id = sqlc.arg('project_id')::uuid
)
SELECT EXISTS (
    SELECT 1 FROM ancestors anc
    JOIN descendants d ON d.id = anc.id
) AS hit;

-- name: UpsertArchEdge :one
INSERT INTO arch_edges (project_id, from_node_id, to_node_id, kind, label, description_md,
                        created_by_user_id, created_by_token_id,
                        updated_by_user_id, updated_by_token_id)
VALUES (
    sqlc.arg('project_id')::uuid,
    sqlc.arg('from_node_id')::uuid,
    sqlc.arg('to_node_id')::uuid,
    sqlc.arg('kind')::text,
    sqlc.arg('label')::text,
    sqlc.arg('description_md')::text,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid
)
ON CONFLICT (project_id, from_node_id, to_node_id, kind) DO UPDATE
SET label = EXCLUDED.label,
    description_md = EXCLUDED.description_md,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_by_token_id = EXCLUDED.updated_by_token_id,
    updated_at = now()
RETURNING id, project_id, from_node_id, to_node_id, kind, label, description_md, created_at, updated_at;

-- name: DeleteArchEdge :execrows
DELETE FROM arch_edges WHERE project_id = $1 AND id = $2;

-- name: ListArchEdges :many
SELECT e.id, e.project_id, e.from_node_id, e.to_node_id, e.kind, e.label, e.description_md,
       e.created_at, e.updated_at,
       a.slug AS from_slug, a.name AS from_name,
       b.slug AS to_slug,   b.name AS to_name
FROM arch_edges e
JOIN arch_nodes a ON a.id = e.from_node_id
JOIN arch_nodes b ON b.id = e.to_node_id
WHERE e.project_id = $1
  AND (sqlc.narg('kind')::text IS NULL OR e.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('node_id')::uuid IS NULL
       OR (sqlc.arg('direction')::text = 'out' AND e.from_node_id = sqlc.narg('node_id')::uuid)
       OR (sqlc.arg('direction')::text = 'in'  AND e.to_node_id   = sqlc.narg('node_id')::uuid)
       OR (sqlc.arg('direction')::text NOT IN ('out','in')
           AND (e.from_node_id = sqlc.narg('node_id')::uuid
                OR e.to_node_id = sqlc.narg('node_id')::uuid)))
ORDER BY e.kind, a.slug, b.slug;

-- name: InsertArchNodeLink :exec
INSERT INTO arch_node_links (project_id, node_id, link_type, target_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: DeleteArchNodeLink :exec
DELETE FROM arch_node_links
WHERE project_id = $1 AND node_id = $2 AND link_type = $3 AND target_id = $4;

-- name: ListArchNodeLinks :many
SELECT project_id, node_id, link_type, target_id, created_at
FROM arch_node_links WHERE node_id = $1
ORDER BY link_type, target_id;
