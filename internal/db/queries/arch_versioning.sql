-- Arch versioning: per-project editing sessions (locks) and their
-- snapshot revisions. The lock is the singleton "open session" per
-- project; revisions is the append-only log of closed sessions.

-- name: GetArchLock :one
SELECT project_id, author_user_id, author_token_id, locked_at,
       last_write_at, write_count, base_version
FROM arch_locks
WHERE project_id = sqlc.arg('project_id')::uuid;

-- Same shape as GetArchLock but with FOR UPDATE; used inside a write
-- transaction to atomically acquire/extend the lock.
-- name: GetArchLockForUpdate :one
SELECT project_id, author_user_id, author_token_id, locked_at,
       last_write_at, write_count, base_version
FROM arch_locks
WHERE project_id = sqlc.arg('project_id')::uuid
FOR UPDATE;

-- name: InsertArchLock :exec
INSERT INTO arch_locks (project_id, author_user_id, author_token_id,
                        locked_at, last_write_at, write_count, base_version)
VALUES (
    sqlc.arg('project_id')::uuid,
    sqlc.arg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid,
    now(), now(), 1,
    sqlc.arg('base_version')::int
);

-- name: ExtendArchLock :exec
UPDATE arch_locks
SET last_write_at = now(),
    write_count = write_count + 1,
    author_token_id = sqlc.narg('author_token_id')::uuid
WHERE project_id = sqlc.arg('project_id')::uuid;

-- name: DeleteArchLock :exec
DELETE FROM arch_locks WHERE project_id = sqlc.arg('project_id')::uuid;

-- name: MaxArchRevisionVersion :one
-- Returns the highest existing version for the project, or 0 when the
-- project has no revisions yet. Used to compute the next version on
-- both lock acquisition (base_version) and snapshot insertion.
SELECT COALESCE(MAX(version), 0)::int AS version
FROM arch_revisions
WHERE project_id = sqlc.arg('project_id')::uuid;

-- name: InsertArchRevision :one
INSERT INTO arch_revisions (project_id, version, snapshot, message,
                            author_user_id, author_token_id,
                            write_count, auto_flushed)
VALUES (
    sqlc.arg('project_id')::uuid,
    sqlc.arg('version')::int,
    sqlc.arg('snapshot')::jsonb,
    sqlc.arg('message')::text,
    sqlc.narg('author_user_id')::uuid,
    sqlc.narg('author_token_id')::uuid,
    sqlc.arg('write_count')::int,
    sqlc.arg('auto_flushed')::bool
)
RETURNING id, version, created_at;

-- name: ListArchRevisions :many
-- History view: omits the (potentially large) snapshot column. Newest
-- first. `before_version` lets the caller paginate; pass NULL to start
-- from the top.
SELECT id, version, message, author_user_id, author_token_id,
       write_count, auto_flushed, created_at
FROM arch_revisions
WHERE project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('before_version')::int IS NULL
       OR version < sqlc.narg('before_version')::int)
ORDER BY version DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetArchRevision :one
SELECT id, project_id, version, snapshot, message,
       author_user_id, author_token_id, write_count, auto_flushed, created_at
FROM arch_revisions
WHERE project_id = sqlc.arg('project_id')::uuid
  AND version = sqlc.arg('version')::int;

-- name: ListExpiredArchLocks :many
-- Returns all locks whose last write is older than the project's idle
-- threshold. The threshold is (projects.arch_lock_idle_seconds OR the
-- caller-supplied default). Used by the background ticker.
SELECT l.project_id, l.author_user_id, l.author_token_id,
       l.locked_at, l.last_write_at, l.write_count, l.base_version,
       COALESCE(p.arch_lock_idle_seconds, sqlc.arg('default_idle_seconds')::int) AS idle_seconds
FROM arch_locks l
JOIN projects p ON p.id = l.project_id
WHERE now() - l.last_write_at
      > make_interval(secs => COALESCE(p.arch_lock_idle_seconds, sqlc.arg('default_idle_seconds')::int));

-- name: GetProjectArchIdleSeconds :one
-- Returns the project's idle threshold (override or NULL).
SELECT arch_lock_idle_seconds FROM projects WHERE id = sqlc.arg('project_id')::uuid;

-- name: BuildArchSnapshot :one
-- Build the full graph snapshot for one project as a single JSONB
-- document. Used both by the migration baseline and by the runtime
-- flush path.
SELECT jsonb_build_object(
    'nodes', COALESCE((
        SELECT jsonb_agg(to_jsonb(n.*) ORDER BY n.position, n.created_at)
        FROM arch_nodes n
        WHERE n.project_id = sqlc.arg('project_id')::uuid
    ), '[]'::jsonb),
    'edges', COALESCE((
        SELECT jsonb_agg(to_jsonb(e.*) ORDER BY e.created_at)
        FROM arch_edges e
        WHERE e.project_id = sqlc.arg('project_id')::uuid
    ), '[]'::jsonb),
    'kinds', COALESCE((
        SELECT jsonb_agg(to_jsonb(k.*) ORDER BY k.key)
        FROM arch_node_kinds k
        WHERE k.project_id = sqlc.arg('project_id')::uuid
    ), '[]'::jsonb),
    'links', COALESCE((
        SELECT jsonb_agg(to_jsonb(l.*) ORDER BY l.created_at)
        FROM arch_node_links l
        WHERE l.project_id = sqlc.arg('project_id')::uuid
    ), '[]'::jsonb)
)::jsonb AS snapshot;
