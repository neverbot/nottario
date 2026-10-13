---
name: nottario-domain-tasks
description: 'Complete reference for the Nottario tasks domain: schema, semantics, every tool, edge cases.'
---

# Tasks domain — full reference

## Entities

### Task

| Field                   | Type                                 | Notes                                                                 |
|-------------------------|--------------------------------------|-----------------------------------------------------------------------|
| `id`                    | uuid                                 |                                                                       |
| `project_id`            | uuid                                 |                                                                       |
| `parent_task_id`        | uuid \| null                         | When set, this task is a child of a `feature` parent.                 |
| `type`                  | `task`\|`bug`\|`chore`\|`spike`\|`feature` |                                                                       |
| `title`                 | text                                 | Required.                                                             |
| `description_md`        | text                                 | Markdown.                                                             |
| `state`                 | `todo`\|`doing`\|`done`\|`wont_do`   | Lifecycle. `done` and `wont_do` are both terminal.                    |
| `priority`              | int                                  | 0–100, read-only. Higher = picked sooner. Set it with `priority_key`. |
| `assignee_user_id`      | uuid \| null                         | Specific user. MCP shows it as `assignee`: `"me"` or a login.         |
| `target_role_id`        | uuid \| null                         | Role-scoped; eligible to any holder. MCP shows `role`: the key.       |
| `actual_start`          | timestamp \| null                    | Set automatically when entering `doing` (kept across re-enters).      |
| `actual_end`            | timestamp \| null                    | Set automatically when entering `done`, cleared on revert.            |
| `created_by_user_id`    | uuid \| null                         | Audit.                                                                |
| `created_by_token_id`   | uuid \| null                         | Audit.                                                                |

### Dependency

A directed edge `(task_id, depends_on_id)`. The Nottario server
rejects edges that would create a cycle. A task is considered
*eligible* (`nottario.tasks.next`) only when every `depends_on` is
in `state = 'done'`.

### Commit link

`(task_id, repo, sha, message)`. Repo must be in `"owner/repo"`
form. Re-linking the same `(task_id, repo, sha)` updates the message.

### Comment

`(id, task_id, author_user_id, author_token_id, body_md,
created_at)`. Append-only.

### Step

`(id, position, text, done, done_at, done_by_user_id)`. An ordered
checklist on a task: short actions worth remembering that need no
owner, role, dependency, commits or discussion of its own. If an item
needs any of those, it is a subtask, not a step. A step is one line of
up to 200 characters; a task holds at most 50. Steps are either done
or deleted: there is no "skipped" state.

## Tool surface

### `nottario.tasks.list`

Filter parameters (all optional except `project_id`):

- `state` — restrict to one state.
- `type` — restrict to one task type.
- `assignee_user_id` / `target_role_id` — restrict assignment. Pass
  `"me"` for yourself and a role by its key (`"backend"`); uuids work
  too. Responses use the same terms: `assignee: "me"` or a login,
  `role: "backend"`, and `updated_at` in UTC to the second.
- `parent_task_id` — list children of a specific feature.
- `include_children: true` — by default only top-level tasks
  (parent_task_id IS NULL) are returned; set this to flatten feature
  subtrees.
- `include_closed: true` — by default the response is restricted to
  **open** tasks (state `todo` or `doing`) so closed rows don't
  dominate every walk in a long-lived project. Set this to also
  include `done` and `wont_do`. An explicit `state` filter
  (`state: 'done'`, `state: 'wont_do'`) always wins regardless of this
  flag.
- `cycle_id` (optional) — when **omitted**, all read tools
  (`tasks.list`, `tasks.next`, `tasks.claim_next`) default to the
  project's **active cycle**. Pass a specific `cycle_id` to inspect a
  closed cycle. Pass `"all"` for the no-filter behaviour (every
  cycle). To see what shipped in sprint-2 specifically:

  ```
  nottario.tasks.list { project_id, cycle_id: <sprint-2 id>, state: 'done' }
  ```

  See `domains/cycles.md` for the full cycles surface.

Ordering: `priority DESC, created_at ASC`.

#### Pagination

Keyset pagination: `limit` (1..500; default the project's
`mcp_page_size`, 50 unless changed in Settings → MCP) and `cursor`
(empty for the first page, then the previous `next_cursor`). Each
page is `{tasks, next_cursor, has_more}`; loop while `has_more`. The
cursor encodes `(priority, created_at, id)`, so concurrent writes do
not skip or repeat rows.

`tasks.next` and `claim_next` only look at `todo`: before previewing
new work, list your own `doing` rows (`assignee_user_id: "me"`) and
report them first (`skill.md` §3).

### `nottario.tasks.next`

Returns the next eligible task or `{task: null}` if nothing is
available. Eligibility:

