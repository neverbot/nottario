# Nottario — Project context for Claude Code

This file collects the invariants and decisions that every working
session must keep in mind from the start. It is deliberately short;
detail lives in `docs/initial/` during the analysis phase and, later
on, in the project's own context markdown documents managed by
Nottario itself.

## Language policy

**All written artefacts in this project are in English.** That
includes:

- Source code, comments, variable and function names.
- Documentation under `docs/`, `readme.md`, `changelog.md`, etc.
- Commit messages.
- The default content of any markdown document seeded by the
  application (skill bundle, default project templates, error
  messages shown to users).
- Issue/PR titles and bodies.

Conversation with the user can happen in any language they choose; the
artefacts that get written to disk must be in English regardless.

## What Nottario is

An open source, self-hosted service that coordinates human developers
and their AI agents. Three functional domains:

1. **Tasks** — prioritised backlog with dependencies, no dates. Gantt
   and Kanban board. Agents query "what's next", file bugs, mark
   progress, link commits.
2. **Markdown context** — shared repository of skills, documentation
   and notes. Replaces the loose `.md` files scattered across each
   developer's laptop.
3. **Architecture** — navigable diagram of expandable boxes with
   arrows that describes the product, maintained by agents in textual
   form.

Everything is accessible via an MCP server (for agents) and a web
interface (for humans; architecture is read-only for them).

## Technical invariants

- **Lightweight is a first-order goal.** The reference bar is
  [owl](../../owl): Go backend with embedded assets, single binary,
  small Docker image, vanilla frontend. Every dependency must justify
  its presence.
- **Backend:** Go (owl pattern: `cmd/nottario` + `internal/...`,
  embed.go).
- **Frontend:** vanilla CSS + Lit (~5KB, ES modules, no build step).
  No React/Vue/Svelte/Angular. Charts hand-rolled following the style
  of `internal/design/chart.js` in owl.
- **Documented exception:** the architectural graph uses **dagre** as
  layout engine and renders to SVG. It is the only visualisation
  library allowed.
- **Database:** Postgres always. SQLite is off the table.
- **Real-time:** SSE (Server-Sent Events) + Postgres `LISTEN/NOTIFY`.
  No WebSockets.
- **Deployment:** Docker, `compose.yml` with nottario + postgres.
- **Human identity:** GitHub OAuth from v1.
- **Agent identity:** API tokens generated from the web UI.
- **MCP:** served over HTTP+SSE from the same binary. No separate MCP
  binary.

## Design invariants

- **Aesthetic: GitHub-like.** Neutral palette (greys + a blue/green
  accent), system sans-serif typography, medium density, thin
  borders, flat colour badges, simple dropdowns. If a visual decision
  doesn't sit naturally next to GitHub, pause and reconsider.
- **Hand-drawn (Rough.js) is parked for now.** May be revisited once
  the architectural graph renderer works — not before.
- **No decorative animation.** Things appear where they belong without
  dances or transitions added for show. Owl sets the bar.

## Operational rules for agents

- Markdown filenames are always lowercase (`readme.md`, `claude.md`,
  etc.), even when tools traditionally use uppercase. Use a symlink
  if a tool strictly requires an uppercase name.
- Commit messages: a single line, Conventional Commits. No body, no
  trailers, no `Co-Authored-By`.
- Do not touch `git config`, do not push, do not use `--no-verify`,
  do not amend without explicit request.
- The main branch is named `master` when the repo gets initialised.
- Any externally-visible action (push, comment on PR/issue, send
  messages, upload content to third-party tools) requires explicit
  confirmation from the human before execution.

## Project status

Analysis phase. No code yet. Documents from this phase live in
`docs/initial/` (not versioned; excluded by `.gitignore`). When the
analysis is signed off, a formal implementation plan is written and
implementation begins.
