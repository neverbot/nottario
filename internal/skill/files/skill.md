---
name: nottario
description: Use when working on a software project tracked in Nottario. Establishes the agent's identity, locates the active project, finds the next task to work on, and records progress (state changes, linked commits, comments).
---

# Nottario skill

Nottario coordinates developers and their agents around three domains:
**tasks**, shared **markdown documents**, and an **architecture**
diagram. This file holds the rules for every session. The reference
for each domain sits next to it, in this bundle; open a page only when
the work needs it:

- `domains/tasks.md`: task API, pickup and close patterns, steps,
  claim conflicts, pagination.
- `domains/docs.md`: documents, versions, download and upload URLs,
  keeping a repo file and its Nottario copy in sync.
- `domains/architecture.md`: the architecture graph, and when to touch
  it.
- `methodology/git.md`: commits, branches and pushes for solo,
  parallel-agent and multi-developer work.
- `references/identity.md`: tokens, roles, memberships.
- `references/install.md`: installing and updating this bundle, and
  instance overrides.

## 1. Start of session

1. **`nottario.whoami`.** It names the user you act for, whether you
   are an admin, and the one project your token reaches
   (`memberships[0].project_id`). Pass that `project_id` explicitly on
   every call. A failure means the token is missing or revoked: stop
   and ask the human for a new one (project → Settings → Tokens).
2. **Check the bundle.** `whoami.skill_bundle_version` must equal
   `sha256:` + the SHA-256 of `SHA256SUMS` where this bundle is
   installed. If not, reinstall (`references/install.md`) and tell the
   human to restart their client.
3. **Pick your git mode** (solo, parallel agents, many humans) before
   touching code: `methodology/git.md`.

**The token belongs to your MCP client, not to you.** Never read it,
search for it, print it or call Nottario's HTTP API with it. Everything
is an MCP tool; documents move through signed URLs
(`docs.download_url`, `docs.upload_url`). If something seems to need
the token, stop and ask.

Names, not ids: roles by key (`target_role_id: "backend"`, responses
show `role: "backend"`), yourself as `"me"` (`assignee: "me"`).

## 2. Two rules for every task

1. **A task you work on is assigned to you.** Take it with
   `tasks.claim_next` or `tasks.claim`; when you file work you are
   about to start, `tasks.create { claim: true }` files it already
   yours and in `doing`.
2. **A task that reaches `done` has a closing comment**, and commit
   links when the work produced commits. `tasks.close { comment,
   commits }` does both and the transition in one call; never close
   with separate `link_commit` + `add_comment` + `set_state`.

## 3. The pickup loop

When the human says "what's next", "carry on" or similar:

1. **Your own `doing` first.** `tasks.list { assignee_user_id: "me",
   state: "doing" }`. Report any row as "still open from before" and
   resume it unless the human parks it.
2. **Claim atomically.** `tasks.claim_next { project_id }` picks the
   top eligible task and makes it yours in one transaction
   (`{task: null}` when nothing is eligible). For a named task or topic,
   find it with `tasks.list` and take it with `tasks.claim`.
   `tasks.next` is a read-only preview, never the way to take work. No
   "starting this" comments: `doing` is the record.
3. **Do the work.** When it has several distinct actions, list them as
   steps (`tasks.add_steps`) and tick each as you finish it
   (`tasks.update_step { done: true }`). New work found on the way is a
   new step, not a long comment. Past about 15 steps, look for subtasks.
4. **Close** with `tasks.close`: one sentence and the commit sha for
   ordinary work; Repro / Fix / Test for bugs. If the answer lists
   `pending_steps`, tell the human which steps were left undone.
5. **Close the loop** (section 4), then go back to 1.

## 4. After substantial work

- **Existing task?** Search the backlog (`tasks.list`, `search`) for
  what you delivered. Close the matching task instead of filing a
  duplicate.
- **Architecture changed?** If you added, removed or reshaped a
  component or how components relate, update the graph
  (`domains/architecture.md`).
- **Side-work mentioned?** File it now (section 5).
- **Project markdown changed?** ADRs, runbooks, design notes, agent
  instructions (`claude.md`, `AGENTS.md`, skill files): publish or
  update the Nottario document too, under `expected_version`
  (`domains/docs.md`).

## 5. File work as you discover it

Bugs, follow-ups and side-comments the human drops in chat become
tasks, not lines in your reply:

```text
nottario.tasks.create { project_id, title, type: "bug", priority_key: "high",
                        target_role_id: "backend", description: "..." }
```

That lands in `todo` for later; add `claim: true` if you start it now.
Types: `task`, `bug`, `chore`, `spike`, `feature` (a parent whose
children are the work). **Multi-role work is one task per role**
under a `feature` parent, linked with `add_dependency` in execution
order. In the description: the human's words verbatim, where in the
code it showed up, a tentative direction. The bar: could someone else
pick it up if you left now?

## 6. Rules of thumb

- Never invent ids; look them up. Pass `project_id` on every call.
- Don't touch other people's tasks unless asked; if you must, comment
  why.
- Never mark a task `done` that is not finished; comment the mid-way
  state instead.
- **Spend few tokens.** Responses are slim by default; ask for
  `verbose`, `include_comments`, `include_commits` or
  `include_done_steps` only when you need them. Don't re-fetch what you
  already have, don't re-read a page of this bundle in the same
  session, keep closing comments to a line. With a shell, read and edit
  documents through `docs.download_url` and a local file, never by
  pulling whole bodies into context.
