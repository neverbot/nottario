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

### `nottario.tasks.set_state`

The only correct way to move a task between states. It manages
`actual_start` and `actual_end` for you:

- `todo` → clears both.
- `doing` → fills `actual_start` (only if currently null).
- `done` → fills `actual_end` and preserves any earlier `actual_start`.

#### Always link commits before closing

Before `set_state done`, call `nottario.tasks.link_commit { repo,
sha }` once per commit the task produced. This is non-negotiable
whenever the work yielded code: the Commits panel in the UI, the
"what shipped here" queries and any traceability audit all depend on
the structured link, not on prose in a comment. The bar is: a future
reader of the closed task can jump straight to the diff without
grepping git. Tasks that are pure documentation or bug-recovery in
the DB legitimately have no commit; everything else does.

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
  nottario.tasks.add_comment { task_id: task.id, body: "..." }   # optional
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
slip in and claim the same task. `tasks.next` still exists but is now
a PREVIEW with no side effects — useful to inspect what `claim_next`
would pick, never to actually take a task.

If a task you want is already claimed by another user, the right
moves are: leave a comment with `nottario.tasks.add_comment`, or
escalate to the human. Do not silently re-assign.

### "I found a bug while doing my task"

Create it; do **not** silently fix it without filing:

```
nottario.tasks.create {
  project_id, title, type: "bug", priority: 70,
  description: "context, repro, suspected fix",
}
```

Then continue with your current task.

### "The user just mentioned a different task / bug / feature"

Same rule, broader scope: whenever the human (or any teammate) drops
a side-comment about work that is **not** what you're currently
executing — a "we should also…", a half-formed feature idea, a
visual bug they noticed, a recommendation about a future page — the
FIRST action is `nottario.tasks.create`. Only after the row exists
do you decide whether to keep going with your current task or pivot
to the new one.

The reasoning is identical for humans and for other agents reading
the backlog later: if the item only lives in conversation, it's
invisible. Multi-agent and multi-session work depend on the backlog
being the single source of truth. Doing the work without filing it
silently destroys that property.

What to capture in the description:
- Verbatim what the user said (the bug repro, the design hunch).
  Future-you will not remember the phrasing.
- The current state-of-the-code that triggered it (a file path,
  a screenshot reference, the URL of the broken view).
- The proposed direction if you have one — even if uncertain. Mark
  it "tentative" rather than omit it.
- The role split when the work obviously crosses backend / frontend
  / design / qa. Either file as a `type=feature` parent with role
  children, or as one task with a clear role-split note in the body.

When to pivot vs. keep going: cheap context-switch (≤5 min of work
to make the side comment visibly better, no architectural decision
required) → pivot, file, fix, return. Otherwise: file with enough
context that someone else can pick it up, and resume your current
task. The bar is "if I had to leave the session right now, would
someone else be able to pick this up?" — if no, add more context.

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