1. `state = 'todo'`.
2. `type <> 'feature'` (features are containers).
3. No `depends_on` row points to a task that is not `done`.

Filters narrow the candidate set:

- `assignee_user_id` set → tasks assigned to that user **or**
  unassigned tasks whose `target_role` matches one of the user's
  roles in the project.
- `role_id` set → tasks whose `target_role_id` equals it or is null.

### Priorities

Each project defines its own priority buckets (named labels mapped to
a numeric value). Defaults seeded on project creation: `low=30`,
`medium=60`, `high=90`, `critical=100`. Admins can rename, retune or
add buckets per project.

**Priorities are set by key, never by number.** Pass one of the
project's keys as `priority_key` to `tasks.create` / `tasks.update`;
`nottario.projects.list_priorities` returns them. The task tools take
no numeric priority: a number that misses every bucket would show on
the board as an unnamed `p70` instead of `high`. An unknown key is
rejected, and the error lists the keys the project does have.

The `priority` number you read back on a task is the value of its
bucket. Do not try to place a task between two buckets: tasks inside
a bucket are already ordered oldest first. If the project needs a
level it does not have, ask the humans to add a bucket in the project
settings.

### `nottario.tasks.create`

Defaults: `state=todo`, `type=task`, the project's default priority.
**`claim: true` when the work is yours and starts now**: the task is
created assigned to you and in `doing`. Leave it out for backlog work,
work for someone else, or later.

**One task per role.** Work that spans roles is a `feature` parent
with one child per role, chained with `add_dependency` in execution
order; each child is pickable by the right agent, and the parent
closes itself when they are all done:

```text
F  = create { type: "feature", title: "Sign in with Google" }
B  = create { parent_task_id: F.id, target_role_id: "backend",  title: "OAuth callback + session" }
Fe = create { parent_task_id: F.id, target_role_id: "frontend", title: "Sign-in button" }
     add_dependency { task_id: Fe.id, depends_on_id: B.id }
Q  = create { parent_task_id: F.id, target_role_id: "qa",       title: "Smoke the flow" }
     add_dependency { task_id: Q.id, depends_on_id: Fe.id }
```

New tasks land in the active cycle (no `cycle_id` here); move one with
`tasks.update` afterwards.

**Send text as you would type it.** Plain UTF-8: `Build & deploy`, not
`Build &amp; deploy`; real quotes and real line breaks, never `\"` or
a literal `\n`. The transport encodes for you, and pre-escaped text is
stored and shown verbatim. Same for `tasks.update`.

### `nottario.tasks.update`

Mutates the fields you pass. Notable nuances:

- Pass `assignee_user_id: ""` (empty string) to **unassign** the
  user. Same for `target_role_id`.
- Changing the priority is the canonical way to reorder; pass
  `priority_key`. There is no numeric input.
- Use this for description edits and renames; do not delete-and-recreate.
- **Reparenting cascades `cycle_id`**: setting `parent_task_id` on a
  leaf task forces the task's `cycle_id` to match the new parent's
  cycle (DB trigger; any `cycle_id` you pass alongside is overridden).
  To move a leaf to a different cycle, either detach it from its
  feature parent first, or move the whole feature subtree instead.

### `nottario.tasks.set_state`

Moves a task between states and manages `actual_start` /
`actual_end` for you:

- `todo` → clears both, and the task goes back to having no owner.
- `doing` → fills `actual_start` (only if currently null).
- `done` → fills `actual_end` and preserves any earlier `actual_start`.

**Moving a task out of `todo` makes it yours if nobody owns it.** The
server assigns you as it moves, so a task can never sit in `doing`,
or reach `done`, with nobody answering for it. It never takes a task
away from an existing assignee. This is a safety net, not the
intended route: claim the work deliberately with `tasks.claim`,
`claim_next`, or `create { claim: true }`, so the assignment says "I
took this" rather than "the server noticed nobody had".

**For `done` and `wont_do`, use `nottario.tasks.close` instead.** It
carries the closing comment and the commit links in the same
transaction, so a precondition failure cannot leave an orphan comment
behind. `set_state done` works and is not going away, but on its own
it closes a task with no record of what shipped — if you use it, you
still owe the comment and the commit links.

### `nottario.tasks.close`

The only way to finish a task: commit links, closing comment and state
transition in one transaction.

```
nottario.tasks.close {
  project_id, task_id,
  state: "done",                                   # or "wont_do"
  comment: "Fix: … Repro: … Test: … abc1234.",
  commits: [{ repo: "owner/repo", sha: "abc1234", message: "feat: x" }]
}
# → {task, comment_id?, linked_commit_count}
```

