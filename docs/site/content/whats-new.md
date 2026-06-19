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

## 2026-06-20

- **More MCP responses slimmed down.** `arch.list_nodes` and
  `arch.list_edges` now return a slim shape per row (no description,
  metadata, linked_repo, linked_path, from_name/to_name) and accept
  `verbose: true` to opt back into the full object. `arch.get_node`
  no longer eagerly packs children, edges and links into every
  response — pass `include_children`, `include_edges` or
  `include_links` opt-in flags for the slices you need. The arch
  mutations (`upsert_node`, `upsert_edge`, `move_node`, `upsert_kind`)
  return a slim ack by default with `verbose: true` for the full
  object. `nottario.search` drops the raw `description` fallback from
  each hit (the highlighted `description_html` snippet is what
  matters), lowers the default limit from 50 to 20 (max 100), and
  accepts `verbose: true` to keep the raw description. `docs.read`
  gains `head_only: true` returning frontmatter plus a 400-char
  preview (with `truncated` and `body_length` markers) for catalogue
  flows that just need to confirm the document's identity. The skill
  bundle documents the new defaults in `domains/architecture.md`
  ("Token discipline") and `domains/docs.md`.
- **`nottario.tasks.list` defaults to open tasks only.** Without an
  explicit `state` filter the MCP tool now returns just `todo` and
  `doing` rows. Closed tasks (`done`, `wont_do`) accumulate forever
  and would dominate every backlog walk; pass `include_closed: true`
  to opt into the previous behaviour, or set `state='done'` /
  `state='wont_do'` to scope to a closed bucket. The skill bundle
  documents the rule in `domains/tasks.md` ("Token discipline" and
  the `tasks.list` reference).

## 2026-06-19

- **MCP responses are now slim by default.** The high-frequency task
  tools (`tasks.create`, `tasks.update`, `tasks.set_state`,
  `tasks.claim`, `tasks.claim_next`, `tasks.next`, `tasks.add_comment`)
  and `tasks.list` return only the fields needed to chain the next
  call — `id`, `title`, `state`, `priority`, `updated_at`,
  role/assignee — and no longer echo back the description or comment
  body. `tasks.get` returns the base task in full but **omits**
  `depends_on`, `commits` and `comments` unless the caller passes
  `include_deps`, `include_commits` or `include_comments`. Pass
  `verbose: true` on the mutations to opt back into the full Task
  shape. This change cut a typical "claim → comment → done" loop
  from ~5 KB of MCP traffic to ~600 B; sessions that hammer Nottario
  via Claude Code should see a drop in token bill. The skill bundle
  ships a new **"Token discipline"** section in `domains/tasks.md`
  with the full rules.
- **Edit task title, description and comments from the UI.** The
  task detail dialog now exposes a quiet `Edit` button on the title
  and the description (revealed on hover or keyboard focus). The
  description opens a GitHub-style `Write` / `Preview` markdown
  editor (Ctrl/Cmd+Enter to save, Esc to cancel). Project members
  can edit title and description; admins can also change the role
  inline. Every comment grows an `Edit` / `Delete` action visible
  only to its author or an admin; deletion is confirmed in place
  without a modal. Edited fields show a quiet `(edited Xh ago by
  @user)` marker; optimistic concurrency rejects stale writes with a
  toast that preserves the in-progress draft.

## 2026-06-15

- **Skill bundle: git methodology for agents.** New file
  `methodology/git.md` teaches agents how to drive git across the
  three project shapes — solo agent, parallel agents under one human
  (worktree per task), multi-dev with PRs — plus the rules that hold
  across all three (agents never push by default, no force-push, no
  amend on pushed commits, Conventional Commits single-line). Linked
  from `skill.md` §1 and from `.claude/claude.md`.
- **Closing comment is now required.** `domains/tasks.md` documents
  the convention agents follow before flipping a task to `done` or
  `wont_do`: a short paragraph for ordinary tasks, a terse
  Repro / Fix / Test triplet for bugs, and a one-line "why" for
  `wont_do`. The diff is truth; the comment is the story future
  readers need to understand the diff without reverse-engineering
  it.

## 2026-06-14

