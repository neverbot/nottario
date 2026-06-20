---
name: nottario-domain-architecture
description: Reference for the Nottario architecture domain — how to declare and maintain the diagram of services, modules, components and their relationships in a project.
---

# Architecture domain — full reference

The architecture domain is Nottario's textual model of how a project
is organised: which services exist, which modules live inside them,
how they call or depend on each other. Humans browse it read-only;
**agents are the authors**. The skill below assumes you are an agent.

In v1 there is no automatic code analysis. You build and maintain
the diagram by hand based on what you see in the codebase. A future
milestone will add a `scan_repo` verb that suggests nodes from
tree-sitter parsing, but for now: every node and every edge is
something you decided was worth recording.

## When to touch the architecture

Add or update the architecture when:

- You introduce a new service, module or component the team didn't
  have before. (e.g. spinning up a new microservice, extracting a
  module, integrating a new external system).
- You add a new cross-boundary call — one service starts talking to
  another, a module starts reading from a new database, etc.
- The human asks "what does the architecture look like?" and you
  realise it is stale.

Do **not** touch it on every commit. The diagram is a coarse-grained
view; it goes stale gracefully, and noise from over-detailing makes
it less useful.

### After substantial work — close the architecture loop

Whenever you finish a task that added/removed/substantially modified
a software component, or changed how components relate to each
other, run a closing pass:

1. `nottario.arch.list_nodes { root_only: true }` — eyeball the
   top-level set. Is something missing that exists in the code now?
   Something present that you just deleted?
2. For each container that touched the new work, `arch.get_node` and
   confirm its children and incident edges still describe reality.
3. Apply the deltas via `upsert_node` / `upsert_edge` / `remove_*`.

This is one of the three closing-the-loop checks in `skill.md` §4 —
the other two are "is there already a task for this delivery?" and
"did the human mention side-work I should file?". Skipping the arch
pass is how the diagram becomes a lie that the next agent has to
re-derive from the code.

## Entities

### Node

A box in the diagram. Identified by a project-scoped `slug`
(snake/dotted-case, matching `[a-z0-9][a-z0-9._-]*`). The slug is
what you use everywhere else — markdown cross-domain links, edges,
parents.

| Field          | Meaning                                                                 |
|----------------|-------------------------------------------------------------------------|
| `slug`         | required, human-readable identifier (e.g. `backend.auth`).             |
| `parent_slug`  | optional; the parent in the tree. Empty for a root.                     |
| `kind`         | one of the project's catalogued kinds (see below).                      |
| `name`         | display name.                                                           |
| `description`  | markdown — short paragraph the human will read in the panel.            |
| `metadata`     | free-form `{}`; common keys: `lang`, `framework`, `port`, `runtime`.    |
| `linked_repo`  | `'owner/repo'` if the node maps to one GitHub repo.                     |
| `linked_path`  | path inside the repo (e.g. `internal/auth`).                            |
| `position`     | int, sibling ordering for visual stability.                             |

### Kind

A node's "type" label, with an icon and a colour. Every project starts
with five defaults seeded automatically the first time you touch the
architecture:

- **`system`** — the root container (the whole product).
- **`service`** — a microservice, binary or long-running process.
- **`module`** — a logical grouping inside a service.
- **`component`** — a concrete piece (controller, service, model).
- **`external`** — an external system (GitHub, Postgres, …).

Custom kinds (`worker`, `queue`, `cli`, `database`, …) can be added
with `arch.upsert_kind`. **Prefer reusing a default** — only add a
new kind when none of the existing ones fit.

### Edge

A directed arrow between two nodes (possibly at different levels of
the tree). Keyed by `(from_slug, to_slug, kind)`.

Common edge kinds:

- `depends_on` — generic dependency.
- `uses` — uses a library or subsystem.
- `calls` — synchronous call (HTTP, RPC, function).
- `reads` / `writes` — data flow.
- `publishes` / `subscribes` — pub/sub.

Self-loops are rejected. Re-issuing the same `(from, to, kind)`
updates the label/description in place.

### Link

A side-attachment from a node to a document path or a task uuid.
Used for "here is the ADR explaining this service" or "here are the
tasks in flight on this module".

## Tools

### `nottario.arch.list_kinds`

Returns the project's kind catalogue. Call this first when you start
working on architecture to know what kinds you can use.

### `nottario.arch.upsert_kind` / `remove_kind`

Add or remove custom kinds. The defaults can be removed only if no
node uses them.

### `nottario.arch.list_nodes`

Lists nodes. With no filter, returns every node. Useful filters:

- `root_only=true` — only top-level nodes (typical first call).
- `parent_slug="backend"` — direct children of one node.

