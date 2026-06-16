---
name: nottario-domain-tasks
description: Complete reference for the Nottario tasks domain: schema, semantics, every tool, edge cases.
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
| `state`                 | `todo`\|`doing`\|`done`              | Lifecycle.                                                            |
| `priority`              | int                                  | 0–100 by convention. Higher = picked sooner.                         |
| `assignee_user_id`      | uuid \| null                         | Specific user.                                                        |
| `target_role_id`        | uuid \| null                         | Role-scoped; eligible to any holder.                                 |
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

## Tool surface

### `nottario.tasks.list`

Filter parameters (all optional except `project_id`):

- `state` — restrict to one state.
- `type` — restrict to one task type.
- `assignee_user_id` / `target_role_id` — restrict assignment.
- `parent_task_id` — list children of a specific feature.
- `include_children: true` — by default only top-level tasks
  (parent_task_id IS NULL) are returned; set this to flatten feature
  subtrees.
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

`tasks.list` is **paginated** with a keyset cursor:

- `limit` — page size, 1..500. Omit to use the project's configured
  `mcp_page_size` (default 50, editable in the web UI under
  *Project settings → MCP*).
- `cursor` — opaque string. Empty/omitted ⇒ first page. Otherwise pass
  the previous response's `next_cursor`.

Each response is now `{tasks, next_cursor, has_more}` (instead of just
`{tasks}`). The canonical walk loops while `has_more`:

```text
cursor = ""
loop:
  page = tasks.list { project_id, state: "todo", cursor }
  process page.tasks
  if not page.has_more: break
  cursor = page.next_cursor
```

Filters can change between calls (e.g. swap `state` mid-walk) without
corrupting the cursor — the ordering is stable across mutations
because the cursor encodes `(priority, created_at, id)`.

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

**Always pick a key from the project's vocabulary** — call
`nottario.projects.list_priorities` first and pass the chosen key as
`priority_key` to `tasks.create` / `tasks.update`. Avoid passing raw
numbers in `priority` unless you have a deliberate reason to bypass
the buckets (e.g. inserting between two existing buckets).

### `nottario.tasks.create`

Defaults: `state=todo`, `type=task`, `priority=50`. To create a
feature with subtasks:

```text
1. create(type='feature', title='Sign in with Google')        → F
2. create(parent_task_id=F.id, type='task', target_role_id=design)    → A
3. create(parent_task_id=F.id, type='task', target_role_id=backend)   → B
   then add_dependency(B, A)
4. create(parent_task_id=F.id, type='task', target_role_id=frontend)  → C
   then add_dependency(C, B)
5. create(parent_task_id=F.id, type='task', target_role_id=qa)        → D
   then add_dependency(D, C)
```

The parent transitions to `done` automatically when all its children
are `done`.

`tasks.create` does **not** accept a `cycle_id` argument — every new
task lands in the project's active cycle. To create a task in a
different cycle, create it first, then move it with `tasks.update`
(and reparent the whole feature if the target is a leaf under a
feature parent — see the cascade note under `tasks.update`).

**Send `title` and `description` as plain UTF-8.** Do not HTML-encode
ampersands, angle brackets or quotes — `Build & deploy`, not
`Build &amp; deploy`. The kanban, gantt and detail dialog escape
these for display, so an encoded payload renders to the user as the
literal `&amp;`. The server now decodes a small set of common
entities defensively when it receives them, but the cleanest fix is
to never encode in the first place. The same rule applies to
`tasks.update`.

#### One task per role

When a unit of work spans multiple roles (e.g. backend migration +
frontend reorder UI + qa smoke), **do not file a single multi-role
task**. Create one task per affected role and link them with
`add_dependency` in the order they must be executed. Each role-scoped
task is then individually pickable by the right agent and tracked
independently.

Typical pattern: group the role-scoped tasks under a `type=feature`
parent so they share a title and roll up to `done` together.

```text
1. create(type='feature', title='Roles: add order, drag UI, Gantt lanes')  → F
2. create(parent_task_id=F.id, target_role_id=backend, title='migration + API')   → B
3. create(parent_task_id=F.id, target_role_id=frontend, title='drag UI + Gantt')  → Fe
   then add_dependency(Fe, B)
4. create(parent_task_id=F.id, target_role_id=qa, title='smoke reorder + lanes')  → Q
   then add_dependency(Q, Fe)
```

### `nottario.tasks.update`

