---
title: Gantt timeline
section: Views
nav_order: 1
---

# Gantt timeline

The Gantt view is the same backlog as the [Kanban](/kanban/),
arranged along time instead of by state. A **NOW** line splits the
stage into a PAST zone (finished work, oldest on the left) and a
FUTURE zone (todo and doing, leftmost is up next).

![Gantt-style timeline with PAST and FUTURE zones around a NOW line](/screenshots/gantt-view.png)

## How it lays out

Bars are coloured by state, sized by the task's duration when known
(actual start → actual end for past work; eligibility window for
future). Within each role lane, tasks order topologically by
dependency first, then by priority bucket. A `feature` parent
collapses its children into a single grouped bar with an unfold
control.

## Interaction

- Hover a bar: a card surfaces with the full title and the task's
  current state, dependencies and assignee.
- Click a bar: opens the detail dialog (the same dialog the Kanban
  uses).
- The `Now` button in the toolbar re-centres the NOW line if you
  scrolled away.

## What it is good for

- Spotting the critical path: which open task is blocking the most
  others.
- Picking up the next eligible work without re-reading the backlog.
- Reviewing what shipped in a sprint at a glance — the PAST zone
  reads as a stratigraphy of done work.

The Gantt and Kanban share the same data and the same realtime
channel, so edits propagate live across both.

## How an agent uses the Gantt

The Gantt is a humans-only visualisation; an agent never opens it.
But the layout reflects the same data the agent reaches through
the [MCP tasks domain](/skills/tasks/), so the same lens is
available programmatically:

- `nottario.tasks.list { project_id, cycle_id?, target_role_id? }`
  returns the same rows the Gantt lays out — type, priority,
  state, dependencies, target role, actual_start, actual_end.
  Order them by topological depth then priority to recover the
  Gantt's left-to-right ordering.
- `nottario.tasks.next { project_id, target_role_id? }` is the
  read-only equivalent of the Gantt's "what's coming up next" —
  it returns the eligible todo at the head of the FUTURE zone
  without claiming it. Use it to plan; use `claim_next` to act.
- After landing work, `nottario.tasks.link_commit` and
  `set_state done` move the bar from the FUTURE zone to the PAST
  zone (the actual_start / actual_end timestamps are stamped on
  the state transitions).

The PAST zone is a structured record of "what shipped, when": an
agent reviewing what was done in the last sprint can list
`{ state: 'done', cycle_id: <past sprint> }` and read the same
stratigraphy the human sees on the chart.
