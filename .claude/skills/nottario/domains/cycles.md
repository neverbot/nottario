---
name: nottario-domain-cycles
description: Reference for the Nottario cycles (sprint/iteration) domain — what a cycle is, how to navigate it, how the end-cycle transition works.
---

# Cycles domain — full reference

A cycle is a named bucket of tasks that ship together. Every task —
feature or leaf — belongs to exactly one cycle (`cycle_id NOT NULL`).
A project always has exactly one active (unclosed) cycle. Closing it
opens the next one and moves in-flight work forward; finished work
stays stamped with the cycle it shipped in.

## Tools

### nottario.cycles.list

Returns every cycle of a project, newest first. Closed cycles carry
`closed_at` + `closed_by_user_id`.

### nottario.cycles.current

Returns the active cycle. Equivalent to `list[0]` when the latest is
unclosed, but cheaper.

### nottario.cycles.get

Read a specific cycle by id.

### nottario.cycles.end

Close the active cycle and open the next. **Owner-gated**. Atomic:

1. Closes the active cycle (`closed_at = now()`, `closed_by_*` set).
2. Opens a new cycle (`<label>-<N+1>` by default, or `next_name` if
   provided).
3. Moves every partial feature subtree (a feature whose rollup state
   isn't `done`) — including its done children, which get re-stamped
   to the new cycle.
4. Moves standalone non-done tasks to the new cycle.
5. Done tasks not under a partial feature stay in the closed cycle.

Returns `{ closed, next }`.

## Carry-on pattern (with cycles)

`tasks.claim_next` without an explicit `cycle_id` operates on the
**active** cycle. Same for `tasks.next` and `tasks.list`. Pass
`cycle_id` to inspect a closed cycle.

## Idiomatic patterns

### "What shipped in sprint-2?"

```text
nottario.tasks.list { project_id, cycle_id: <sprint-2's id>, state: 'done' }
```

### "Move this work to next sprint"

Use `tasks.update` to change `cycle_id`. If the task is a leaf with a
feature parent, the cascade trigger overrides your value with the
parent's cycle_id. Move the whole feature instead.

### "End this sprint"

`nottario.cycles.end { project_id, next_name? }`. Requires owner.

## Token discipline

Cycles is a small domain — most of its discipline comes from how
**other** tools default to the active cycle.

**Don't pass `cycle_id` when you mean "current".** `tasks.list`,
`tasks.next`, `tasks.claim_next` and `tasks.list_priorities` all
default to the project's active cycle when `cycle_id` is omitted.
Sending it explicitly costs you ~50 tokens of uuid + the round-trip
to `cycles.current` you used to look it up. Just omit it.

**`cycles.current` once per session, max.** The active cycle changes
at most once per sprint cadence (weeks). Cache its id locally if you
need to mention it in prose; do not re-fetch each turn.

**Pass `cycle_id: "all"` deliberately.** It disables the active-cycle
default and lets `tasks.list` walk every cycle the project ever had.
Useful for "find this task somewhere in the history" but expensive on
long-lived projects — always combine with `state` or `assignee_user_id`
to keep the result set small.

**`cycles.end` is high-impact and rare.** One call closes the cycle,
moves in-flight work and opens the next. The response carries the
`{closed, next}` pair — that's a single tool call's worth of cost for
something that happens weekly. No special discipline needed beyond
"don't call it speculatively".
