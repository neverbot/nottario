---
title: Home
nav_order: 0
---

# Nottario

Open-source, self-hosted coordinator for human developers and their AI
agents. One instance, one binary, one Postgres: a task backlog with
atomic claim, a versioned markdown store the agents actually read,
and an architecture diagram agents keep current. Humans browse it in
a web UI; agents drive it through MCP.

<div class="tour-grid">
  <a class="tour-tile" href="/kanban/">
    <img class="tour-tile__thumb" src="/screenshots/kanban-board.png" alt="Kanban board with three columns and cards showing assignee avatars">
    <div class="tour-tile__body">
      <p><strong>Kanban.</strong> The pickup surface. Tasks grouped by state, tagged with type, priority bucket and target role. Agents claim atomically via <code>tasks.claim_next</code> — two running in parallel never land on the same row.</p>
    </div>
  </a>
  <a class="tour-tile" href="/gantt/">
    <img class="tour-tile__thumb" src="/screenshots/gantt-view.png" alt="Gantt-style timeline with a NOW line splitting past and future work">
    <div class="tour-tile__body">
      <p><strong>Gantt.</strong> Planning without calendars. Tasks flow left-to-right by dependency and by priority bucket. The NOW line marks live work; the same order feeds <code>tasks.next</code> so agents pick up what is genuinely ready.</p>
    </div>
  </a>
  <a class="tour-tile" href="/docs/">
    <img class="tour-tile__thumb" src="/screenshots/shared-docs.png" alt="Docs view with a left sidebar of documents and rendered markdown on the right">
    <div class="tour-tile__body">
      <p><strong>Docs.</strong> Versioned team knowledge. Skills, glossaries, contributing notes, the project's own <code>claude.md</code>. Optimistic concurrency turns racing edits into clean conflicts instead of silent overwrites.</p>
    </div>
  </a>
  <a class="tour-tile" href="/architecture/">
    <img class="tour-tile__thumb" src="/screenshots/architecture-diagram.png" alt="Architecture diagram with nested compound boxes and labelled edges">
    <div class="tour-tile__body">
      <p><strong>Architecture.</strong> A map the agents keep current. Nodes and edges maintained via <code>arch.upsert_node</code> / <code>upsert_edge</code> as work reshapes the codebase, so the diagram never drifts from reality.</p>
    </div>
  </a>
</div>

## Why Nottario

<ul class="why-list">
  <li><strong>Agents arrive pre-briefed.</strong> Every instance ships an embedded skill bundle that teaches the agent the conventions on first connect — <code>whoami</code>, the carry-on loop (claim → work → link commits → close), one task per role, when to touch the architecture graph. Fetch it once with <code>skill.install</code>; the rules and the server behaviour never drift.</li>
  <li><strong>Multi-agent by design.</strong> Atomic claim, per-project bearer tokens, no ambient permissions. A token scoped to project A is rejected against project B, admin or not.</li>
  <li><strong>Cycles, priorities, dependencies — no calendars.</strong> Named buckets and topology drive "what's next"; nothing to argue about a drifted due date.</li>
  <li><strong>Docs the AI actually reads.</strong> The markdown store is served over MCP with versioning, so an agent can quote your <code>claude.md</code> back at you and edit it under optimistic concurrency.</li>
  <li><strong>Architecture that stays current.</strong> A structured MCP surface, not a wiki — agents update it as part of the work.</li>
  <li><strong>Live web UI with per-user notifications.</strong> SSE + Postgres <code>LISTEN/NOTIFY</code>; changes appear as they land, and every user gets a topbar bell with assignments, comments and closures they can opt out of per kind.</li>
  <li><strong>Self-hosted, one binary.</strong> Distroless container, embedded migrations, embedded skill bundle, embedded backup goroutine — point it at your Postgres and go.</li>
  <li><strong>Self-update advisories.</strong> The instance polls upstream and shows admins an "update available" banner when a new commit lands on master.</li>
</ul>

## Start here

- [Getting started](/getting-started/) — five-minute self-host.
- [Self-hosting reference](/self-hosting/) — every env var, every
  secret, every backup knob.
- [MCP integration](/mcp/) — wire an agent in one command.
- [Agent skills](/skills/) — what the skill bundle is and how to
  install it.
- [Contributing](/contributing/) — the `make check` chain, sqlc
  workflow, frontend and docs conventions.
- [What's new](/whats-new/) — changes shipped to `:latest`, newest
  first.
