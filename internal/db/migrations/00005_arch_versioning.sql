-- +goose Up
-- +goose StatementBegin
--
-- Arch versioning: per-project session-based snapshots.
--
-- arch_locks holds the single open editing session per project. A
-- write acquires (or extends) the lock for its author; other authors
-- get a 423 Locked until the lock expires (idle timeout) and a
-- background ticker flushes the resulting snapshot into
-- arch_revisions.
--
-- arch_revisions is the append-only log of project graph snapshots.
-- Each row carries the full graph state at the time the session
-- closed (nodes + edges + kinds + node-links) so the row is
-- self-contained for restore + diff. version is monotonic per project
-- starting at 1.
--
-- Per-row {created,updated}_by_{user,token}_id on arch_nodes /
-- arch_edges lets the UI answer "who last touched this node" without
-- walking the revisions log.

CREATE TABLE public.arch_revisions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      uuid NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
    version         integer NOT NULL,
    snapshot        jsonb NOT NULL,
    message         text NOT NULL DEFAULT ''::text,
    author_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    author_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    write_count     integer NOT NULL DEFAULT 0,
    auto_flushed    boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, version)
);
CREATE INDEX arch_revisions_project_version_idx
    ON public.arch_revisions (project_id, version DESC);

CREATE TABLE public.arch_locks (
    project_id      uuid PRIMARY KEY REFERENCES public.projects(id) ON DELETE CASCADE,
    author_user_id  uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    author_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    locked_at       timestamptz NOT NULL DEFAULT now(),
    last_write_at   timestamptz NOT NULL DEFAULT now(),
    write_count     integer NOT NULL DEFAULT 0,
    base_version    integer NOT NULL DEFAULT 0
);
CREATE INDEX arch_locks_last_write_at_idx
    ON public.arch_locks (last_write_at);

ALTER TABLE public.arch_nodes
    ADD COLUMN created_by_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    ADD COLUMN created_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    ADD COLUMN updated_by_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    ADD COLUMN updated_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL;

ALTER TABLE public.arch_edges
    ADD COLUMN created_by_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    ADD COLUMN created_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    ADD COLUMN updated_by_user_id  uuid REFERENCES public.users(id) ON DELETE SET NULL,
    ADD COLUMN updated_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL;

-- Per-project override for the idle threshold. NULL means "use the
-- global default" (NOTTARIO_ARCH_LOCK_IDLE_SECONDS, default 120).
ALTER TABLE public.projects
    ADD COLUMN arch_lock_idle_seconds integer;

-- Seed every existing project with an initial v1 snapshot so the
-- revisions log has a baseline. The snapshot captures the current
-- materialized state of arch_nodes, arch_edges, arch_node_kinds and
-- arch_node_links per project. The author / token are NULL because we
-- can't attribute this to anyone — it's the state we found at upgrade.
INSERT INTO public.arch_revisions (project_id, version, snapshot, message, auto_flushed)
SELECT
    p.id,
    1,
    jsonb_build_object(
        'nodes', COALESCE((
            SELECT jsonb_agg(to_jsonb(n.*) ORDER BY n.position, n.created_at)
            FROM public.arch_nodes n
            WHERE n.project_id = p.id
        ), '[]'::jsonb),
        'edges', COALESCE((
            SELECT jsonb_agg(to_jsonb(e.*) ORDER BY e.created_at)
            FROM public.arch_edges e
            WHERE e.project_id = p.id
        ), '[]'::jsonb),
        'kinds', COALESCE((
            SELECT jsonb_agg(to_jsonb(k.*) ORDER BY k.key)
            FROM public.arch_node_kinds k
            WHERE k.project_id = p.id
        ), '[]'::jsonb),
        'links', COALESCE((
            SELECT jsonb_agg(to_jsonb(l.*) ORDER BY l.created_at)
            FROM public.arch_node_links l
            WHERE l.project_id = p.id
        ), '[]'::jsonb)
    ),
    'initial state at migration',
    true
FROM public.projects p;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.projects
    DROP COLUMN IF EXISTS arch_lock_idle_seconds;

ALTER TABLE public.arch_edges
    DROP COLUMN IF EXISTS updated_by_token_id,
    DROP COLUMN IF EXISTS updated_by_user_id,
    DROP COLUMN IF EXISTS created_by_token_id,
    DROP COLUMN IF EXISTS created_by_user_id;

ALTER TABLE public.arch_nodes
    DROP COLUMN IF EXISTS updated_by_token_id,
    DROP COLUMN IF EXISTS updated_by_user_id,
    DROP COLUMN IF EXISTS created_by_token_id,
    DROP COLUMN IF EXISTS created_by_user_id;

DROP TABLE IF EXISTS public.arch_locks;
DROP TABLE IF EXISTS public.arch_revisions;
-- +goose StatementEnd
