# claude.md

Project context and operating rules for Claude Code (and any other AI
agent working on Nottario). Short and load-bearing; every working
session keeps these invariants in mind from the start. Deeper detail
lives in the project's own Nottario documents (`context` kind) and in
the skill bundle (`internal/skill/files/`).

## What Nottario is

Open-source, self-hosted service that coordinates human developers
and their AI agents. Three functional domains, all surfaced via MCP
(for agents) and a web UI (for humans; architecture is read-only for
humans):

1. **Tasks** — prioritised backlog with dependencies, no calendar
   dates. Kanban and Gantt views. Agents query "what's next", claim
   tasks, file bugs, link commits.
2. **Markdown context** — shared repository of skills, docs and
   notes. Replaces loose `.md` files scattered across laptops.
   Documents are versioned with optimistic concurrency
   (`expected_version` on every write).
3. **Architecture** — navigable diagram of expandable boxes with
   arrows describing the product, maintained by agents in textual
   form.

## Language policy

**All written artefacts are in English.** Source, comments, identifier
names, `docs/`, `readme.md`, `changelog.md`, commit messages, seeded
markdown, issue/PR titles and bodies, default UI strings.

Conversation with the user can happen in any language they choose;
artefacts written to disk must be in English regardless.

## Local vs production — which Nottario am I hitting?

