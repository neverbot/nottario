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

**Required order of operations.** Always assign the task to yourself
(your `whoami.user_id`) BEFORE moving it to `doing`. Without that,
the task sits in `doing` with no owner; humans cannot tell which
agent is working on what, and two agents may pick up the same task.

```
loop:
  me   = nottario.whoami { }
  task = nottario.tasks.next { project_id, assignee_user_id: me.user_id }
  if task is null: tell the human "no eligible tasks" and stop

  // 1) Take ownership before flipping state.
  nottario.tasks.update    { task_id: task.id, assignee_user_id: me.user_id }
  nottario.tasks.set_state { task_id: task.id, state: "doing" }

  ...do the work in the local repo...

  nottario.tasks.link_commit { task_id: task.id, repo, sha }
  nottario.tasks.add_comment { task_id: task.id, body: "..." }   // optional
  nottario.tasks.set_state   { task_id: task.id, state: "done" }
goto loop
```

If a task you want to pick up is already assigned to another user,
leave a comment before stealing it, or ask the human first.

### "I found a bug while doing my task"

Create it; do **not** silently fix it without filing:

```
nottario.tasks.create {
  project_id, title, type: "bug", priority: 70,
  description: "context, repro, suspected fix",
}
```

Then continue with your current task.

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