- **Link every commit the work produced.** The UI's Commits panel and
  "what shipped" queries read the links, not the comment. Only
  documentation or DB-recovery work has none.
- **Always comment, briefly.** Task / chore / spike: one sentence of
  what and why, plus the sha. Bug: `Repro: … Fix: … Test: …` in one or
  two lines, so a future agent can re-trigger it and trust the fix.
  `wont_do`: why ("superseded by <id>", "out of scope after X"). The
  commit message and the diff carry the detail. Feature parents close
  themselves when their children are done.
- **Steps left undone** do not block the close; the response adds
  `pending_steps: [{id, text}]` and a `notice`. Tell the human which
  ones were left.
- **Never split it** into `link_commit` + `add_comment` + `set_state`:
  a failure half-way leaves a comment and links on an open task.

**Preconditions.** `done` is refused while a direct dependency is not
`done`; nothing is written and the response lists what is open:

```jsonc
{ "error": "cannot close task: 1 unresolved precondition",
  "preconditions": [{ "id": "…", "title": "API endpoint", "state": "todo" }] }
```

Close the preconditions first (claim the ones still in `todo`), never
work around them. Feature parents are exempt: they roll up on their
own.

### `nottario.tasks.add_dependency` / `remove_dependency`

`add_dependency` rejects edges that would form a cycle:

```
A depends_on B   ✓
B depends_on A   ✗   (cycle)
```

Cycle detection considers the entire transitive graph, not just the
direct edge.

### `nottario.tasks.link_commit`

`repo` is the GitHub-style `"owner/repo"` string. `sha` can be the
full 40-char hash or a shorter prefix; we store what you send.
`message` is optional (it's the human-readable subject for
display). Re-linking the same `(task, repo, sha)` updates the
message in place.

### `nottario.tasks.unlink_commit`

The undo of `link_commit`: `{ project_id, task_id, repo, sha }`.
Reach for it when a link is wrong — the commit went to the wrong task,
or you amended or rebased after linking and the old sha no longer
exists. In the second case, unlink the old sha and link the new one.

`repo` and `sha` must match the link exactly as it was stored; read
them with `tasks.get { include_commits: true }` rather than guessing.
A link that does not exist is an error, not a silent success: it
almost always means the sha is wrong.

### `nottario.tasks.add_comment`

Body is markdown. The comment is attributed to the calling user and
token. Comments are append-only; there is no edit or delete from
the agent surface (a human can purge data directly in Postgres if
needed).

### Steps: `add_steps`, `update_step`, `remove_step`, `move_step`

```
nottario.tasks.add_steps   { project_id, task_id, texts: ["Write the migration", "Regenerate sqlc"] }  # → {steps: new ones, progress}
nottario.tasks.update_step { project_id, task_id, step_id, done: true }                               # → {id, text, done, progress}
nottario.tasks.update_step { project_id, task_id, step_id, text: "Regenerate sqlc and commit dbq" }
nottario.tasks.remove_step { project_id, task_id, step_id }                                            # → {removed: true, progress}
nottario.tasks.move_step   { project_id, task_id, step_id, position: 0 }                               # → {moved: true, position}
```

Each step comes as `{id, text, done}`, and `progress` is
`{done, total}`. Writes answer with what they touched, never the whole
checklist: keep the ids `add_steps` gives you, you need them to tick.
`tasks.get` lists only the **open** steps under `steps`, in order, with
the count in `task.steps: {done, total}`; pass
`include_done_steps: true` when you need the done ones too. List rows
and slim task shapes carry only the progress, `steps: {done, total}`,
and omit it when the task has no steps.

How to use them:

- **When you start a task whose work has more than one distinct
  action, write its steps.** The list tells the human, and the next
  agent, what is done and what is left without reading comments.
- **When new work appears mid-task, add a step** instead of writing a
  long comment about it. If the new work needs its own owner, role or
  commits, file a subtask instead.
- **Tick each step as you finish it,** not all at the end: the progress
  on the board is only useful while it is current.
- **Past about 15 steps, look for subtasks.** A list that long usually
  holds separate deliverables (different commits, roles or review);
  split those into child tasks and keep steps for the small actions
  inside each. The cap stays at 50.
