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
