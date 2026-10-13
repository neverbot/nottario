-- +goose Up
-- MCP usage, per day, project, token and tool: how many calls and how
-- many bytes went in (tool arguments) and out (result text). Tokens in
-- an agent's context are the running cost of Nottario, and this is how
-- a project sees which agents and tools spend them. tools/list is
-- recorded under the tool name 'tools/list': it is the catalogue an
-- agent loads into its context.
CREATE TABLE public.mcp_usage_daily (
    day        date NOT NULL,
    project_id uuid NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
    token_id   uuid NOT NULL REFERENCES public.api_tokens(id) ON DELETE CASCADE,
    tool       text NOT NULL CHECK (char_length(tool) BETWEEN 1 AND 100),
    calls      bigint NOT NULL DEFAULT 0,
    errors     bigint NOT NULL DEFAULT 0,
    bytes_in   bigint NOT NULL DEFAULT 0,
    bytes_out  bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (project_id, day, token_id, tool)
);

-- +goose Down
DROP TABLE public.mcp_usage_daily;