- **Keep steps short and imperative** ("Add the index", not "I think
  we should probably add an index at some point").
- **Delete a step that turned out to be unneeded** rather than leaving
  it pending; a pending step reads as unfinished work.
- **Before closing, check the steps.** Closing with steps undone is
  allowed: `tasks.close` and `set_state done` still close the task,
  and their response adds `pending_steps` and a `notice`. When you see
  them, **tell the human which steps were left undone**, in your reply,
  not only in the closing comment.

## When to file a task before doing the work

Two shapes of "new work" both go through `nottario.tasks.create`
**before** you write any code, open any editor, or run any command:

1. **Side-channel requests / bugs spotted in passing.** "Ah, and we
   should also…", "this is broken: …", "I noticed X". File the row,
   decide whether to pivot or stay on the current task. Verbatim
   quotes from the user (the bug repro, the half-formed idea) belong
   in the description — future-you will not remember them.
2. **Substantive new work the user explicitly asks you to do.**
   "Let's add Biome", "do the design review of the Kanban", "rename
   `content_md` to `content`". Even when the user is telling you to
   *act*, the act starts with `tasks.create { claim: true }` → work.
   Skipping the row because "the request is obviously the task"
   leaves the backlog blind: the work has no handle for tracking, no
   audit trail, no link to the resulting commits. The exception is
   conversational tweaks that fit in a single small commit and need
   no follow-up (a typo fix in a doc, a one-line CSS adjustment) —
   those can land directly.

File the first shape plain — it goes to the backlog — and the second
with `claim: true` when you start on it right away.

Both shapes need the right `target_role`, an honest description,
dependencies linked if relevant, and a split into role children when
multi-role. The bar is: if I had to leave the session right now,
would someone else be able to pick this up? If not, file more
context.

## Idiomatic patterns

### "Carry on" — the loop

The loop itself is in `skill.md` §3. Details:

- `claim_next` is `SELECT … FOR UPDATE SKIP LOCKED`: parallel agents
  get different tasks. Pass `assignee_user_id: "me"` to include todos
  targeted at your roles, or `role_id: "backend"` to stay in one lane.
- **A topic or an id from the human:** find candidates with
  `tasks.list { state: "todo" }` (already ordered priority desc, then
  oldest first) and `tasks.claim` them one by one until one succeeds.
  A failed claim returns `{error, reason, current_state,
  current_assignee, preconditions, pending_children_count}`: someone
  took it, a precondition is open, or a feature still has children.
  Surface that to the human rather than retrying blindly.
- **Never `next` + `update` + `set_state`.** Another agent can take
  the task between any two calls.
- A task someone else holds is theirs: comment or ask the human,
  never reassign it silently.

### "I found a bug while doing my task"

Create it; do **not** silently fix it without filing:

```text
nottario.tasks.create {
  project_id, title, type: "bug", priority_key: "high",
  description: "context, repro, suspected fix",
}
```

Then continue with your current task. The general rule (verbatim
quote, file path, proposed direction, role split) lives in `skill.md`
§5.

### "The user just mentioned a different task / bug / feature"

Same rule, broader scope: any side-comment from the human about work
that is NOT what you're currently executing → FIRST action is
`nottario.tasks.create`, only THEN decide whether to pivot or keep
going. If the item lives only in conversation, multi-agent and
multi-session work lose the single source of truth.

When to pivot vs. keep going: cheap context-switch (≤5 min, no
architectural decision required) → pivot, file, fix, return.
Otherwise: file with enough context that someone else can pick it
up, and resume your current task.

### "Block this until X is done"

After you create both tasks, declare the order:

```
nottario.tasks.add_dependency {
  project_id, task_id: B, depends_on_id: A,
}
```

`tasks.next` will then skip B until A is `done`.

## Token discipline

Everything a tool returns stays in your context for the rest of the
session. Ask for the minimum.

- **Responses are slim by default.** Mutations and `tasks.list` return
  `{id, type, title, state, priority, role, assignee, updated_at,
  steps}`, never the description or comment you just sent.
  `tasks.get` adds the description, `related` and the open steps.
  `verbose: true` brings the full row; you almost never need it.
- **Opt in deliberately**: `include_commits`, `include_comments`,
  `include_done_steps` on `tasks.get`; `include_closed` or a closed
  `state` on `tasks.list` (closed rows pile up forever).
- **Don't re-fetch.** Reuse the id and fields a previous call gave you;
  `whoami` once per session; never re-read a page of this bundle you
  already read this session.
- **No pickup comments** ("starting this", "on it"): `doing` and the
  assignee already say it. Comment when a future reader needs it: a
  decision, a blocker, the close.
- **Closing comments are a line**, not an essay (§`tasks.close`).
- **Step writes answer small** (the touched step and the progress);
  keep the ids `add_steps` returns instead of re-reading the task.
- **`nottario.search`** is capped at 20 hits by default (max 100) and
  slim; raise the limit only for a genuinely wide sweep.

## Things you cannot do (today)

- Delete a comment via MCP (the web UI can; agents cannot).
- Edit historic actual_start/actual_end.
- Modify the project, role catalogue or memberships (admin-only,
  done via the web UI).
- Subscribe to live updates from the MCP — agents poll. The web UI
  uses SSE; agents do not need real-time and we keep the protocol
  simple.
