-- name: UnifiedSearch :many
WITH q AS (SELECT plainto_tsquery('simple', sqlc.arg('query')::text) AS tsq)
(
    SELECT 'task'::text AS kind,
           project_id::text AS project_id,
           ts_rank(search_vector, (SELECT tsq FROM q))::real AS rank,
           title,
           left(coalesce(description_md, ''), 200) AS description,
           id::text AS task_id,
           ''::text AS doc_path, ''::text AS doc_scope,
           ''::text AS node_slug, ''::text AS node_kind,
           state AS task_state, type AS task_type
    FROM tasks
    WHERE project_id = sqlc.arg('project_id')::uuid
      AND search_vector @@ (SELECT tsq FROM q)
      AND sqlc.arg('include_task')::bool
)
UNION ALL
(
    SELECT 'document'::text AS kind,
           coalesce(project_id::text, '') AS project_id,
           ts_rank(search_vector, (SELECT tsq FROM q))::real AS rank,
           title,
           left(coalesce(description, ''), 200) AS description,
           ''::text AS task_id,
           path AS doc_path, scope AS doc_scope,
           ''::text AS node_slug, ''::text AS node_kind,
           ''::text AS task_state, ''::text AS task_type
    FROM documents
    WHERE (project_id = sqlc.arg('project_id')::uuid OR scope = 'global')
      AND deleted_at IS NULL
      AND search_vector @@ (SELECT tsq FROM q)
      AND sqlc.arg('include_document')::bool
)
UNION ALL
(
    SELECT 'arch_node'::text AS kind,
           project_id::text AS project_id,
           ts_rank(search_vector, (SELECT tsq FROM q))::real AS rank,
           name AS title,
           left(coalesce(description_md, ''), 200) AS description,
           ''::text AS task_id,
           ''::text AS doc_path, ''::text AS doc_scope,
           slug AS node_slug, kind AS node_kind,
           ''::text AS task_state, ''::text AS task_type
    FROM arch_nodes
    WHERE project_id = sqlc.arg('project_id')::uuid
      AND search_vector @@ (SELECT tsq FROM q)
      AND sqlc.arg('include_arch_node')::bool
)
ORDER BY rank DESC
LIMIT sqlc.arg('lim')::int;
