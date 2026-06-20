---
title: MCP integration
section: Reference
nav_order: 2
---

# MCP integration

Nottario speaks MCP over streamable HTTP from the same binary that
serves the web UI. One container, two interfaces.

## Wire an agent in one command

From your project's **Settings → Tokens** tab in the web UI, issue
an API token. The page shows a copy-paste line that looks like:

```
claude mcp add nottario --transport http \
  --header "Authorization: Bearer <your-token>" \
  -- https://your-host/mcp
```

That installs the MCP server into your local Claude Code config,
scoped to one project (the project the token was issued under).
An agent using that token can only read or write data in that
project — admin tokens are not exempt.

## Tools

The MCP server exposes one tool per natural verb. Highlights:

- `nottario.tasks.list` — list tasks, with filters by state, type,
  assignee, role, parent, cycle.
- `nottario.tasks.claim_next` — atomic "pick the next eligible
  task and assign it to me". Use this instead of the legacy
  three-step pattern.
- `nottario.tasks.claim` — atomic claim by id.
- `nottario.tasks.set_state` — transitions with precondition checks.
- `nottario.tasks.add_comment`, `nottario.tasks.link_commit` — keep
  the audit trail honest.
- `nottario.docs.read`, `nottario.docs.write` — versioned markdown
  documents with optimistic concurrency via `expected_version`.
- `nottario.arch.upsert_node`, `nottario.arch.upsert_edge` —
  maintain the architecture graph.
- `nottario.search` — full-text across tasks, docs and arch nodes.
- `nottario.skill.install` — returns a signed URL + install plan for
  the skill bundle zip (see below).

`nottario.tasks.next` exists as a read-only preview but does not
claim — prefer `claim_next` to avoid races.

## Response shape

The tools return **slim** payloads by default. A mutation like
`tasks.create`, `tasks.set_state` or `tasks.add_comment` returns only
the fields an agent needs to chain the next call — `id`, `title`,
`state`, `priority`, `updated_at`, role/assignee — and does **not**
echo back the description or comment body the agent just sent.
`tasks.list` rows follow the same rule.

When the full object is genuinely needed (e.g. an agent reading a
task it didn't create), pass `verbose: true` on the call to opt
back into every field.

`tasks.list` defaults to **open tasks only** (`state='todo'` or
`'doing'`). Closed rows accumulate forever and would dominate every
walk in a long-lived project; pass `include_closed: true` when you
genuinely need them, or set an explicit `state` filter (e.g.
`state='done'`) to scope to a closed bucket.

`tasks.get` returns the base task in full (descriptions are the
reason you called `get` in the first place) but **omits** the
related collections unless you ask for them:

```
nottario.tasks.get {
  project_id, task_id,
  include_deps: true,
  include_commits: true,
  include_comments: true,
}
```

Each flag defaults to false. The motivation is plain: comment lists
and per-row descriptions are the largest payloads in the API, and an
agent that doesn't need them shouldn't pay tokens for them. A typical
"claim → comment → done" loop now sits in ~600 B of MCP traffic
instead of ~5 KB.

## Skills: zip-and-install

Nottario ships a skill bundle aimed at agents (operating rules, task
discipline, sqlc conventions). Installation is a single tool call:

`nottario.skill.install` returns a small JSON descriptor with a
short-lived signed URL for the bundle zip, the install plan, and a
`bundle_version` hash for cache short-circuiting. The bundle content
**does not flow through the response** — the agent fetches the URL
out of band and unzips into the local skill directory using whatever
HTTP and unzip tools its host exposes. Prefer
`<repo>/.claude/skills/nottario/` (workspace-scoped, committed
alongside the code); fall back to `~/.claude/skills/nottario/` when
you work across unrelated checkouts.

Restart the host application (Claude Code or whichever agent client
you use) after a sync — most clients load the bundle once at session
start. See [Agent skills](/skills/) for the response shape
and copy-pasteable fetch/extract commands per platform.

## Authentication

Every MCP request must carry a Bearer token. Tokens are issued
per-project from the Settings tab and stored hashed. Revoking from
the UI invalidates immediately.
