---
name: nottario
description: Use when working on a software project tracked in Nottario. Establishes the agent's identity, locates the active project, finds the next task to work on, and records progress (state changes, linked commits, comments).
---

# Nottario skill

This skill teaches you to use the Nottario MCP server. Nottario coordinates
developers and their agents around three domains: tasks, shared markdown
context, and an architecture diagram. This skill currently covers the
**tasks** domain; documents and architecture arrive in later milestones.

## Identifying yourself

Always start by calling `nottario.whoami` to confirm:

- which **user** you are acting on behalf of (`user_id`, `github_login`);
- whether you are an **admin**;
- the **token_id** that authenticates you.

If `whoami` fails, the configured API token is missing or revoked — stop
and ask the human to provide a fresh one from the Nottario web UI under
**Tokens → New token**.

## Locating the active project

Nottario does **not** keep a "currently active project" on the server side.
**Every tool call that needs a project requires `project_id` as an
explicit argument.** Resolve it once at the start of your session:

1. Call `nottario.projects.list` to see what is available.
2. Pick the project whose `slug` or `name` matches what the human asked
   you to work on (or ask the human if unsure).
3. Cache the `id` in your working memory for the rest of the session and
   pass it on every subsequent call.

If you need the role catalogue (to assign a task to "any backend"), call
`nottario.projects.list_roles` once and cache the result.

## Working a task end-to-end

The expected flow when the human says "carry on" or "do the next thing":

1. **Atomic claim.** Call `nottario.tasks.claim_next` with the
   project_id (and optionally `assignee_user_id = your user_id` from
   `whoami`, or a `role_id`). It picks the highest-priority eligible
   `todo` task AND marks it `assignee = you`, `state = doing` in a
   single SQL — safe to run from multiple agents in parallel without
   colliding on the same task. Returns `{task: null}` if nothing is
   eligible.
2. **Do the work in the human's repo locally.** Nottario does not store
   code; you work in the existing checkout as you always would.
3. **Commit and link.** When you commit, call
   `nottario.tasks.link_commit` with `repo="owner/repo"` and the SHA so
   that future readers can trace the work.
4. **Note non-obvious things.** Use `nottario.tasks.add_comment` for
   anything a future reader (human or agent) would benefit from: tricky
   trade-offs, follow-up ideas, why you chose A over B.
5. **Close.** `nottario.tasks.set_state` with `state="done"`. This
   records `actual_end`. The server rejects the close if the task
   still has unresolved preconditions.
6. **Loop.** Call `nottario.tasks.claim_next` again.

When the human narrows the pickup ("the next task about topic X" or
"work on this id"), discover candidates via `nottario.tasks.list` and
then take one atomically with `nottario.tasks.claim {task_id}` — see
`domains/tasks.md` for the canonical loops, including how to read the
409-shaped conflict if you lose the race or the task isn't eligible.

`nottario.tasks.next` is now a **preview-only** tool: it returns what
`claim_next` would pick, without mutating anything. Do not use it as
the first step of pickup — always go through `claim_next` / `claim`.

## Filing work as you discover it

While working, you will spot things the human cares about but did not
ask for: bugs, follow-ups, missing features. **File them as tasks**
rather than dropping them in your reply:

```
nottario.tasks.create {
  project_id: "...",
  title: "Fix null deref in auth callback on duplicate state",
  type: "bug",
  priority: 70,           // pick something sensible
  target_role_id: "..."   // optional: route to a role rather than a person
}
```

Pick `type`:

- `task` for ordinary work,
- `bug` for defects you found,
- `chore` for cleanup,
- `spike` for time-boxed investigations,
- `feature` for a *parent* task whose children are the actual work.

For multi-role features (design → backend → frontend → qa), create a
`feature` parent and child tasks linked to it via `parent_task_id`, then
declare the order with `nottario.tasks.add_dependency`.

## Rules of thumb

- **Always identify and re-confirm the project.** Do not assume the
  project from earlier in the conversation; pass `project_id`
  explicitly every time.
- **Do not invent ids.** Always look them up via `list` / `get`.
- **Do not change tasks that belong to other people without being asked.**
  If you have to, leave a comment explaining why.
- **Prefer small, frequent updates over silent batching.** Set state to
  `doing` as soon as you start; mark `done` as soon as you finish.
- **If you are unsure whether to file a comment or not, file it.** A
  short comment is cheap; missing context is expensive later.
- **Never call `nottario.tasks.set_state` with `done` if the task is
  not actually finished.** Use a comment to explain mid-way state.

## When in doubt

Call `nottario.skill.read` with the path of a deeper guide — for
example:

- `references/identity.md` — token and identity details.
- `domains/tasks.md` — the full task API surface and edge cases.
- `domains/docs.md` — the shared-markdown domain: skills, context,
  notes; frontmatter; optimistic concurrency.
- `domains/architecture.md` — the textual architecture diagram:
  nodes, edges, kinds, when to touch it and when to leave it alone.

The skill is bundled with the binary; what you read here is what
shipped. Each Nottario instance can **override or extend** any file
without rebuilding the binary: an admin (or an agent with admin
permissions) creates a document with `scope=global`, `kind=skill`
and `path=global/skills/<file>`. The next call to
`nottario.skill.read` resolves to the override; otherwise the
embedded copy is served. `nottario.skill.list` includes both,
tagging each entry with `origin: "embedded" | "global"`.

Use this to:

- **Override** a shipped file (e.g. tighten the rules in
  `domains/tasks.md` for your team).
- **Add** files that the bundle doesn't include
  (`by-language/go.md`, `by-role/security.md`,
  `recipes/deploying-to-our-k8s.md`).

A full snapshot of the current bundle (overrides applied) is
available as a zip at `GET /skill.zip` — useful to mirror the skill
into `~/.claude/skills/nottario/` on a machine, or for backups.
