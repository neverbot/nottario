---
title: What's new
section: Updates
nav_order: 1
---

# What's new

User-visible changes shipped to `ghcr.io/neverbot/nottario:latest`,
newest first. Every push to master ships `:latest`; versioned tags
are cut on demand.

## 2026-07-05

- **Self-hosted admins now see an "update available" banner** when a
  newer commit lands on upstream master. The check runs in-process
  every 24 hours (configurable via `SELF_UPDATE_CHECK_INTERVAL`);
  disable with `SELF_UPDATE_CHECK_ENABLED=false`. See the
  [update notifications](/self-hosting/#update-notifications)
  reference.

## 2026-06-20

- **BREAKING — skill bundle install collapsed into one tool.**
  `nottario.skill.list` and `nottario.skill.read` are gone. The new
  `nottario.skill.install` returns a small JSON descriptor with a
  short-lived signed URL for the bundle as a single zip, plus an
  install plan and a `bundle_version` hash. The bundle content no
  longer flows through the MCP response — the agent fetches the URL
  out of band with any HTTP tool, unzips into
  `<workspace>/.claude/skills/nottario/` (or `~/.claude/skills/...`
  as fallback), and restarts the client. A typical first sync drops
  from ~20-30k tokens to ~200.
- **New `nottario.tasks.close` MCP tool.** Atomic close that bundles
  `link_commit` + `add_comment` + `set_state` into one transaction.
  On precondition failure the whole thing rolls back. Slim ack
  `{task, comment_id?, linked_commit_count}`; `verbose: true` for the
  full Task. Skill bundle now teaches `tasks.close` as the canonical
  close and explicitly forbids "starting work" / "claimed" pickup
  comments.
- **Arch and search MCP responses slimmed.** `arch.list_nodes` /
  `list_edges` return slim rows; `arch.get_node` opts in to
  children/edges/links via `include_*`; `upsert_*` and `move_node`
  return slim acks. `search` drops the raw description (snippet only),
  default limit 50→20. All accept `verbose: true`.
- **`docs.read { head_only: true }`.** Returns frontmatter + 400-char
  preview with `truncated` / `body_length` markers.
- **`tasks.list` defaults to open tasks only.** Pass
  `include_closed: true` or an explicit `state` to see done/wont_do.

## 2026-06-19

- **MCP responses are slim by default.** High-frequency task
  mutations return only the keys needed to chain the next call; no
  description or comment body echoed. `tasks.get` omits deps, commits
  and comments unless `include_*` is set. `verbose: true` opts back
  in. A typical "claim → comment → done" loop drops from ~5 KB to
  ~600 B. New "Token discipline" section in the skill bundle.
- **Edit task title, description and comments from the UI.** Quiet
  `Edit` button reveals a GitHub-style Write/Preview markdown editor
  (Ctrl/Cmd+Enter, Esc). Members edit text; admins edit role. Per
  comment Edit/Delete for author or admin. Edited marker; optimistic
  concurrency rejects stale writes.

## 2026-06-15

- **Skill bundle: git methodology.** New `methodology/git.md` covers
  solo / parallel-agent / multi-dev workflows.
- **Closing comment is now required** before `done` / `wont_do`.
  Bugs get a terse Repro / Fix / Test triplet; documented in
  `domains/tasks.md`.

## 2026-06-14

- **Architecture diagram is now versioned.** Edits open a per-author
  session; the idle window (default 120s) auto-flushes one
  `arch_revisions` row with the full graph snapshot.
  New MCP tool `nottario.arch.checkpoint { message? }` flushes
  immediately. New env vars `NOTTARIO_ARCH_LOCK_IDLE_SECONDS` and
  `NOTTARIO_ARCH_TICK_SECONDS`. Different-author writes during an
  active session return `423 Locked` with `retry_after_seconds`. New
  REST endpoints `/arch/history` and `/arch/revisions/{version}`.

## 2026-06-12

- **Brand-anchored palette + Gantt visual refresh.** Single CSS-token
  system rooted in the brand gradient (`#1f6feb`, `#2da44e`); docs at
  `docs/design/palette.md`. Gantt: `NOW` pill with glow column,
  past-zone wash, hairline lane separators, role-tinted bars, corner
  red dot for bugs, collapsed legend.

## 2026-06-10

- **Kanban: filter row, priority dots, column rename.** Filter chips
  for Mine / Role / Type, mirrored to the URL hash. Priority shows as
  a coloured dot. Columns renamed to **To do** / **In progress** /
  **Done**. Drag across columns shows an Undo toast (6s). In-app
  delete confirm. `feature` type is gated behind Advanced.
- **Realtime: comment events propagate live.** `task.comment.*`
  refreshes the open detail dialog without reload.
- **Search dropdown: keyboard nav, grouping, error state.** Results
  grouped by source with headers. Top hit auto-selected; ↑/↓/Enter/
  Esc work. Network failures surface a Retry row instead of a silent
  "No matches". `/` hint in the empty input. Meta lines drop the
  internal column prefixes.

## v0.1.0 — 2026-06-08

First public release, published as `ghcr.io/neverbot/nottario:0.1.0`
(`linux/amd64`).

- **Tasks.** Atomic claim (`SELECT … FOR UPDATE SKIP LOCKED`),
  dependency cycles caught with a project-scoped advisory lock,
  cascading rollup on feature parents. Cycles ("sprints") with
  rollover. Kanban and hand-rolled-SVG Gantt views.
- **Documents.** Versioned markdown store with optimistic
  concurrency. Full history per path.
- **Architecture.** Compound-layout diagram backed by ELK, rendered
  with our own SVG.
- **Identity.** GitHub OAuth for humans; per-project API tokens for
  agents. Admin tokens are not exempt from project scope.
- **MCP server.** Streamable HTTP, Bearer auth. Tools cover whoami /
  projects / tasks / docs / arch / cycles / search / skill.
- **Skill bundle.** Operating rules read on demand via MCP or
  pre-installed in Claude Code.
- **Search.** Full-text across tasks, documents and arch nodes,
  multi-language.
- **Realtime.** SSE + Postgres `LISTEN/NOTIFY`. No WebSockets.
- **Backups.** Optional in-process `pg_dump`, daily with N-day
  rotation. Off when `NOTTARIO_BACKUP_DIR` is unset.
- **Licence.** MIT.

See [Self-hosting reference](/self-hosting/) and
[Getting started](/getting-started/).
