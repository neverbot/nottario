---
title: What's new
section: Updates
nav_order: 1
---

# What's new

User-visible changes shipped to `ghcr.io/neverbot/nottario:latest`,
newest first. Versioned tags (`vX.Y.Z`) are cut on demand; the
rolling `:latest` tag is updated on every push to master. Anything
that changes default behaviour, adds a config knob, or removes a
feature shows up here.

## 2026-06-10

- **Realtime: comments now propagate live.** Adding, editing or
  deleting a task comment fires a `task.comment.*` event so the open
  task-detail dialog refreshes its comment thread without a manual
  reload. The kanban and Gantt views keep refreshing on every
  `task.*` event as before.

## v0.1.0 — 2026-06-08

First public release, published as `ghcr.io/neverbot/nottario:0.1.0`
(`linux/amd64`).

- **Tasks.** Atomic claim (`tasks.claim_next` / `tasks.claim`) with
  `SELECT … FOR UPDATE SKIP LOCKED`, dependency cycles caught with a
  project-scoped advisory lock, cascading "done" rollup on feature
  parents, bug-recovery reconciler. Cycles ("sprints") with rollover
  of in-flight work. Kanban and hand-rolled-SVG Gantt views.

  ![Kanban view](/screenshots/kanban-board.png)
  ![Gantt view](/screenshots/gantt-view.png)

- **Documents.** Versioned markdown store with optimistic concurrency
  (`expected_version` on every write). Full history per path.

  ![Shared docs view](/screenshots/shared-docs.png)

- **Architecture.** Compound-layout diagram backed by ELK, rendered
  with our own SVG. Editable from the web UI; agent-friendly textual
  representation.

  ![Architecture diagram](/screenshots/architecture-diagram.png)
- **Identity.** GitHub OAuth for humans; per-project API tokens for
  agents. Admin tokens are not exempt from project scope.
- **MCP server.** Streamable HTTP transport with Bearer-token auth.
  Tools cover whoami / projects / tasks / docs / arch / cycles /
  search / skill.
- **Skill bundle.** Operating rules and conventions agents read on
  demand via the MCP, or pre-installed into Claude Code.
- **Search.** Full-text across tasks, documents and arch nodes.
  Multi-language stemming.
- **Realtime.** SSE + Postgres `LISTEN/NOTIFY`. No WebSockets.
- **Backups.** Optional in-process `pg_dump` goroutine, daily with
  N-day rotation. Disabled when `NOTTARIO_BACKUP_DIR` is unset.
- **Licence.** MIT.

See [Self-hosting reference](/self-hosting/) for the full list of env
vars and the [Getting started](/getting-started/) guide for a
5-minute walkthrough.
