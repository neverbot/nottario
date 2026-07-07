# Nottario

**Full documentation: [neverbot.github.io/nottario](https://neverbot.github.io/nottario/).**

Open-source, self-hosted coordinator for human developers and their AI
agents. One instance brings three things under a single source of
truth: a **task backlog** with cycles, named priority buckets,
dependencies and atomic claim semantics; a **versioned markdown
store** for skills, specs and team notes; and an **architecture
diagram** of services and their connections that agents maintain
themselves. Humans browse it in a web UI; agents drive it through MCP.

Per-project bearer tokens make multi-agent work safe by construction:
a token scoped to project A is rejected the moment it touches project
B, admin or not, and `tasks.claim_next` hands a task to exactly one
caller even when two agents race for it. Drop the MCP into Claude
Code or Cursor and the agent can list, claim, deliver, link commits
and update the architecture diagram without humans relaying state by
hand.

The whole server is a single Go binary in one container, talking to
whatever Postgres you already run. The web UI updates live as agents
and humans work — no refresh — and the binary takes its own daily
backups so that's not a separate piece of infrastructure to remember.

## A tour, in four screens

![Kanban board with three columns (todo, doing, done), tasks tagged by type, priority bucket and role; cards in doing and done show the assignee's avatar in the bottom-right corner; sprint progress in the header](assets/screenshots/kanban-board.png)

*Kanban — the pickup surface.* Tasks grouped by state, tagged with
type, named priority bucket and target role. Cards that have an
assignee show the owner's avatar in the bottom-right corner — at a
glance you see who's holding what. Humans grab a card with a click;
agents take the same row atomically via `nottario.tasks.claim_next`,
so two of them running in parallel never land on the same task. The
sprint header summarises throughput without leaving the view.

![Gantt-style timeline with PAST and FUTURE zones around a NOW line, tasks ordered by topological dependency and priority bucket, a hovered card showing the full task title](assets/screenshots/gantt-view.png)

*Gantt — the planning surface.* No calendar dates: Nottario sorts
tasks by **dependencies** (left-to-right is what blocks what) and by
**priority bucket** inside each layer. The NOW line marks live work,
dashed boxes are pending, solid grey is done. Hovering a pill
reveals the full title in place. Helps humans see the critical path;
helps agents (and `tasks.next`) reason about what is genuinely ready
to pick up versus what is still waiting on a precondition.

![Docs view with a left sidebar listing context documents (claude.md, glossary, contributing, skill files) and the rendered markdown of claude.md on the right](assets/screenshots/shared-docs.png)

*Shared markdown context — versioned project knowledge.* Everything
the team writes that isn't code lives here: skills, glossary,
contributing notes, post-mortems, the project's own `claude.md`.
Optimistic concurrency (`expected_version` on every write) means two
agents editing the same doc in parallel get a clean conflict instead
of a silent overwrite. Agents read these via `nottario.docs.read`
to learn the project's conventions before they touch code; humans
browse and edit them in the web UI.

![Architecture diagram with nested boxes (System → Service → Module → Component) connected by labelled edges, a right-rail panel showing the highlighted node's description, repo, incoming and outgoing edges](assets/screenshots/architecture-diagram.png)

*Architecture — a living map of the codebase.* Nodes are services,
modules and components; edges are calls, data flow and dependencies.
Hand-rolled SVG over an ELK-computed layout, with a detail rail that
surfaces description, linked repo path, and incident edges per
selected node. **Agents are the cartographers**: every time work
adds, removes or rewires a component, the same agent updates the
diagram via `nottario.arch.upsert_node`/`upsert_edge`, so the map
never goes stale against the code. Humans browse it read-only as a
living-system overview.

## Why you might want this

- **Agents arrive pre-briefed.** Every instance ships an embedded
  skill bundle: `whoami`, the carry-on loop (claim → work → link
  commits → close), one task per role, when to touch the
  architecture graph. Fetch it once with `skill.install`; the rules
  never drift from the server behaviour.
- **Multi-agent by design.** Atomic claim, per-project bearer
  tokens, no ambient permissions. A token scoped to project A is
  rejected against project B, admin or not — even instance admins
  don't bypass the boundary.
- **Cycles, priorities, dependencies — no calendars.** Named
  priority buckets and dependency topology drive "what's next"
  automatically. Move a task, everything downstream reflows. Nothing
  to argue about a drifted due date.
- **Docs the AI actually reads.** A versioned markdown store served
  over MCP — glossaries, post-mortems, on-call runbooks, design
  briefs. Agents can quote a page back at you, edit it under
  optimistic concurrency, and search across everything the team has
  written.
- **Architecture that stays current.** The diagram is a structured
  MCP surface (`arch.upsert_node`, `arch.upsert_edge`), not a wiki —
  agents rewire it as part of the work, so the map never drifts from
  the code.
- **Live web UI — see every agent working.** SSE + Postgres
  `LISTEN/NOTIFY` push every state change to every open browser as
  it lands: cards flip to doing, commits link, comments appear. The
  topbar bell surfaces assignments and closures per user, opt-out
  per kind.
- **Self-hosted, one binary.** Distroless container, embedded
  migrations, embedded skill bundle, embedded backup goroutine
  (`pg_dump` on a schedule). Point it at your Postgres and go.
- **Self-update advisories.** The instance polls upstream once a
  day and shows admins a banner when a new commit lands on master.
  `SELF_UPDATE_CHECK_ENABLED=false` turns it off for air-gapped
  hosts.

## Run it locally

```bash
git clone https://github.com/neverbot/nottario.git
cd nottario
cp .env.example .env       # fill in GITHUB_OAUTH_CLIENT_ID/SECRET and SESSION_KEY
docker compose up --build  # http://localhost:8080
```

Full walkthrough (env vars, GitHub OAuth App setup, first-admin
promotion): [Getting started](https://neverbot.github.io/nottario/getting-started/).

For production (reverse proxy, secrets on disk, external Postgres,
backups, upgrades): [Self-hosting](https://neverbot.github.io/nottario/self-hosting/).

## Wire an AI agent

Nottario exposes its full surface to MCP-capable clients (Claude
Code, Claude Desktop, Cursor). One token per project, no ambient
scope. From a project's web UI: **Settings → Tokens → New token**,
then in the repo the agent will work on:

```bash
claude mcp add nottario http://localhost:8080/mcp \
  --transport http \
  --header "Authorization: Bearer ntr_…" \
  --scope local
```

Client-specific setup (Claude Desktop, Cursor), token semantics,
multi-project workflow and the on-first-connect skill bundle:
[MCP setup](https://neverbot.github.io/nottario/mcp/) and
[Skills](https://neverbot.github.io/nottario/skills/).

## Development

Tests run on the host with the Go toolchain against a Postgres you
already have up (the compose file's `db` service is fine):

```bash
TEST_DATABASE_URL='postgres://nottario:nottario@localhost:5432/postgres?sslmode=disable' \
  make check
```

`make check` chains gofmt, `go vet`, `golangci-lint` (with a custom
`sqlcheck` analyzer that flags unsafe SQL concatenation), `sqlc diff`
so generated code stays in sync with the queries, a docs-site smoke
build, Node's `--check` parse over the frontend, Biome, and the full
Go test suite.

After editing Go or frontend assets:

```bash
docker compose up -d --build nottario
```

## Tech stack

Backend (Go):

- [pgx/v5](https://github.com/jackc/pgx) — Postgres driver.
- [sqlc](https://sqlc.dev) — type-safe Go generated from SQL.
- [goose](https://github.com/pressly/goose) — embedded schema migrations.
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — MCP server transport.
- [goldmark](https://github.com/yuin/goldmark) — CommonMark renderer for docs and task descriptions.
- [bluemonday](https://github.com/microcosm-cc/bluemonday) — HTML sanitiser that runs after goldmark.
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2) — GitHub OAuth flow.
- [google/uuid](https://github.com/google/uuid) — UUID generation and parsing.
- [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML frontmatter on skill bundle and shared docs.
- [joho/godotenv](https://github.com/joho/godotenv) — `.env` loader for local dev.
- [golang.org/x/tools](https://pkg.go.dev/golang.org/x/tools) — analyzer framework for the custom SQL-injection lint.
- [`time/tzdata`](https://pkg.go.dev/time/tzdata) — embedded IANA zoneinfo so `TZ=…` resolves on the minimal alpine runtime image.

Frontend (vanilla, no build step):

- [Lit](https://lit.dev) — web-components framework for the UI.
- [elkjs](https://github.com/kieler/elkjs) — vendored layout engine that computes positions for the architecture diagram (we render the SVG ourselves).
- [highlight.js](https://highlightjs.org) — vendored syntax highlighting inside rendered markdown.

Infrastructure:

- [Postgres](https://www.postgresql.org/) — primary datastore. `pg_dump` and `pg_restore` from `postgresql17-client` ship inside the runtime image, used by the in-process backup goroutine and by `scripts/restore.sh`.
- [Docker](https://www.docker.com/) / [Docker Compose](https://docs.docker.com/compose/) — local and deploy runtime; the runtime image is `alpine:3.21` + the binary.

Tooling:

- [gofmt](https://pkg.go.dev/cmd/gofmt), [go vet](https://pkg.go.dev/cmd/vet), [golangci-lint](https://golangci-lint.run) — Go lint stack chained by `make check`.
- `internal/tools/sqlcheck` — in-tree golangci-lint analyzer that flags any `fmt.Sprintf` or string concatenation of runtime values feeding `pgx`'s `Query`/`Exec`/`QueryRow`.
- [Biome](https://biomejs.dev) — JavaScript format + lint gate for `internal/web/static/`. Runs via `npx --yes @biomejs/biome` so there is no `node_modules` in the repo; the Rust binary is cached under `~/.npm/_npx/` after first use.

## License

MIT — see [`license.md`](license.md).