This project has **two live Nottario instances**, and a working
session almost always touches both. Mistaking one for the other has
already caused confusion (canary state drift, "why did the value
revert?", etc.); keep the distinction front-of-mind.

- **Local dev container** — `http://localhost:8080`. Brought up by
  `docker compose up -d --build nottario`. Has its own Postgres
  volume (`nottario_db-data`). Used by:
  - The Chrome-devtools MCP browser (`mcp__chrome-devtools__*`).
  - Any `curl http://localhost:8080/...` from the shell.
  - Manual smoke tests after a code change — this is the only
    instance we can rebuild on every commit.
- **Production Nottario** — `http://10.99.1.1:8081` (currently;
  `claude mcp list` reports the exact URL). This is the canonical
  task / docs / arch store and runs `ghcr.io/neverbot/nottario:latest`,
  which CI publishes on every push to master. Used by:
  - The MCP tools (`mcp__nottario__nottario_tasks_*`,
    `mcp__nottario__nottario_docs_*`, `mcp__nottario__nottario_arch_*`,
    `mcp__nottario__nottario_search`, etc.).

Concrete corollaries an agent must keep in mind:

1. **A `set_state` via MCP changes production**; a button click in
   the Chrome browser changes the dev container. They are NOT the
   same database. A task can be `wont_do` locally and `todo` in
   production at the same time, and that's not a bug — that's two
   different instances.
2. **Backend changes need to ship before MCP reflects them.** After
   a backend feature lands, the local container is up-to-date on
   the next `docker compose up -d --build`, but the production MCP
   still runs the previous `:latest` until CI publishes the new
   image and the host pulls it. During that window, the MCP can
   refuse newly-introduced values (e.g. a new enum) even though
   the local binary accepts them. This is a deploy-lag issue, not
   a code bug; either wait for the pull, or verify locally via
   `curl` / Chrome until the upgrade lands.
3. **Smoke a new feature on BOTH** when feasible: local container
   for "does the code work", production MCP for "does the canonical
   row reflect the change". For a behaviour-only change with no new
   wire shape (a typo fix, a tooltip, a CSS rewrite), local is
   enough.
4. **Stop assuming the canary state from memory.** Every time you
   reference "the task is in state X", re-fetch it via MCP — that
   IS production, and that IS the truth.

## Technical invariants

- **Lightweight is a first-order goal.** Single Go binary with
  embedded assets, small Docker image, vanilla frontend. Every
  dependency justifies its presence.
- **Backend:** Go (`cmd/nottario` + `internal/...`, embed.go).
  pgx/v5 against Postgres 16.
- **Frontend:** vanilla CSS + Lit (~5KB, ES modules, no build step).
  No React/Vue/Svelte/Angular. Charts hand-rolled SVG.
- **No JS visualisation libraries.** Every chart in the app is
  hand-rolled SVG: Gantt (`pages/gantt.js`) and the architecture
  diagram (`components/arch-canvas.js`, currently powered by the
  vendored `elkjs` for compound layout but rendered with our own SVG).
- **Database:** Postgres always. SQLite is off the table.
  The backend is fully on **sqlc**; all queries live in
  `internal/db/queries/*.sql` and the generated Go is committed at
  `internal/db/dbq/`. Outside sqlc, pgx with `$N` placeholders only;
  never `fmt.Sprintf` or concatenation of runtime values into
  `Query`/`Exec`/`QueryRow` (CI blocks it via the custom
  `internal/tools/sqlcheck` analyzer).
- **Postgres client in the container.** The Dockerfile installs
  `postgresql<N>-client` so the in-process backup goroutine can
  shell out to `pg_dump`. `pg_dump` refuses to dump a server newer
  than itself, but works fine against older servers, so the bundled
  client must always match the **newest** Postgres major a
  self-hoster might run against. Bump `postgresql<N>-client` in the
  Dockerfile whenever a new Postgres major ships and alpine has the
  package; do not pin to the dev compose's version.
- **Concurrency model (multi-agent safe):**
  - `nottario.tasks.claim_next` / `nottario.tasks.claim` are atomic
    (`SELECT … FOR UPDATE SKIP LOCKED` and per-row locks). Never use
    the legacy three-step `next + update + set_state` pattern.
  - `SetState(done)` runs inside a transaction with `FOR UPDATE` on
    the task row + precondition check. `AddDependency` takes a
    project-scoped `pg_advisory_xact_lock` so concurrent edits across
    multiple tasks can't form cycles.
  - `rollUpParentDone` runs inside the SetState transaction; a
    background reconciler catches anything that drifted.
- **Real-time:** SSE + Postgres `LISTEN/NOTIFY`. No WebSockets.
- **Deployment:** Docker. `compose.yml` brings up `nottario` +
  Postgres. Local smoke tests always hit the container at
  `http://localhost:8080`; never run `./bin/nottario` directly.
  After any change to the binary or `internal/web/static/`, run
  `docker compose up -d --build nottario` so the user can verify
  immediately.
- **Human identity:** GitHub OAuth from v1.
- **Agent identity:** API tokens generated per-project from the web
  UI (open the project → Settings → Tokens). One token = one project;
  an agent using a token issued for project A cannot read or write
  anything in project B. Admin tokens are not exempt from project
  scope.
- **MCP:** served over HTTP+SSE from the same binary. No separate
  MCP binary.

## Design invariants

- **Aesthetic: GitHub-like.** Neutral palette (greys + blue/green
  accent), system sans-serif typography, medium density, thin
  borders, flat colour badges, simple dropdowns. If a visual choice
  doesn't sit naturally next to GitHub, pause and reconsider.
- **Hand-drawn (Rough.js) is parked.** Revisit only after the
  architectural graph renderer is solid.
- **No decorative animation.** Things appear where they belong.
  Motion is acceptable only when it's *functional* (showing what just
  changed, scrolling somewhere). Respect `prefers-reduced-motion`.
- **Shadow DOM ignores global box-sizing.** Every Lit component's
  shadow styles need an explicit `box-sizing: border-box`. Global
  rules in `/static/styles.css` do not penetrate.

## Operational rules

### Throwaway artefacts (screenshots, dumps, scratch files)
- The repo has a dedicated `/.scratch/` directory (already in
  `.gitignore`). **Every** throwaway file an agent generates during a
  session — Chrome-devtools screenshots, ad-hoc debug dumps, JSON
  snapshots, temporary CSV exports — goes there. Never in `.claude/`
  (read-only context for the agent), never at the repo root, never
  in `docs/` (which IS versioned).
- Naming: prefix with the page or topic so the directory stays
  scannable across sessions (`gantt-after.png`, `whoami-probe.json`,
  `arch-tree-empty.png`). Avoid generic `screenshot.png`.
- `tmp-*.png` at the repo root is also gitignored as a fallback when
  a tool can't write to `/.scratch/`, but `.scratch/` is preferred.
