---
name: nottario
description: Use when working on a software project tracked in Nottario. Establishes the agent's identity, locates the active project, finds the next task to work on, and records progress (state changes, linked commits, comments).
---

# Nottario skill

Nottario coordinates developers and their agents around three domains:
**tasks**, shared **markdown context**, and an **architecture** diagram.
This file is the entry point. The full reference for each domain lives
under `domains/`; call `nottario.skill.read` to pull the one you need.

## 1. Identify yourself

Always start with `nottario.whoami`. It tells you the **user** you act
on behalf of (`user_id`, `github_login`), whether you are an **admin**,
the **token_id** authenticating you, and the **single project** the
token is scoped to (`memberships[0].project_id` and friends — token
callers always see exactly one membership: the one belonging to the
token's project). On failure the token is missing or revoked — stop
and ask the human for a fresh one (web UI → open the project →
**Settings → Tokens → New token**).

Tokens are **per-project**. One token = one project. Passing
`project_id` of a different project on any subsequent call returns
`"token scoped to project X, request targets Y"`. Cache the project
id from `whoami` and reuse it on every tool call — never let the user
or another step override it without re-running `whoami`.

Deep dive: `references/identity.md`.

## 2. Locate the project

Every tool call that touches a project needs `project_id` as an
explicit argument. With per-project tokens, the answer comes straight
from step 1: `whoami.memberships[0].project_id` is the only project the
token can reach. Cache it once and pass it on every call.

`nottario.projects.list` exists for completeness but for a token
caller it returns only that one project. Use it if you want the slug
or name; the id is the same one `whoami` already gave you.

If you need the role catalogue (to assign to "any backend"), call
`nottario.projects.list_roles` once and cache.

## 3. Work a task end-to-end

The loop when the human says "carry on" or "do the next thing":

1. **Atomic claim.** `nottario.tasks.claim_next { project_id }`
   (optionally `assignee_user_id` = your user_id, or `role_id`). It
   picks the highest-priority eligible `todo` task AND sets
   `assignee = you`, `state = doing` in a single SQL transaction
   (`SELECT … FOR UPDATE SKIP LOCKED`). Safe to run from many agents
   in parallel. Returns `{task: null}` when nothing is eligible.
2. **Do the work** in the local repo. Nottario does not store code.
3. **Link commits.** `nottario.tasks.link_commit { repo, sha }` for
   every commit the task produced. Non-negotiable when the task
   yielded code — see `domains/tasks.md` for why.
4. **Note non-obvious things.** `nottario.tasks.add_comment` for
   trade-offs, follow-up ideas, why A over B. Future readers benefit.
5. **Close.** `nottario.tasks.set_state { state: "done" }`. The
   server rejects this if a dependency is still open and returns a
   `preconditions` array — work those first.
6. **Close the loop** (see below), then go back to step 1.

When the human narrows the pickup ("the next task about topic X" or
"work on this id"), discover candidates with `nottario.tasks.list` and
take one atomically with `nottario.tasks.claim { task_id }`. Read the
409 conflict shape if you lose the race.

`nottario.tasks.next` is a **preview only** — no side effects.
Useful to inspect what `claim_next` would pick. Never the first step
of pickup.

Deep dive: `domains/tasks.md`.

## 4. Close the loop after substantial work

Every time you finish a meaningful change — whether you took it via
`claim_next` or just did it because the human asked directly — run
these three checks before moving on:

1. **Is there already a task for this work?** Search the backlog
   (`nottario.tasks.list` or `nottario.search`) for whatever you just
   delivered. If a matching task is open and your commits cover its
   acceptance, link the commits and `set_state done`. Do not leave
   a duplicate row sitting in `todo`; do not file a new task that
   describes the same delivery.
2. **Did the work add, remove or substantially modify any software
   component, or change how components relate?** If yes, walk the
   architecture with `nottario.arch.list_nodes` and
   `nottario.arch.get_node`, then `upsert_node` / `upsert_edge` /
   `remove_*` to match the new reality. The diagram is the team's
   shared mental model — let it lag and the next agent reads a lie.
3. **Did the human mention side-work along the way?** A "we should
   also…", a half-formed bug repro, a future-page idea. File it as
   a task NOW with verbatim context, before you forget. See "Filing
   work as you discover it" below.

## Filing work as you discover it

While working, you will spot things the human cares about but did not
ask for: bugs, follow-ups, missing features, side-comments dropped
into chat. **File them as tasks** rather than dropping them in your
reply:

```text
nottario.tasks.create {
  project_id,
  title: "Fix null deref in auth callback on duplicate state",
  type: "bug",
  priority_key: "high",
  target_role_id: "..."   // optional: route to a role rather than a person
}
```

Pick `type`:

- `task` — ordinary work,
- `bug` — defects you found,
- `chore` — cleanup,
- `spike` — time-boxed investigation,
- `feature` — *parent* whose children are the real work.

**Multi-role work → one task per role.** Create a `type=feature`
parent and one child task per affected role (design / backend /
frontend / qa), linked with `add_dependency` in execution order. The
parent rolls up to `done` automatically when all children are done.

Capture in the description:

- Verbatim what the human said (the repro, the design hunch).
  Future-you will not remember the phrasing.
- The current state-of-the-code that triggered it (a file path, a
  screenshot reference, the URL of the broken view).
- The proposed direction if you have one — mark "tentative" rather
  than omit.
- Role split when the work crosses multiple roles.

The bar: "if I had to leave the session right now, could someone else
pick this up?" If no, add more context.

Deep dive on the patterns: `domains/tasks.md` ("I found a bug while
doing my task", "The user just mentioned a different task / bug /
feature", "Block this until X is done").

## Rules of thumb

- **Re-confirm the project on every call.** Pass `project_id`
  explicitly; do not infer from conversation memory.
- **Never invent ids.** Always look them up via `list` / `get`.
- **Don't touch tasks belonging to other people without being asked.**
  If you must, leave a comment explaining why.
- **Prefer small, frequent updates over silent batching.** Set
  `state = doing` when you start, `state = done` when you finish.
- **If you are unsure whether to file a comment or not, file it.**
  Short comments are cheap; missing context is expensive.
- **Never `set_state done` if the task is not actually finished.** Use
  a comment to record mid-way state instead.

## Deeper guides

Call `nottario.skill.read` with the path:

- `references/identity.md` — token and identity mechanics.
- `domains/tasks.md` — full task API surface, edge cases, pagination,
  claim conflict shape, "carry on" / "block this" / "found a bug"
  patterns.
- `domains/docs.md` — the shared-markdown domain: skills, context,
  notes, frontmatter, optimistic concurrency, local↔Nottario sync.
- `domains/architecture.md` — the textual architecture diagram:
  nodes, edges, kinds, when to touch it, when to leave it alone.

## Overrides and snapshots

The skill is bundled with the binary; this is what shipped. Each
instance can **override or extend** any file without rebuilding: an
admin (or an agent with admin permissions) writes a document with
`scope=global`, `kind=skill`, `path=global/skills/<file>`. The next
call to `nottario.skill.read` resolves to the override; otherwise the
embedded copy is served. `nottario.skill.list` includes both, tagging
each entry with `origin: "embedded" | "global"`.

Use this to tighten a shipped file for your team, or to add bundle-
absent files (`by-language/go.md`, `by-role/security.md`,
`recipes/deploying-to-our-k8s.md`).

A snapshot of the current bundle (overrides applied) is available as a
zip at `GET /skill.zip` — useful to mirror the skill into a local
install or back up.

For pre-loaded installs, prefer the workspace path
`<repo>/.claude/skills/nottario/` (commit it; every contributor who
clones the repo gets the rules for free, scoped to that repo). Fall
back to `~/.claude/skills/nottario/` only when you work on the
project across multiple unrelated checkouts or do not want to version
the bundle in any specific repo. The on-demand path you are reading
right now (`nottario.skill.read`) is always available too and stays
in sync with the server.