Rows are **slim** by default: `{id, slug, parent_id, kind, name, position, updated_at}`. The description, metadata, linked_repo and linked_path are omitted so a tree walk stays small. Call `arch.get_node` for the full view, or pass `verbose: true` on the list call when you really need the description echoed per row.

### `nottario.arch.get_node`

Returns the base node (full shape, with description) and, opt-in,
its **direct children**, **incident edges** (in and out) and attached
docs/tasks:

```text
nottario.arch.get_node {
  project_id, slug,
  include_children: true,
  include_edges: true,
  include_links: true,
}
```

Each `include_*` flag defaults to `false` — children and edges in
particular can be a non-trivial slice of the graph; only ask for what
your next decision needs. Children and edges come back in the same
slim shape as `list_nodes` / `list_edges`.

### `nottario.arch.upsert_node`

Create or update a node:

```text
nottario.arch.upsert_node {
  project_id: "...",
  slug: "backend.auth",
  parent_slug: "backend",
  kind: "module",
  name: "Auth",
  description: "OAuth, sessions, API tokens.",
  metadata: { "lang": "go" },
  linked_repo: "neverbot/nottario",
  linked_path: "internal/identity"
}
```

The first time you touch architecture in a project, the default kind
catalogue is seeded automatically.

Mutations return a **slim ack** by default ({id, slug, kind, name,
updated_at} for nodes; {id, kind, label, updated_at} for edges;
{key, project_id, label} for kinds) — the description you sent is not
echoed back. Pass `verbose: true` when you really need the full
object back in your context.

**Send `name` and `description` as plain UTF-8.** Do not HTML-encode
ampersands, angle brackets or quotes — `Pages & Router`, not
`Pages &amp; Router`. The web UI escapes these for display, so an
encoded payload renders to the user as the literal `&amp;`. The
server now decodes a small set of common entities defensively when
it receives them, but the cleanest fix is to never encode in the
first place.

### `nottario.arch.move_node`

Reparent a node. Cycles are rejected.

### `nottario.arch.remove_node`

Delete a node. If it has children, pass `cascade=true`; otherwise
the call fails.

### `nottario.arch.upsert_edge` / `remove_edge` / `list_edges`

Create, delete and inspect edges. `list_edges` accepts `node_slug` +
`direction` ("in", "out", or "" for both) and `kind` for filtering.

`list_edges` rows are **slim** by default: `{id, from_slug, to_slug, kind, label, updated_at}` — `description`, `from_name`, `to_name` and `created_at` are omitted. Pass `verbose: true` for the full `EdgeView`.

### `nottario.arch.link_doc` / `unlink_doc` / `link_task` / `unlink_task`

Attach a markdown document path or a task uuid to a node. The web UI
shows these in the node panel and the agent can use them to navigate
context quickly.

### `nottario.arch.checkpoint`

Closes your current editing session into a single versioned snapshot
of the diagram with the message you provide — like a git commit. Call
this at the end of a coherent block of changes:

```text
nottario.arch.checkpoint {
  project_id: "...",
  message: "added auth subsystem and its incident edges"
}
```

The response carries the new `version`, `created_at` and `write_count`
of the snapshot. From that point forward the next write opens a brand
new session.

## Versioning model (the lock + auto-flush)

Every change you make to the diagram (`upsert_node`, `upsert_edge`,
`remove_*`, `move_node`, `upsert_kind`, `link_*`, `unlink_*`) happens
**inside an open editing session** scoped to the project + the human
behind your token:

- Your first write opens a session and acquires a per-project lock for
  your `user_id`. The diagram's materialised state (`arch_nodes`,
  `arch_edges`, …) is updated in place.
- Subsequent writes from you (same `user_id`) extend the session — no
  new snapshot is created. **You can do 1 or 200 writes; the resulting
  revision is one row.**
- When you stop writing for the configured idle window (default 120s,
  per-project override), a background ticker closes your session into
  a single `arch_revisions` row with `auto_flushed=true` and no
  message.
- **`arch.checkpoint` closes your session immediately with a message**
  — preferred over waiting for the auto-flush, because the human
  reading the history will see your description instead of a blank.

If you try to write while a **different** user owns the session,
Nottario refuses your call with an error carrying `retry_after_seconds`:
the time until the foreign session expires from idle. **Do not retry in
a tight loop.** Either wait, or work on something else (tasks, docs)
until the window passes. The same human's parallel agents share one
session (lock is per `user_id`, not per token), so you never collide
with your own siblings.

### Practical workflow

```text
1. upsert_node { slug: "auth.routes", kind: "module", ... }
2. upsert_node { slug: "auth.handlers", kind: "component", parent_slug: "auth.routes", ... }
3. upsert_edge { from_slug: "auth.handlers", to_slug: "auth.routes", kind: "calls" }
4. upsert_edge { from_slug: "auth.handlers", to_slug: "postgres", kind: "reads" }
5. checkpoint { message: "added auth handlers + their data-flow" }
```