- The directory is local-only and is never cleaned automatically;
  feel free to leave artefacts around within a session and to
  re-read them later in the same conversation.

### Public changelog (`whats-new.md`)
- Any **important** task — a new feature, a backwards-incompatible
  change, a security or data-handling change, a new env var or
  config knob, a default-behaviour change, a removal — must add an
  entry to `docs/site/content/whats-new.md` before the closing
  commit. Small bug fixes, internal refactors, dependency bumps and
  docs typos do not need an entry; they live in `git log`.
- Headings are **dates**, not versions: `## YYYY-MM-DD`, newest
  first. Every push to master ships to `:latest` immediately, so
  there is no "Unreleased" stage — the moment your commit lands,
  it is released. When a `vX.Y.Z` tag is cut on top of a day's
  bullets, fold the version into the heading as
  `## vX.Y.Z — YYYY-MM-DD`; otherwise leave the heading as just the
  date.
- If today already has a heading, append under it; otherwise create
  a new `## YYYY-MM-DD` block at the top.
- Each bullet is one sentence in the past tense, user-facing
  (mention the knob name, the config default, the UI change), with
  a link to a deeper page when relevant. Avoid implementation
  details — those belong in commit messages, not the changelog.
- The CI gate (`make docs-check`) does not enforce this yet; the
  discipline is on the agent closing the task. The site is rebuilt
  on every push to master, so a missed entry is visible publicly
  within minutes.

### File naming
- Markdown filenames are always lowercase (`readme.md`, `claude.md`,
  etc.), even when tools traditionally use uppercase. Use a symlink
  if a tool strictly requires uppercase.

### Git
- Commit messages: single line, Conventional Commits. No body, no
  trailers, no `Co-Authored-By`.
- Don't touch `git config`. Don't push without explicit request.
- Don't use `--no-verify`, `reset --hard`, `clean -f`, `branch -D`,
  amend, or any rewrite-history op unless explicitly asked.
- The primary branch is `master`.
- Prefer `git add <specific>` over `git add -A` to avoid staging
  secrets or unrelated changes by accident.

### Externally visible actions
- Push, PR/issue comment, sending messages, uploading to third-party
  services: requires explicit human confirmation. One-time approval
  doesn't extend to future calls.

### Task discipline (Nottario itself)
- **Every task has a target_role.** One of backend / frontend /
  design / qa. Never null on non-feature tasks.
- **Multi-role work → one task per role**, linked with
  `add_dependency` in execution order. Optionally grouped under a
  `type=feature` parent (which rolls up to done automatically when
  all children are done).
- **Skill-bundle edits go under `target_role = backend`**, not
  frontend. They're operational instructions for agents using the
  MCP server, not UI work.
- **Self-assign before doing.** When picking up a task, set
  `assignee_user_id = your whoami.user_id` BEFORE calling
  `set_state doing`. Otherwise the task sits doing with no owner.
- **Use the MCP for task CRUD/state transitions**, not direct SQL.
  SQL is for read-only inspection or bug-recovery only.
- **Link commits before closing a task.** Whenever the work landed
  produced one or more commits, call
  `nottario.tasks.link_commit { repo, sha }` for each commit BEFORE
  `set_state done` (or right after, but never skip it). The closing
  comment can reference the commits inline for human readers, but the
  structured link is what powers the UI's Commits panel, the
  "what shipped in this task" queries and any future traceability
  audit. The bar is: a future agent reading a closed task should be
  able to jump straight to the diff without grep. Bug-recovery /
  documentation-only tasks legitimately have no commit; everything
  else does.
- **Atomic pickup:** call `nottario.tasks.claim_next` (no filter or
  with role/assignee) or `nottario.tasks.claim` (specific id). The
  legacy three-call pattern (`next` + `update` + `set_state`) is
  racy and disabled by convention — `nottario.tasks.next` is now a
  read-only preview.
- **Default priority is bucket `medium` per project.** Prefer
  `priority_key` over raw integers; the buckets live in
  `nottario.projects.list_priorities`.
