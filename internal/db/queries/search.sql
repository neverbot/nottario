-- name: UnifiedSearch :many
-- Each domain returns the same shape, plus title_headline /
-- description_headline computed with ts_headline. The headline
-- options use rare sentinel strings («MARK» / «/MARK») instead of
-- HTML tags so the Go layer can html-escape the result safely and
-- only then swap the sentinels for <mark>...</mark>. This keeps any
-- raw '<' or '>' in user content escaped while still producing
-- highlighted snippets the UI can render with unsafeHTML.
WITH q AS (
    -- Union the same three dictionaries the search_vector columns
    -- use (see migrations/00013_search_multilang.sql) so a query for
    -- "task" matches stored "tasks" via the english stemmer, "tarea"
    -- matches "tareas" via the spanish stemmer, and verbatim words /
    -- slugs / non-Latin terms still match via the simple config.
    SELECT (
        plainto_tsquery('simple',  sqlc.arg('query')::text) ||
        plainto_tsquery('english', sqlc.arg('query')::text) ||
        plainto_tsquery('spanish', sqlc.arg('query')::text)
    ) AS tsq
)
(
    SELECT 'task'::text AS kind,
           project_id::text AS project_id,
           ts_rank(search_vector, (SELECT tsq FROM q))::real AS rank,
           title,
           left(coalesce(description_md, ''), 400) AS description,
           ts_headline('simple', title, (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=1,MaxWords=15,MinWords=5'
           ) AS title_headline,
           ts_headline('simple', left(coalesce(description_md, ''), 400), (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=2,MaxWords=20,MinWords=8,FragmentDelimiter=" … "'
           ) AS description_headline,
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
           left(coalesce(description, ''), 400) AS description,
           ts_headline('simple', title, (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=1,MaxWords=15,MinWords=5'
           ) AS title_headline,
           ts_headline('simple', left(coalesce(description, ''), 400), (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=2,MaxWords=20,MinWords=8,FragmentDelimiter=" … "'
           ) AS description_headline,
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
           left(coalesce(description_md, ''), 400) AS description,
           ts_headline('simple', name, (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=1,MaxWords=15,MinWords=5'
           ) AS title_headline,
           ts_headline('simple', left(coalesce(description_md, ''), 400), (SELECT tsq FROM q),
             'StartSel=«MARK»,StopSel=«/MARK»,MaxFragments=2,MaxWords=20,MinWords=8,FragmentDelimiter=" … "'
           ) AS description_headline,
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
