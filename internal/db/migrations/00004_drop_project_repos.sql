-- +goose Up
-- Drop the project_repos table. The "list of repositories a project
-- covers" was a UX hint that no other part of the model used —
-- `link_commit` records the repo per commit on `task_commits` and
-- never consulted this table. Removing it simplifies the project
-- shape and shortens the Settings / project-card surfaces.
DROP TABLE IF EXISTS project_repos;

-- +goose Down
-- Recreates the table empty; existing data dropped by the Up
-- migration is NOT recoverable here. Run a pg_dump backup first if
-- you need the rows back.
CREATE TABLE public.project_repos (
    project_id uuid NOT NULL,
    repo text NOT NULL
);
ALTER TABLE ONLY public.project_repos
    ADD CONSTRAINT project_repos_pkey PRIMARY KEY (project_id, repo);
ALTER TABLE ONLY public.project_repos
    ADD CONSTRAINT project_repos_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES public.projects(id) ON DELETE CASCADE;
