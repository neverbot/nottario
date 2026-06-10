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
- `nottario.skill.list`, `nottario.skill.read` — the on-demand
  skill bundle (see below).

`nottario.tasks.next` exists as a read-only preview but does not
claim — prefer `claim_next` to avoid races.

## Skills: on-demand vs pre-loaded

Nottario ships a skill bundle aimed at agents (operating rules,
task discipline, sqlc conventions). It is available two ways:

1. **On-demand via MCP.** Once the agent is wired up, it can call
   `nottario.skill.list` to see what's available and
   `nottario.skill.read` to pull a specific file. No local install
   needed.
2. **Pre-loaded into Claude Code.** Drop `internal/skill/files/`
   into `~/.claude/skills/` (or run the installer in the repo).
   This makes the skill visible at session start without any tool
   call.

The two paths are equivalent in content; pick on-demand if you want
the bundle to update with the server, pre-loaded if you want it
available without the MCP being reachable.

## Authentication

Every MCP request must carry a Bearer token. Tokens are issued
per-project from the Settings tab and stored hashed. Revoking from
the UI invalidates immediately.