- **File new work BEFORE doing it.** Two shapes of "new work" both
  go through `nottario.tasks.create` *before* you write any code,
  open any editor, or run any command:
  1. **Side-channel requests / bugs spotted in passing.** "Ah, y
     también deberíamos…", "this is broken: …", "we noticed X".
     File the row, decide whether to pivot or stay on the current
     task. Verbatim quotes from the user (the bug repro, the
     half-formed idea) belong in the description — future-you will
     not remember them.
  2. **Substantive new work the user explicitly asks you to do.**
     "Let's add Biome", "do the design review of the Kanban",
     "rename `content_md` to `content`". Even when the user is
     telling you to *act*, the act starts with `tasks.create` →
     `claim` → work. Skipping the row because "the request is
     obviously the task" leaves the backlog blind: the work has no
     handle for tracking, no audit trail, no link to the resulting
     commits. The exception is conversational tweaks that fit in a
     single small commit and need no follow-up (a typo fix in a
     doc, a one-line CSS adjustment) — those can land directly.
  Both shapes need the right `target_role`, an honest description,
  dependencies linked if relevant, and a split into role children
  when multi-role. The bar is "if I had to leave the session right
  now, would someone else be able to pick this up?" — if not, file
  more context.

### SQL conventions (sqlc workflow)
- **All new queries land in `internal/db/queries/<domain>.sql`.** One
  file per logical domain (`tasks.sql`, `docs.sql`, `arch.sql`,
  `search.sql`, …). Each query starts with a `-- name: <Name> :<kind>`
  comment where `<kind>` is `one`, `many`, `exec`, `execrows`.
- **Regenerate with `make sqlc`.** Commit the generated
  `internal/db/dbq/*.sql.go` alongside the `.sql` changes — CI runs
  `sqlc diff` (`make sqlc-check`) and a drift will fail the gate.
- **Named args over positional** when a query has more than ~3
  parameters or any optional input. Use `sqlc.arg('name')::<type>`
  for required, `sqlc.narg('name')::<type>` for optional/nullable;
  always cast so Postgres can infer the type. Mixing positional `$N`
  with `sqlc.arg(...)` in the same query is fragile — pick one style
  per query.
- **Optional filters** are expressed as
  `(sqlc.narg('foo')::text IS NULL OR col = sqlc.narg('foo')::text)`
  rather than building SQL dynamically in Go.
- **UUID overrides** (in `sqlc.yaml`) map `uuid` → `uuid.UUID` and
  nullable `uuid` → `*uuid.UUID`. Don't reach for `pgtype.UUID`.
- **Nullable text/timestamps** surface as `pgtype.Text` /
  `pgtype.Timestamptz`. Convert at the boundary with the
  `textPtr` / `timestampPtr` / `pgtypeText` helpers each package
  keeps next to its repo code; do not leak pgtype into domain types.
- **JSONB** comes back as `[]byte`; decode with `json.Unmarshal`
  inside the package's row-mapping helper, never in the consumer.
- **Transactions:** `dbq.New(tx)` binds queries to an open `pgx.Tx`;
  use the same Queries handle throughout a transaction. Always
  `defer func() { _ = tx.Rollback(ctx) }()` immediately after `Begin`.
- **No raw pgx fallback.** If something feels hard to express in
  sqlc, add a named query — don't drop back to inline SQL. The
  `sqlcheck` analyzer will flag any new inline `Query`/`Exec` with
  runtime-formatted SQL.
- **Locks and CTEs:** advisory locks
  (`pg_advisory_xact_lock`), `FOR UPDATE [SKIP LOCKED]` and
  `WITH RECURSIVE` cycle checks all live in sqlc queries today —
  see `dependencies.sql`, `tasks.sql`, `arch.sql` for the patterns.

### Document sync (local files ↔ Nottario)
- The canonical store for shared docs is Nottario (`context` kind).
  Some docs (notably this `claude.md`) also live in the repo. Before
  committing a local change to a doc that also lives in Nottario:
  1. `nottario.docs.read` the matching path; stash `current_version`.
  2. Compare with what you have locally. If Nottario is ahead, merge.
  3. Commit locally, then `nottario.docs.write` with
     `expected_version = current_version`. On `version_conflict`,
     re-read + merge + retry.
