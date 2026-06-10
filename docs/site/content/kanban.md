---
title: Kanban board
section: Views
nav_order: 0
---

# Kanban board

The Kanban is the primary picker for what to work on next. Three
columns — **todo**, **doing**, **done** — show every task in the
active cycle. Cards carry the task type (`task` / `bug` / `feature`),
the priority bucket and the target role. When a task is assigned,
the owner's avatar sits in the bottom-right corner so you can see at
a glance who is holding what.

![Kanban board with three columns, tagged cards and assignee avatars](/screenshots/kanban-board.png)

## Day-to-day

- A click on a card opens the detail dialog with description,
  dependencies, linked commits and comments.
- Drag a card across columns to change its state. The same
  transitions are reachable from the dialog or via
  `nottario.tasks.set_state` over MCP.
- The "Up next" affordance surfaces the highest-priority eligible
  todo when the **doing** column is empty, so you don't have to
  scan the backlog manually. Agents reach the same row atomically
  via `nottario.tasks.claim_next`.

## Sprints

The header shows the current cycle name and progress counters
(`N done · M doing · K todo`). The **End sprint** button at the
top right atomically closes the active cycle and rolls every open
task onto the next sprint; tasks already done stay stamped on the
sprint they shipped in.

The cycle dropdown to the left lets you view a past sprint without
leaving the page; the view narrows to that sprint's snapshot.

## Realtime

Both kanban and gantt are wired to `LISTEN/NOTIFY` over SSE: when a
human or an agent edits a task elsewhere, the card updates in
place within a second. No reload needed. Comment edits in the open
detail dialog refresh the same way.

## How an agent drives the board

The kanban is the human-facing surface; the agent works the same
backlog through the [MCP tasks domain](/skills/tasks/). A typical
loop:

1. `nottario.tasks.claim_next { project_id, target_role_id? }` —
   atomically picks the highest-priority eligible todo and stamps
   `state=doing` + `assignee_user_id=<caller>` in a single query.
   The card visibly moves into the **doing** column on every
   connected browser within a second.
2. The agent does the work, then for each commit it produced calls
   `nottario.tasks.link_commit { task_id, repo, sha }` so the
   Commits panel of the detail dialog can render the diff link.
3. `nottario.tasks.add_comment { task_id, body }` captures a
   short closing note for the human reading the row later.
4. `nottario.tasks.set_state { task_id, state: "done" }` — the
   card moves to **done**, dependents become eligible, and a
   feature parent rolls up to done automatically when all its
   children are done.

Filing new work as the agent discovers it (a side bug spotted, a
follow-up task) is `nottario.tasks.create` with the right
`target_role_id` so it shows up in the right swimlane next time a
matching agent runs `claim_next`.