- **Architecture diagram now has versioning.** Every change to nodes
  / edges / kinds / links opens an editing session per project and
  per author; when the author stops writing for the idle window
  (default 120s, override per project, env var
  `NOTTARIO_ARCH_LOCK_IDLE_SECONDS`), the session is auto-flushed
  into a single `arch_revisions` row with the full graph snapshot.
  Two new env vars: `NOTTARIO_ARCH_LOCK_IDLE_SECONDS` and
  `NOTTARIO_ARCH_TICK_SECONDS` (background ticker interval, default
  30s). New MCP tool `nottario.arch.checkpoint { message? }` lets
  agents close their session immediately with a commit-style
  message — recommended at the end of a coherent block of edits.
  Different-author writes during an active session return `423
  Locked` with `retry_after_seconds`. New REST endpoints `GET
  /api/projects/{id}/arch/history` and `GET
  /api/projects/{id}/arch/revisions/{version}` expose the log.

## 2026-06-12

- **Brand-anchored palette + Gantt visual refresh.** The whole web
  UI now reads its colour from a single CSS-token system rooted in
  the brand gradient (`#1f6feb` blue, `#2da44e` green); the new
  documentation lives in `docs/design/palette.md`. The Gantt picked
  up a confident `NOW` pill with a translucent glow column, a subtle
  past-zone wash, hairline separators between lanes (no more zebra
  stripes), role-tinted doing/todo bars instead of saturated fills,
  a corner red dot for bugs (so several bugs no longer drown the
  timeline in red), and a collapsed legend at the foot.

## 2026-06-10

- **Kanban: filter row, priority dots and column rename.** Three
  filter chips above the columns let you scope the board to your
  own work, one or more roles, and one or more task types; the
  selection is mirrored to the URL hash so deep-links work. Cards
  now encode priority as a small coloured dot (red / amber / grey)
  beside the bucket name instead of a monospace `p3` token, so the
  urgency is the first thing the eye reads. Columns renamed from
  the internal `todo` / `doing` / `done` enums to **To do**, **In
  progress**, **Done**. Drag-drop across columns now shows a
  toast with **Undo** for 6 seconds. The browser-native delete
  confirm dialog was replaced with an in-app one. The `feature`
  type stays hidden behind an Advanced checkbox in the new-task
  dialog so it isn't picked by accident.
- **Realtime: comments now propagate live.** Adding, editing or
  deleting a task comment fires a `task.comment.*` event so the open
  task-detail dialog refreshes its comment thread without a manual
  reload. The kanban and Gantt views keep refreshing on every
  `task.*` event as before.
- **Search dropdown: keyboard navigation, grouping and honest error
  state.** Results are now grouped by source (Tasks, Documents,
  Architecture) with section headers. The top hit is auto-selected;
  Up / Down move the highlight, Enter opens, Escape closes. Network
  failures surface as a visible "Search failed." row with a Retry
  button instead of silently rendering "No matches." A small `/`
  hint inside the empty input advertises the global shortcut. Meta
  lines dropped the internal column prefixes (`state:`, `type:`,
  `path:`, `slug:`) so a row reads `done · task` instead of
  `state: done · type: task`.

## v0.1.0 — 2026-06-08

First public release, published as `ghcr.io/neverbot/nottario:0.1.0`
(`linux/amd64`).

- **Tasks.** Atomic claim (`tasks.claim_next` / `tasks.claim`) with
  `SELECT … FOR UPDATE SKIP LOCKED`, dependency cycles caught with a
  project-scoped advisory lock, cascading "done" rollup on feature
  parents, bug-recovery reconciler. Cycles ("sprints") with rollover
  of in-flight work. Kanban and hand-rolled-SVG Gantt views (see
  [Kanban](/kanban/) and [Gantt](/gantt/) for tours).
- **Documents.** Versioned markdown store with optimistic concurrency
  (`expected_version` on every write). Full history per path. See
  [Docs](/docs/).
- **Architecture.** Compound-layout diagram backed by ELK, rendered
  with our own SVG. Editable from the web UI; agent-friendly textual
  representation. See [Architecture](/architecture/).
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