Mutates the fields you pass. Notable nuances:

- Pass `assignee_user_id: ""` (empty string) to **unassign** the
  user. Same for `target_role_id`.
- Changing `priority` is the canonical way to reorder; pass
  `priority_key` (resolved against project buckets) rather than a raw
  number.
- Use this for description edits and renames; do not delete-and-recreate.
- **Reparenting cascades `cycle_id`**: setting `parent_task_id` on a
  leaf task forces the task's `cycle_id` to match the new parent's
  cycle (DB trigger; any `cycle_id` you pass alongside is overridden).
  To move a leaf to a different cycle, either detach it from its
  feature parent first, or move the whole feature subtree instead.

### `nottario.tasks.set_state`

The only correct way to move a task between states. It manages
`actual_start` and `actual_end` for you:

- `todo` → clears both.
- `doing` → fills `actual_start` (only if currently null).
- `done` → fills `actual_end` and preserves any earlier `actual_start`.

#### Closing-the-loop checklist

After every `set_state done`, run the three checks from `skill.md` §4:

1. Is there another open task that describes the same delivery?
   Close it too instead of letting a duplicate row sit in `todo`.
2. Did the work add/remove/modify components or their relations?
   Update `nottario.arch.*` so the diagram matches reality
   (`domains/architecture.md` §"When to touch the architecture").