- This `claude.md` lives at path `context/claude.md` inside the
  Nottario project (the project_id is the call argument, not part of
  the path); keep it in sync.

## Live databases are sacred — NEVER wipe them

Any Postgres database holding the user's working state (projects,
tasks, comments, docs, arch nodes, API tokens, memberships) is
off-limits to destructive operations. This applies to the local dev
container (`nottario-db-1`, volume `nottario_db-data`) and to any
self-hosted instance the user points the MCP at. **Under no
circumstances may an agent execute commands that drop, wipe, or
recreate that data.** Specifically banned:

- `docker compose down -v` (the `-v` removes named volumes).
- `docker volume rm <volume-name>` against any nottario data volume.
- `DROP DATABASE nottario` against any live instance.
- `TRUNCATE` / `DELETE` against tables in a live DB without an
  explicit, in-this-session user request that names the rows.
- "Reset DB to verify a migration" workflows that touch any live
  container.

If you need a clean DB to verify a migration, **use one of these
options instead**:

1. The Go test helper `internal/testutil.NewPool(t)` creates a
   fresh, uniquely-named database (via `CREATE DATABASE`) inside
   the same Postgres instance, runs migrations, and drops it on
   `t.Cleanup`. Never touches the live `nottario` DB.
2. A separate ephemeral compose project for migration smoke
   (different `-p` name) so a different volume gets created.
3. A scratch postgres container started by hand with no volume
   mount.

If you cannot proceed without resetting the user's DB, **stop and
ask the user first.** Recovery from a wipe means restoring from an
external `pg_dump` backup, which is not configured by default —
losing the live state is effectively unrecoverable.

## Pre-commit gate

Before **every** `git commit`, run:

```
make check
```

That target chains, in order:

1. `gofmt -l .` must list no files (fix with `gofmt -w .`).
2. `go vet ./...` must pass clean.
3. `make lint` — `golangci-lint` (hygiene: gosec, govet,
   staticcheck, ineffassign, unused, errcheck, gofmt) **plus** a
   custom `internal/tools/sqlcheck` analyzer that flags inline
   `fmt.Sprintf` or string concatenation of runtime values passed
   to pgx `Query`/`Exec`/`QueryRow`.
4. `make sqlc-check` — `sqlc diff` confirms the committed generated
   code in `internal/db/dbq` matches the queries in
   `internal/db/queries`.
5. `go test ./...` must pass — every package, including the
   concurrency and integration tests as they land.

If any of these fail, fix the underlying issue. Never bypass with
`--no-verify` or by skipping packages. Linters and tools are pinned
in the Makefile; `make tools` installs them.

Frontend assets (`internal/web/static/**`) are vanilla JS without a
build step. The gate covers them in two layers:

1. `make js-check` runs `node --check` (parse-only) over every `.js`.
   Catches structural breakage like unbalanced brackets or template
   literals — fast, no deps, no false positives. A `package.json` at
   the static root declares `"type": "module"` so Node parses ESM.
2. `make frontend-check` runs Biome (lint + format check) via
   `npx --yes @biomejs/biome` against `internal/web/static/`. Config
   lives in `biome.json` at the repo root. The Rust binary caches
   under `~/.npm/_npx/`, so no `node_modules` in the repo and no
   global install on dev machines. `make frontend-format` rewrites
   files in place.

Together the two layers catch a class of bug we've already hit twice:
**backticks inside comments within a `` css`…` `` or `` html`…` ``
tagged template literal terminate the template early and silently
break the stylesheet/markup**. Avoid backticks in comments inside
those blocks; the gate now enforces it.

## Project status

Approaching v0.1.0 beta. Released under MIT (`license.md`). The
foundation is in place — identity (per-project tokens), tasks
(cycles, priorities, dependencies, atomic claim), docs (versioned,
optimistic concurrency), architecture (ELK-backed canvas), gantt,
kanban, MCP server, skill bundle, search, full-text multi-language.
CI publishes `ghcr.io/neverbot/nottario:latest` on every push to
master.

The remaining work for v0.1.0 lives as tasks inside Nottario itself
(see `nottario.tasks.list { state: 'todo' }`); the headline items
are automated backups, UI design reviews and a few polish bugs.
