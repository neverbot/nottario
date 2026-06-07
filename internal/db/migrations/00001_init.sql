-- +goose Up
-- Consolidated initial schema (Nottario v0.1.0).
--
-- This single migration replaces what used to be migrations 00001 ->
-- 00016. The earlier per-feature migration history can be recovered
-- from `git log internal/db/migrations/`.

--
--




--
-- Name: notify_event(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.notify_event() RETURNS trigger
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

    -- pg_notify accepts at most 8000 bytes for the payload; ours is far smaller.
    PERFORM pg_notify('nottario_events', payload::text);

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: tasks_enforce_cycle_cascade(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.tasks_enforce_cycle_cascade() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.parent_task_id IS NOT NULL THEN
        SELECT cycle_id INTO NEW.cycle_id
        FROM tasks WHERE id = NEW.parent_task_id;
        IF NEW.cycle_id IS NULL THEN
            RAISE EXCEPTION 'parent task % has no cycle_id', NEW.parent_task_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: touch_arch_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.touch_arch_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: touch_document_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.touch_document_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: touch_task_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.touch_task_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd




--
-- Name: api_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    token_hash bytea NOT NULL,
    prefix text NOT NULL,
    default_role_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    project_id uuid NOT NULL
);


--
-- Name: arch_edges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arch_edges (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    from_node_id uuid NOT NULL,
    to_node_id uuid NOT NULL,
    kind text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT arch_edges_check CHECK ((from_node_id <> to_node_id))
);


--
-- Name: arch_node_kinds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arch_node_kinds (
    project_id uuid NOT NULL,
    key text NOT NULL,
    label text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    color text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: arch_node_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arch_node_links (
    project_id uuid NOT NULL,
    node_id uuid NOT NULL,
    link_type text NOT NULL,
    target_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT arch_node_links_link_type_check CHECK ((link_type = ANY (ARRAY['doc'::text, 'task'::text])))
);


--
-- Name: arch_nodes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arch_nodes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    slug text NOT NULL,
    parent_id uuid,
    kind text NOT NULL,
    name text NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    linked_repo text,
    linked_path text,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (((((((setweight(to_tsvector('simple'::regconfig, COALESCE(slug, ''::text)), 'A'::"char") || setweight(to_tsvector('simple'::regconfig, COALESCE(name, ''::text)), 'A'::"char")) || setweight(to_tsvector('english'::regconfig, COALESCE(name, ''::text)), 'A'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(name, ''::text)), 'A'::"char")) || setweight(to_tsvector('simple'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char")) || setweight(to_tsvector('english'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char"))) STORED
);


--
-- Name: cycles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cycles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    name text NOT NULL,
    "position" integer NOT NULL,
    opened_at timestamp with time zone DEFAULT now() NOT NULL,
    closed_at timestamp with time zone,
    closed_by_user_id uuid,
    closed_by_token_id uuid
);


--
-- Name: document_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    document_id uuid NOT NULL,
    version integer NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    content_md text DEFAULT ''::text NOT NULL,
    frontmatter jsonb DEFAULT '{}'::jsonb NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    author_user_id uuid,
    author_token_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    scope text NOT NULL,
    project_id uuid,
    path text NOT NULL,
    kind text DEFAULT 'context'::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    content_md text DEFAULT ''::text NOT NULL,
    frontmatter jsonb DEFAULT '{}'::jsonb NOT NULL,
    current_version integer DEFAULT 1 NOT NULL,
    deleted_at timestamp with time zone,
    created_by_user_id uuid,
    created_by_token_id uuid,
    updated_by_user_id uuid,
    updated_by_token_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (((((((((setweight(to_tsvector('simple'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(title, ''::text)), 'A'::"char")) || setweight(to_tsvector('simple'::regconfig, COALESCE(description, ''::text)), 'B'::"char")) || setweight(to_tsvector('english'::regconfig, COALESCE(description, ''::text)), 'B'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(description, ''::text)), 'B'::"char")) || setweight(to_tsvector('simple'::regconfig, COALESCE(content_md, ''::text)), 'C'::"char")) || setweight(to_tsvector('english'::regconfig, COALESCE(content_md, ''::text)), 'C'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(content_md, ''::text)), 'C'::"char"))) STORED,
    CONSTRAINT documents_check CHECK (((scope = 'global'::text) = (project_id IS NULL))),
    CONSTRAINT documents_kind_check CHECK ((kind = ANY (ARRAY['skill'::text, 'context'::text, 'note'::text]))),
    CONSTRAINT documents_scope_check CHECK ((scope = ANY (ARRAY['project'::text, 'global'::text])))
);


--
-- Name: instance_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.instance_meta (
    key text NOT NULL,
    value text NOT NULL
);


--
-- Name: memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memberships (
    user_id uuid NOT NULL,
    project_id uuid NOT NULL,
    role_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: project_priorities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_priorities (
    project_id uuid NOT NULL,
    key text NOT NULL,
    value integer NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: project_repos; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_repos (
    project_id uuid NOT NULL,
    repo text NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: projects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.projects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    primary_language text,
    project_type text,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    mcp_page_size integer DEFAULT 50 NOT NULL,
    default_view text DEFAULT 'board/kanban'::text NOT NULL,
    cycle_label text DEFAULT 'sprint'::text NOT NULL,
    owner_user_id uuid NOT NULL,
    CONSTRAINT projects_default_view_check CHECK ((default_view = ANY (ARRAY['board/kanban'::text, 'board/gantt'::text, 'docs'::text, 'arch/diagram'::text, 'arch/tree'::text]))),
    CONSTRAINT projects_mcp_page_size_check CHECK (((mcp_page_size >= 1) AND (mcp_page_size <= 500)))
);


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    key text NOT NULL,
    label text NOT NULL,
    color text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    user_agent text,
    ip inet
);


--
-- Name: task_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    task_id uuid NOT NULL,
    author_user_id uuid,
    author_token_id uuid,
    body_md text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: task_commits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_commits (
    task_id uuid NOT NULL,
    repo text NOT NULL,
    sha text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    added_by_user_id uuid,
    added_by_token_id uuid
);


--
-- Name: task_dependencies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_dependencies (
    task_id uuid NOT NULL,
    depends_on_id uuid NOT NULL,
    CONSTRAINT task_dependencies_check CHECK ((task_id <> depends_on_id))
);


--
-- Name: tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    parent_task_id uuid,
    type text DEFAULT 'task'::text NOT NULL,
    title text NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    state text DEFAULT 'todo'::text NOT NULL,
    priority integer DEFAULT 50 NOT NULL,
    assignee_user_id uuid,
    target_role_id uuid,
    actual_start timestamp with time zone,
    actual_end timestamp with time zone,
    created_by_user_id uuid,
    created_by_token_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS ((((((setweight(to_tsvector('simple'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(title, ''::text)), 'A'::"char")) || setweight(to_tsvector('simple'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char")) || setweight(to_tsvector('english'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char")) || setweight(to_tsvector('spanish'::regconfig, COALESCE(description_md, ''::text)), 'B'::"char"))) STORED,
    cycle_id uuid NOT NULL,
    CONSTRAINT tasks_state_check CHECK ((state = ANY (ARRAY['todo'::text, 'doing'::text, 'done'::text]))),
    CONSTRAINT tasks_type_check CHECK ((type = ANY (ARRAY['task'::text, 'bug'::text, 'chore'::text, 'spike'::text, 'feature'::text])))
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    github_login text NOT NULL,
    github_id bigint NOT NULL,
    display_name text NOT NULL,
    avatar_url text,
    is_admin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone
);


--
-- Name: api_tokens api_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_pkey PRIMARY KEY (id);


--
-- Name: api_tokens api_tokens_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_project_name_unique UNIQUE (project_id, name);


--
-- Name: api_tokens api_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: arch_edges arch_edges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_edges
    ADD CONSTRAINT arch_edges_pkey PRIMARY KEY (id);


--
-- Name: arch_node_kinds arch_node_kinds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_node_kinds
    ADD CONSTRAINT arch_node_kinds_pkey PRIMARY KEY (project_id, key);


--
-- Name: arch_node_links arch_node_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_node_links
    ADD CONSTRAINT arch_node_links_pkey PRIMARY KEY (node_id, link_type, target_id);


--
-- Name: arch_nodes arch_nodes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_nodes
    ADD CONSTRAINT arch_nodes_pkey PRIMARY KEY (id);


--
-- Name: arch_nodes arch_nodes_project_id_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_nodes
    ADD CONSTRAINT arch_nodes_project_id_slug_key UNIQUE (project_id, slug);


--
-- Name: cycles cycles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_pkey PRIMARY KEY (id);


--
-- Name: cycles cycles_project_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_project_id_name_key UNIQUE (project_id, name);


--
-- Name: cycles cycles_project_id_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_project_id_position_key UNIQUE (project_id, "position");


--
-- Name: document_versions document_versions_document_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_document_id_version_key UNIQUE (document_id, version);


--
-- Name: document_versions document_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_pkey PRIMARY KEY (id);


--
-- Name: documents documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);


--
-- Name: documents documents_scope_project_id_path_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_scope_project_id_path_key UNIQUE (scope, project_id, path);


--
-- Name: instance_meta instance_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instance_meta
    ADD CONSTRAINT instance_meta_pkey PRIMARY KEY (key);


--
-- Name: memberships memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_pkey PRIMARY KEY (user_id, project_id, role_id);


--
-- Name: project_priorities project_priorities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_priorities
    ADD CONSTRAINT project_priorities_pkey PRIMARY KEY (project_id, key);


--
-- Name: project_repos project_repos_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_repos
    ADD CONSTRAINT project_repos_pkey PRIMARY KEY (project_id, repo);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: projects projects_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_slug_key UNIQUE (slug);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: roles roles_project_id_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_project_id_key_key UNIQUE (project_id, key);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: task_comments task_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_pkey PRIMARY KEY (id);


--
-- Name: task_commits task_commits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_commits
    ADD CONSTRAINT task_commits_pkey PRIMARY KEY (task_id, repo, sha);


--
-- Name: task_dependencies task_dependencies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_pkey PRIMARY KEY (task_id, depends_on_id);


--
-- Name: tasks tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);


--
-- Name: users users_github_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_github_id_key UNIQUE (github_id);


--
-- Name: users users_github_login_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_github_login_key UNIQUE (github_login);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: api_tokens_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX api_tokens_project_idx ON public.api_tokens USING btree (project_id);


--
-- Name: api_tokens_token_hash_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX api_tokens_token_hash_idx ON public.api_tokens USING btree (token_hash);


--
-- Name: api_tokens_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX api_tokens_user_idx ON public.api_tokens USING btree (user_id);


--
-- Name: arch_edges_from_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_edges_from_idx ON public.arch_edges USING btree (project_id, from_node_id);


--
-- Name: arch_edges_to_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_edges_to_idx ON public.arch_edges USING btree (project_id, to_node_id);


--
-- Name: arch_edges_unique_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX arch_edges_unique_idx ON public.arch_edges USING btree (project_id, from_node_id, to_node_id, kind);


--
-- Name: arch_node_links_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_node_links_target_idx ON public.arch_node_links USING btree (link_type, target_id);


--
-- Name: arch_nodes_parent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_nodes_parent_idx ON public.arch_nodes USING btree (parent_id);


--
-- Name: arch_nodes_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_nodes_project_idx ON public.arch_nodes USING btree (project_id);


--
-- Name: arch_nodes_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX arch_nodes_search_idx ON public.arch_nodes USING gin (search_vector);


--
-- Name: cycles_one_active_per_project; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX cycles_one_active_per_project ON public.cycles USING btree (project_id) WHERE (closed_at IS NULL);


--
-- Name: document_versions_doc_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_versions_doc_idx ON public.document_versions USING btree (document_id);


--
-- Name: documents_project_kind_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX documents_project_kind_idx ON public.documents USING btree (project_id, kind);


--
-- Name: documents_scope_path_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX documents_scope_path_idx ON public.documents USING btree (scope, project_id, path);


--
-- Name: documents_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX documents_search_idx ON public.documents USING gin (search_vector);


--
-- Name: memberships_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX memberships_project_idx ON public.memberships USING btree (project_id);


--
-- Name: project_priorities_value_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_priorities_value_idx ON public.project_priorities USING btree (project_id, value);


--
-- Name: sessions_expires_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_expires_at_idx ON public.sessions USING btree (expires_at);


--
-- Name: sessions_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_user_id_idx ON public.sessions USING btree (user_id);


--
-- Name: task_comments_task_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_comments_task_idx ON public.task_comments USING btree (task_id);


--
-- Name: task_dependencies_dep_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_dependencies_dep_idx ON public.task_dependencies USING btree (depends_on_id);


--
-- Name: tasks_assignee_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_assignee_idx ON public.tasks USING btree (project_id, assignee_user_id);


--
-- Name: tasks_cycle_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_cycle_id_idx ON public.tasks USING btree (cycle_id);


--
-- Name: tasks_pagination_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_pagination_idx ON public.tasks USING btree (project_id, priority DESC, created_at, id);


--
-- Name: tasks_parent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_parent_idx ON public.tasks USING btree (parent_task_id);


--
-- Name: tasks_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_project_idx ON public.tasks USING btree (project_id);


--
-- Name: tasks_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_search_idx ON public.tasks USING gin (search_vector);


--
-- Name: tasks_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_state_idx ON public.tasks USING btree (project_id, state);


--
-- Name: tasks_target_role_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_target_role_idx ON public.tasks USING btree (project_id, target_role_id);


--
-- Name: arch_edges arch_edges_notify_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_edges_notify_delete AFTER DELETE ON public.arch_edges FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.edge.deleted');


--
-- Name: arch_edges arch_edges_notify_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_edges_notify_insert AFTER INSERT ON public.arch_edges FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.edge.created');


--
-- Name: arch_edges arch_edges_notify_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_edges_notify_update AFTER UPDATE ON public.arch_edges FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.edge.updated');


--
-- Name: arch_edges arch_edges_touch_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_edges_touch_updated_at BEFORE UPDATE ON public.arch_edges FOR EACH ROW EXECUTE FUNCTION public.touch_arch_updated_at();


--
-- Name: arch_nodes arch_nodes_notify_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_nodes_notify_delete AFTER DELETE ON public.arch_nodes FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.node.deleted');


--
-- Name: arch_nodes arch_nodes_notify_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_nodes_notify_insert AFTER INSERT ON public.arch_nodes FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.node.created');


--
-- Name: arch_nodes arch_nodes_notify_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_nodes_notify_update AFTER UPDATE ON public.arch_nodes FOR EACH ROW EXECUTE FUNCTION public.notify_event('arch.node.updated');


--
-- Name: arch_nodes arch_nodes_touch_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arch_nodes_touch_updated_at BEFORE UPDATE ON public.arch_nodes FOR EACH ROW EXECUTE FUNCTION public.touch_arch_updated_at();


--
-- Name: documents documents_notify_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER documents_notify_delete AFTER DELETE ON public.documents FOR EACH ROW EXECUTE FUNCTION public.notify_event('doc.deleted');


--
-- Name: documents documents_notify_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER documents_notify_insert AFTER INSERT ON public.documents FOR EACH ROW EXECUTE FUNCTION public.notify_event('doc.created');


--
-- Name: documents documents_notify_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER documents_notify_update AFTER UPDATE ON public.documents FOR EACH ROW EXECUTE FUNCTION public.notify_event('doc.updated');


--
-- Name: documents documents_touch_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER documents_touch_updated_at BEFORE UPDATE ON public.documents FOR EACH ROW EXECUTE FUNCTION public.touch_document_updated_at();


--
-- Name: tasks tasks_cycle_cascade; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tasks_cycle_cascade BEFORE INSERT OR UPDATE OF parent_task_id, cycle_id ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.tasks_enforce_cycle_cascade();


--
-- Name: tasks tasks_notify_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tasks_notify_delete AFTER DELETE ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.notify_event('task.deleted');


--
-- Name: tasks tasks_notify_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tasks_notify_insert AFTER INSERT ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.notify_event('task.created');


--
-- Name: tasks tasks_notify_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tasks_notify_update AFTER UPDATE ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.notify_event('task.updated');


--
-- Name: tasks tasks_touch_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tasks_touch_updated_at BEFORE UPDATE ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.touch_task_updated_at();


--
-- Name: api_tokens api_tokens_default_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_default_role_id_fkey FOREIGN KEY (default_role_id) REFERENCES public.roles(id);


--
-- Name: api_tokens api_tokens_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: api_tokens api_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: arch_edges arch_edges_from_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_edges
    ADD CONSTRAINT arch_edges_from_node_id_fkey FOREIGN KEY (from_node_id) REFERENCES public.arch_nodes(id) ON DELETE CASCADE;


--
-- Name: arch_edges arch_edges_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_edges
    ADD CONSTRAINT arch_edges_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: arch_edges arch_edges_to_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_edges
    ADD CONSTRAINT arch_edges_to_node_id_fkey FOREIGN KEY (to_node_id) REFERENCES public.arch_nodes(id) ON DELETE CASCADE;


--
-- Name: arch_node_kinds arch_node_kinds_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_node_kinds
    ADD CONSTRAINT arch_node_kinds_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: arch_node_links arch_node_links_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_node_links
    ADD CONSTRAINT arch_node_links_node_id_fkey FOREIGN KEY (node_id) REFERENCES public.arch_nodes(id) ON DELETE CASCADE;


--
-- Name: arch_node_links arch_node_links_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_node_links
    ADD CONSTRAINT arch_node_links_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: arch_nodes arch_nodes_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_nodes
    ADD CONSTRAINT arch_nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.arch_nodes(id) ON DELETE CASCADE;


--
-- Name: arch_nodes arch_nodes_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arch_nodes
    ADD CONSTRAINT arch_nodes_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: cycles cycles_closed_by_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_closed_by_token_id_fkey FOREIGN KEY (closed_by_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: cycles cycles_closed_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_closed_by_user_id_fkey FOREIGN KEY (closed_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: cycles cycles_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cycles
    ADD CONSTRAINT cycles_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: document_versions document_versions_author_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_author_token_id_fkey FOREIGN KEY (author_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: document_versions document_versions_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: document_versions document_versions_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_document_id_fkey FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE;


--
-- Name: documents documents_created_by_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_created_by_token_id_fkey FOREIGN KEY (created_by_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: documents documents_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: documents documents_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: documents documents_updated_by_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_updated_by_token_id_fkey FOREIGN KEY (updated_by_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: documents documents_updated_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_updated_by_user_id_fkey FOREIGN KEY (updated_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: memberships memberships_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: memberships memberships_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: memberships memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: project_priorities project_priorities_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_priorities
    ADD CONSTRAINT project_priorities_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: project_repos project_repos_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_repos
    ADD CONSTRAINT project_repos_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: projects projects_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);


--
-- Name: projects projects_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id);


--
-- Name: roles roles_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: sessions sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: task_comments task_comments_author_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_author_token_id_fkey FOREIGN KEY (author_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: task_comments task_comments_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_comments task_comments_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: task_commits task_commits_added_by_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_commits
    ADD CONSTRAINT task_commits_added_by_token_id_fkey FOREIGN KEY (added_by_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: task_commits task_commits_added_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_commits
    ADD CONSTRAINT task_commits_added_by_user_id_fkey FOREIGN KEY (added_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_commits task_commits_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_commits
    ADD CONSTRAINT task_commits_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: task_dependencies task_dependencies_depends_on_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_depends_on_id_fkey FOREIGN KEY (depends_on_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: task_dependencies task_dependencies_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: tasks tasks_assignee_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_assignee_user_id_fkey FOREIGN KEY (assignee_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tasks tasks_created_by_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_created_by_token_id_fkey FOREIGN KEY (created_by_token_id) REFERENCES public.api_tokens(id) ON DELETE SET NULL;


--
-- Name: tasks tasks_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tasks tasks_cycle_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_cycle_id_fkey FOREIGN KEY (cycle_id) REFERENCES public.cycles(id) ON DELETE RESTRICT;


--
-- Name: tasks tasks_parent_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_parent_task_id_fkey FOREIGN KEY (parent_task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: tasks tasks_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: tasks tasks_target_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_target_role_id_fkey FOREIGN KEY (target_role_id) REFERENCES public.roles(id) ON DELETE SET NULL;


--
--

-- Seed instance metadata (preserved from the original 00001_init.sql).
INSERT INTO public.instance_meta (key, value)
VALUES ('schema_initialised_at', NOW()::text)
ON CONFLICT (key) DO NOTHING;


-- +goose Down
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