3. Did the human mention side-work? File it with
   `nottario.tasks.create` BEFORE moving on (see §"The user just
   mentioned a different task / bug / feature" below).

#### Always link commits before closing

Before `set_state done`, call `nottario.tasks.link_commit { repo,
sha }` once per commit the task produced. This is non-negotiable
whenever the work yielded code: the Commits panel in the UI, the
"what shipped here" queries and any traceability audit all depend on
the structured link, not on prose in a comment. The bar is: a future
reader of the closed task can jump straight to the diff without
grepping git. Tasks that are pure documentation or bug-recovery in
the DB legitimately have no commit; everything else does.

#### Always leave a closing comment

Before `set_state done` (or `wont_do`, or any terminal state), call
`nottario.tasks.add_comment` with a short summary of what actually
happened. The diff is the truth; the closing comment is the **story**
a future reader needs to understand the diff without reverse-
engineering it. Skipping this is a low-friction mistake that compounds
— months later nobody remembers why a task was closed without code,
why `wont_do` was the right call, or what the bug actually was.

What the comment must carry, by task type:

- **task / chore / spike** — one paragraph: what you delivered, the
  non-obvious decisions you made, and any follow-up you spotted but
  did not file as its own task (a small enough loose end to live as a
  note here instead of a new row). Link the commits inline for
  readability even though the structured `link_commit` already exists.
- **bug** — three sections, terse but explicit:
  1. **Repro** — the exact steps or input that triggered the bug, in
     past tense. Make it copy-pasteable. The bar: a future agent
     should be able to re-trigger from this paragraph alone, without
     reading the original report.
  2. **Fix** — the change in one or two sentences, focusing on *why*
     this is the right fix and not just the apparent one. If you
     rejected an obvious alternative, mention it.
  3. **Test** — what you ran to confirm: integration test added, manual
     reproduction now fails to trigger, smoke test in the dev
     container, etc. "Verified the gate is green" is not a test plan.
- **feature** parents close automatically when their children flip to
  `done`; you don't usually comment on the parent. Leave the closing
  story on each child.
- **`wont_do`** — say why. "Superseded by `<id>`", "out of scope after
  user clarified X", "infeasible because Y". A `wont_do` with no
  comment looks indistinguishable from "forgot about it" to anyone
  who comes back later.

```text
nottario.tasks.add_comment {
  task_id,
  body: "Fix: deferred the cycle check to inside the tx (was racing\n
  the FOR UPDATE). Repro: two concurrent `add_dependency` calls on the\n
  same node — used to allow A→B→A intermittently. Test: new\n
  TestAddDependency_NoCycleUnderRace in repo_integration_test.go,\n
  10 iterations under -race. Commits abc1234, def5678."
}
nottario.tasks.set_state { task_id, state: "done" }
```

The pattern is **always**: link commits → add comment → set_state.
Comment before state, not after — once a task is `done` it slips out
of the active backlog and the comment you forgot is much less likely
to land.

#### Preconditions are enforced

Closing a task (`state: "done"`) is **rejected** when the task has at
least one direct dependency whose own state is not `done`. The
response carries a `preconditions` array listing what's still open:

```jsonc
{
  "error": "cannot close task: 2 unresolved preconditions",
  "preconditions": [
    { "id": "…", "title": "Backend migration", "state": "doing" },
    { "id": "…", "title": "API endpoint",      "state": "todo"  }
  ]
}
```

The fix is always to close the preconditions first, never to bypass.
Canonical pattern when an agent hits this:

```text
me = nottario.whoami { }
# walk every unresolved precondition; pick whichever is yours.
for p in error.preconditions:
    if p.assignee_user_id == me.user_id and p.state == "todo":
        # work on p first.
        ...
```

Feature parents (`type: feature`) are an exception — the engine rolls
them up automatically when all their children are `done`, so the
check is skipped for them.

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

### `nottario.tasks.add_comment`

Body is markdown. The comment is attributed to the calling user and
token. Comments are append-only; there is no edit or delete from
the agent surface (a human can purge data directly in Postgres if
needed).

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
   *act*, the act starts with `tasks.create` → `claim` → work.
   Skipping the row because "the request is obviously the task"
   leaves the backlog blind: the work has no handle for tracking, no
   audit trail, no link to the resulting commits. The exception is
   conversational tweaks that fit in a single small commit and need
   no follow-up (a typo fix in a doc, a one-line CSS adjustment) —
   those can land directly.

Both shapes need the right `target_role`, an honest description,
dependencies linked if relevant, and a split into role children when
multi-role. The bar is: if I had to leave the session right now,
would someone else be able to pick this up? If not, file more
context.

## Idiomatic patterns

### "Carry on" — the loop

Use **`nottario.tasks.claim_next`** to atomically pick AND claim the
next eligible task in one MCP call. It sets `assignee = you` and
`state = doing` in a single Postgres UPDATE backed by `SELECT … FOR
UPDATE SKIP LOCKED`, so two agents running this loop in parallel get
two DIFFERENT tasks — no double-claim, no race.

```
loop:
  result = nottario.tasks.claim_next {
    project_id,
    assignee_user_id: me.user_id   # optional: include role-matched todos for me
  }
  if result.task is null: tell the human "no eligible tasks" and stop
  task = result.task

  ...do the work in the local repo...

  nottario.tasks.link_commit { task_id: task.id, repo, sha }
  nottario.tasks.add_comment { task_id: task.id, body: "..." }   # REQUIRED — see §"Always leave a closing comment"
  nottario.tasks.set_state   { task_id: task.id, state: "done" }
goto loop
```

#### "Take the next task about topic X"

When the human (or your own judgement) narrows the pickup to a topic,
discover candidates with `tasks.list`, then **claim a specific id
atomically** with `nottario.tasks.claim`. The claim either succeeds or
returns a 409-shaped conflict with details; if it loses the race or
the task isn't eligible, try the next candidate.

```
candidates = nottario.tasks.list { project_id, state: "todo" }
relevant   = filter(candidates, matches=topic_X)   # client-side reasoning
for t in relevant:                                 # already ordered priority DESC, created_at ASC
  result = nottario.tasks.claim { project_id, task_id: t.ID }
  if result.error:
    # 409: somebody else just took it OR preconditions still pending OR feature has open children
    # See result.reason, result.current_state, result.current_assignee_user_id,
    #     result.preconditions[], result.pending_children_count.
    continue
  task = result   # task is yours, already in doing
  break
```

#### "Work on this specific task"

If the human hands you an id, call `claim` directly. Read the
conflict shape on failure and surface it to the human ("that task is
already in doing assigned to X", "preconditions still pending: …").

#### Why the old three-call pattern is gone

The historical `tasks.next` + `tasks.update {assignee}` + `set_state
doing` sequence is **racy**: between any two calls another agent can
slip in and claim the same task. Use `claim_next` / `claim` instead;
`tasks.next` is a preview, never a pickup.

If a task you want is already claimed by another user: leave a
`nottario.tasks.add_comment` or escalate to the human. Do not
silently re-assign.

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
§"Filing work as you discover it".

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

## Things you cannot do (today)

- Delete a comment.
- Edit historic actual_start/actual_end.
- Modify the project, role catalogue or memberships (admin-only,
  done via the web UI).
- Subscribe to live updates from the MCP — agents poll. The web UI
  uses SSE; agents do not need real-time and we keep the protocol
  simple.