Five tool calls → one revision in the history with your message.

### When NOT to checkpoint mid-burst

Don't checkpoint between every pair of writes. The whole point is that
**a coherent block becomes one revision.** Checkpoint when you have
finished a logical unit, not after each individual mutation. A good
heuristic: checkpoint at the end of the closing-the-loop pass that
you'd otherwise do silently.

### Reading the history

The REST surface for humans (and for an agent that wants to inspect
what changed) lives at:

- `GET /api/projects/{id}/arch/history?limit=N` — list of revisions,
  newest first, without the snapshot payload.
- `GET /api/projects/{id}/arch/revisions/{version}` — one full
  snapshot.

## Idiomatic patterns

### "Bootstrap an architecture from a single repo"

When you first start working on a project that has nothing recorded:

1. `arch.list_kinds` — confirm the defaults are there.
2. Add the top-level system:
   ```text
   upsert_node { slug: "system", kind: "system", name: "<Product>" }
   ```
3. Add each top-level service / external dependency under it:
   ```text
   upsert_node { slug: "backend", parent_slug: "system", kind: "service", name: "Backend API", linked_repo: "...", linked_path: "cmd/..." }
   upsert_node { slug: "postgres", kind: "external", name: "Postgres" }
   ```
4. Declare the obvious edges:
   ```text
   upsert_edge { from_slug: "backend", to_slug: "postgres", kind: "reads", label: "main data" }
   ```
5. **Stop**. Don't try to model every controller and helper. Add
   modules and components incrementally as you actually work on
   them.

### "I'm working on a new module"

While implementing something non-trivial, add the module to the
architecture as part of the work, not after:

```text
upsert_node {
  slug: "backend.auth",
  parent_slug: "backend",
  kind: "module",
  name: "Auth",
  description: "GitHub OAuth flow, sessions, API tokens."
}
```

If the module talks to something new, add the edge:

```text
upsert_edge {
  from_slug: "backend.auth",
  to_slug: "github.api",  // external node you may need to create first
  kind: "calls",
  label: "OAuth"
}
```

### "Attach the ADR you just wrote"

After writing a decision document, link it from the relevant node so
future readers find it:

```text
arch.link_doc {
  project_id: "...",
  slug: "backend.auth",
  doc_path: "projects/<id>/context/decisions/2026-05-19-github-oauth.md",
}
```

### "Sanity-check the diagram"

Before declaring work done:

```text
arch.list_nodes { root_only: true }
```

If the top-level set surprises you (missing a service you know
exists, contains something deprecated), fix it.

## Token discipline

The arch surface follows the same slim-by-default discipline as tasks.
Keep your responses small.

**`list_nodes` and `list_edges` are slim by default.** Rows carry only
the keys you need to keep walking the graph (`id`, `slug`, `parent_id`,
`kind`, `name`, `position`, `updated_at` for nodes; `id`, `from_slug`,
`to_slug`, `kind`, `label`, `updated_at` for edges). The
`description_md`, `metadata`, `linked_repo`, `linked_path` and the
`from_name` / `to_name` mirrors are omitted. Pass `verbose: true` only
when you genuinely need the full shape (rare during a walk).

**`get_node` opts in to children/edges/links.** The base node is
returned in full (description included — that's why you called `get`)
but the related collections are off by default. Pass
`include_children: true`, `include_edges: true`, `include_links: true`
only for the ones you actually need next.

**Mutations return a slim ack by default.** `upsert_node`,
`upsert_edge`, `move_node` and `upsert_kind` echo back only the keys
you need to chain the next call — not the description you just sent.
Pass `verbose: true` when you want the full object (rare; usually you
already know what you wrote).

**Don't re-`get_node` the same slug in a session.** If you just
upserted it, you already have the state. The skill bundle is stable —
reading `domains/architecture.md` twice in one session learns nothing
new.

## Anti-patterns to avoid

- **Don't model every function as a node.** The architecture is a
  conceptual map, not a call graph.
- **Don't invent custom kinds for one-off cases.** Reuse `service`,
  `module`, `component`, `external` whenever possible.
- **Don't leave the architecture stale.** When you delete a module
  in the code, `arch.remove_node` it in the same session.
- **Don't write circular edges**. They are rejected by self-loops,
  but you can still create dependency cycles `A→B→C→A`. The graph
  visualisation (M5) will flag these visually; until then, when you
  notice one, restructure.

## Things you cannot do (today)

- Build the diagram automatically from the source code
  (tree-sitter scan). Planned for v2.
- Validate that declared edges match real calls in the codebase.
  Also v2.
- Edit the diagram from the web UI. Humans read (tree view + hand-
  rolled SVG graph view, both read-only). Agents write.
