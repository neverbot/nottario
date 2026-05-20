# claude.md

Project context and operating rules for Claude Code (and any other AI
agent working on Nottario). Short and load-bearing; every working
session keeps these invariants in mind from the start. Deeper detail
lives in the project's own Nottario documents (`context` kind), in the
skill bundle (`internal/skill/files/`), and historically in
`docs/initial/` (pre-implementation phase, may be partially outdated).

## What Nottario is

Open-source, self-hosted service that coordinates human developers
and their AI agents. Three functional domains, all surfaced via MCP
(for agents) and a web UI (for humans; architecture is read-only for
humans):

1. **Tasks** — prioritised backlog with dependencies, no calendar
   dates. Kanban and Gantt views. Agents query "what's next", claim
   tasks, file bugs, link commits.
2. **Markdown context** — shared repository of skills, docs and
   notes. Replaces loose `.md` files scattered across laptops. Doc
   versioning with optimistic concurrency is on the roadmap.
3. **Architecture** — navigable diagram of expandable boxes with
   arrows describing the product, maintained by agents in textual
   form.

## Language policy

**All written artefacts are in English.** Source, comments, identifier
names, `docs/`, `readme.md`, `changelog.md`, commit messages, seeded
markdown, issue/PR titles and bodies, default UI strings.

Conversation with the user can happen in any language they choose;
artefacts written to disk must be in English regardless.

## Technical invariants

- **Lightweight is a first-order goal.** Reference: [owl](../../owl).
  Single Go binary with embedded assets, small Docker image, vanilla
  frontend. Every dependency justifies its presence.
- **Backend:** Go (owl pattern — `cmd/nottario` + `internal/...`,
  embed.go). pgx/v5 against Postgres 16.
- **Frontend:** vanilla CSS + Lit (~5KB, ES modules, no build step).
  No React/Vue/Svelte/Angular. Charts hand-rolled (style cue:
  `internal/design/chart.js` in owl).
- **Documented exception:** the architectural graph uses **dagre**
  as layout engine and renders to SVG. It is the only visualisation
  library allowed.
- **Database:** Postgres always. SQLite is off the table.
  The backend is fully on **sqlc**; all queries live in
  `internal/db/queries/*.sql` and the generated Go is committed at
  `internal/db/dbq/`. Outside sqlc, pgx with `$N` placeholders only;
  never `fmt.Sprintf` or concatenation of runtime values into
  `Query`/`Exec`/`QueryRow` (CI blocks it via the custom
  `internal/tools/sqlcheck` analyzer).
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
- **Agent identity:** API tokens generated from the web UI.
- **MCP:** served over HTTP+SSE from the same binary. No separate
  MCP binary.

## Design invariants

- **Aesthetic: GitHub-like.** Neutral palette (greys + blue/green
  accent), system sans-serif typography, medium density, thin
  borders, flat colour badges, simple dropdowns. If a visual choice
  doesn't sit naturally next to GitHub, pause and reconsider.
- **Hand-drawn (Rough.js) is parked.** Revisit only after the
  architectural graph renderer is solid.
- **No decorative animation.** Things appear where they belong. Owl
  sets the bar. Motion is acceptable only when it's *functional*
  (showing what just changed, scrolling somewhere). Respect
  `prefers-reduced-motion`.
- **Shadow DOM ignores global box-sizing.** Every Lit component's
  shadow styles need an explicit `box-sizing: border-box`. Global
  rules in `/static/styles.css` do not penetrate.

## Operational rules

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
- **Atomic pickup:** call `nottario.tasks.claim_next` (no filter or
  with role/assignee) or `nottario.tasks.claim` (specific id). The
  legacy three-call pattern (`next` + `update` + `set_state`) is
  racy and disabled by convention — `nottario.tasks.next` is now a
  read-only preview.
- **Default priority is bucket `medium` per project.** Prefer
  `priority_key` over raw integers; the buckets live in
  `nottario.projects.list_priorities`.

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
- This `claude.md` lives at `projects/<id>/context/claude.md` in
  Nottario; keep it in sync.

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
build step. If a Lit/JS linter is added in the future, treat it the
same as `go vet`.

## Project status

Pre-alpha but dogfooding actively. Foundation, identity, tasks,
docs, architecture, gantt, kanban, MCP server, skill bundle, search
all in place. Currently working on:

- SQL safety (Tier 1 lint guard landed; Tier 2 sqlc migration
  complete across tasks, identity, docs, arch and search).
- Documents optimistic concurrency + `claude.md` sync flow.
- Concurrency primitives for multi-agent safety
  (`claim_next`/`claim` atomic; `SetState` transactional;
  `AddDependency` project-scoped lock; rollUp reconciler).
- Backend test battery (testcontainers, real coverage target).
- Gantt polish: feature folding, Features lane, role-coloured
  dots, jump-to-now, arrow rendering across all fold states.

License decision: MIT when M10 lands.
