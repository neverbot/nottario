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
