---
title: Sprint / cycle / iteration buckets
date: 2026-05-26
status: design — pending implementation plan
parent_task: ffcc42e1-39f0-488e-b577-d9f6fad375e3
---

# Sprint / cycle / iteration buckets

A lightweight execution-tracking concept that groups tasks into named iterations
**without** introducing calendar dates. A cycle is a labeled bucket of tasks;
closing one rolls in-flight work forward to the next one. Replaces the implicit
"flat backlog" view with an explicit batch model.

## Why

Today every task in a project lives in a flat backlog. There's no notion of
"what shipped this sprint" or "what we're committing to this iteration". For
multi-person / multi-agent projects this becomes painful:

- Done tasks pile up and you can't slice the history into "what shipped when".
- The Kanban "doing" column conflates "started today" with "started two months
  ago and never finished".
- "Carry-on" semantics for an agent are ambiguous: claim ANY todo, or just the
  ones the team committed to?

A cycle gives the team a shared "batch we're working on right now". When you
close one, in-progress work moves to the next cycle automatically; finished
work stays stamped with the cycle it shipped in.

## Locked decisions (brainstorm 2026-05-26)

| # | Decision |
|---|---|
| 1 | **Cycle membership is mandatory**. Every task — feature or leaf — has a `cycle_id NOT NULL`. There's no "floating" state. |
| 2 | **Features cascade their cycle to children**. A feature and its descendants always share a cycle. Cascading is enforced at the DB level (trigger). |
| 3 | **Partial features on close move whole**. When closing a cycle, any feature whose rollup state isn't `done` carries its **entire subtree** (including done children, re-stamped) to the next cycle. |
| 4 | **One-click close**. The "End sprint" action atomically creates the next cycle and moves the affected tasks. |
| 5 | **New `project-owner` role**. Owner can: change settings, manage memberships, close cycles. Backfilled to `created_by_user_id`. Instance admins always have owner powers. |
| 6 | **Cycles are irreversible**. Once `closed_at` is set, the cycle is read-only. Mistakes are corrected by manually moving tasks. |
| 7 | **Auto-numeric naming with per-project label**. Default label = `sprint`. Default name = `<label>-N`. Both are editable. |
| 8 | **Bootstrap**: migration creates one `sprint-1` per project and assigns every existing task to it. |
| 9 | **UI surface**: switcher + "End sprint" button live in the Kanban and Gantt page headers. No dedicated settings tab; just a `cycle_label` field in Settings → General. |
| 10 | **MCP default**: `tasks.list` / `tasks.next` / `tasks.claim_next` without `cycle_id` return only the active cycle. Pre-alpha breaking change is acceptable. |
| 11 | **Reparent forces the cycle**. Moving a task between features re-stamps its `cycle_id` to the new parent's (cascade invariant). |

## Architecture

Five-layer cut, scoped per package boundary so each unit has a single concern:

1. **DB schema + trigger** — `cycles` table, `tasks.cycle_id`, `projects.cycle_label`,
   `projects.owner_user_id`, cascade trigger. Migration `00014_cycles.sql`.
2. **`internal/cycles` package (new)** — repo + domain ops (list, get, current, end).
   Owns the close-cycle transaction.
3. **`internal/identity` extensions** — owner-gate helper + `SetProjectOwner`.
4. **REST + MCP wiring** — `internal/web/api_cycles.go`, `internal/mcp/tools_cycles.go`,
   plus the `cycle_id` filter additions to existing tasks tools.
5. **Frontend** — switcher + diálogo de End sprint + label config + owner picker.
   Touches `board.js`, `gantt.js`, `project-settings.js`.

Each layer is testable in isolation; the trigger keeps the cascade invariant
true even if some future caller forgets to set `cycle_id` correctly.

## Schema

### `cycles` table

```sql
CREATE TABLE cycles (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name               text NOT NULL,
    position           int  NOT NULL,
    opened_at          timestamptz NOT NULL DEFAULT now(),
    closed_at          timestamptz,
    closed_by_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    closed_by_token_id uuid REFERENCES api_tokens(id) ON DELETE SET NULL,
    UNIQUE (project_id, position),
    UNIQUE (project_id, name)
);
-- At most one active (closed_at IS NULL) cycle per project.
CREATE UNIQUE INDEX cycles_one_active_per_project
    ON cycles(project_id) WHERE closed_at IS NULL;
```

### `tasks` changes

```sql
ALTER TABLE tasks
    ADD COLUMN cycle_id uuid REFERENCES cycles(id) ON DELETE RESTRICT;
-- Backfill, then:
ALTER TABLE tasks ALTER COLUMN cycle_id SET NOT NULL;
CREATE INDEX tasks_cycle_id_idx ON tasks(cycle_id);
```

### `projects` changes

```sql
ALTER TABLE projects
    ADD COLUMN cycle_label   text NOT NULL DEFAULT 'sprint',
    ADD COLUMN owner_user_id uuid REFERENCES users(id);
-- Backfill owner = created_by_user_id, then:
ALTER TABLE projects ALTER COLUMN owner_user_id SET NOT NULL;
```

### Cascade trigger

```sql
CREATE FUNCTION tasks_enforce_cycle_cascade() RETURNS trigger AS $$
BEGIN
    IF NEW.parent_task_id IS NOT NULL THEN
        NEW.cycle_id := (SELECT cycle_id FROM tasks WHERE id = NEW.parent_task_id);
        IF NEW.cycle_id IS NULL THEN
            RAISE EXCEPTION 'parent task % has no cycle_id', NEW.parent_task_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tasks_cycle_cascade
    BEFORE INSERT OR UPDATE OF parent_task_id, cycle_id ON tasks
    FOR EACH ROW EXECUTE FUNCTION tasks_enforce_cycle_cascade();
```

The trigger guarantees the cascade invariant — every leaf shares a cycle with
its feature parent — regardless of which code path mutates the task.

## Domain operations (`internal/cycles`)

```go
type Cycle struct {
    ID         uuid.UUID
    ProjectID  uuid.UUID
    Name       string
    Position   int
    OpenedAt   time.Time
    ClosedAt   *time.Time
    ClosedBy   Authorship   // user_id + token_id (token nil for sessions)
}

// Read-only.
ListCycles(ctx, pool, projectID) ([]Cycle, error)
GetCycle(ctx, pool, cycleID) (*Cycle, error)
ActiveCycle(ctx, pool, projectID) (*Cycle, error)

// Mutation. Atomic: closes the active cycle AND creates the next AND moves
// tasks, all in one transaction guarded by an advisory lock per project.
type EndCycleParams struct {
    ProjectID uuid.UUID
    NextName  string   // optional; defaults to "<label>-N+1"
}
type EndCycleResult struct {
    Closed *Cycle
    Next   *Cycle
}
EndCycle(ctx, pool, p EndCycleParams, by Authorship) (*EndCycleResult, error)
```

### `EndCycle` algorithm

In one transaction:

1. `SELECT pg_advisory_xact_lock(hash('cycles', project_id))` — serializes any
   other close attempt against the same project.
2. `SELECT … FROM cycles WHERE project_id = $1 AND closed_at IS NULL FOR UPDATE`
   — fetch + lock the active cycle. If none, error.
3. `INSERT INTO cycles (project_id, name, position) VALUES (project_id,
   '<label>-N+1' OR p.NextName, closing.position + 1)` — open the next cycle.
4. **Move partial features**: identify features where
   `cycle_id = closing.id AND state != 'done'`. For each, compute the
   recursive descendant closure with `WITH RECURSIVE` and
   `UPDATE tasks SET cycle_id = next.id WHERE id IN closure`.
5. **Move standalone non-done tasks**: tasks with `cycle_id = closing.id AND
   state != 'done' AND parent_task_id IS NULL` (or whose parent is not in the
   set being moved by step 4) → update to `next.id`.
6. `UPDATE cycles SET closed_at = now(), closed_by_user_id = $by.user_id,
   closed_by_token_id = $by.token_id WHERE id = closing.id`.
7. Emit SSE events: `cycle.closed { project_id, cycle_id: closing.id }` and
   `cycle.created { project_id, cycle_id: next.id }`. Per-task `task.updated`
   events fire from the existing tasks trigger.

### Owner gate

In `internal/identity`:

```go
func RequireProjectOwner(ctx, pool, projectID, callerUserID, isAdmin) error
func SetProjectOwner(ctx, pool, projectID, newOwnerID) error  // admin-only caller
```

`RequireProjectOwner` returns nil when caller is admin OR
`projects.owner_user_id = callerUserID`; otherwise an error.

## REST + MCP surface

### New MCP tools

| Tool | Args | Gating |
|---|---|---|
| `nottario.cycles.list` | `project_id` | project access |
| `nottario.cycles.get` | `project_id, cycle_id` | project access |
| `nottario.cycles.current` | `project_id` | project access |
| `nottario.cycles.end` | `project_id, next_name?` | **owner** |
| `nottario.projects.set_owner` | `project_id, owner_user_id` | **instance admin** |

### Changes to existing MCP tools

- `tasks.list` / `tasks.next` / `tasks.claim_next` / `tasks.create`: add an
  optional `cycle_id` argument. Default for the read tools = the project's
  active cycle. Default for create = active (overridden by parent's cycle via
  trigger when `parent_task_id` is set).
- `projects.get`: response includes `OwnerUserID` and `CycleLabel`.

### New REST endpoints

```
GET   /api/projects/{id}/cycles
GET   /api/projects/{id}/cycles/{cycle_id}
GET   /api/projects/{id}/cycles/current
POST  /api/projects/{id}/cycles/end          body: { next_name? }   (owner-gated)
PATCH /api/projects/{id}/owner               body: { owner_user_id } (admin)
```

Existing task endpoints accept `cycle_id` as a query param (list/next) or body
field (create), matching the MCP behavior.

### SSE events

Extend `internal/realtime.Event`:
- `cycle.created { project_id, cycle_id }`
- `cycle.closed  { project_id, cycle_id }`

Existing `task.updated` continues to fire once per moved task during a close.

### Skill bundle

- New file `internal/skill/files/domains/cycles.md` describing the model, the
  end-cycle semantics, and the agent patterns.
- Update `domains/tasks.md`: mention `cycle_id` filter on list/next, default
  behavior (active), and the cascade-on-reparent rule.

## Frontend

### Kanban / Gantt page headers

A cluster left of the existing "New task" button:

```
[ sprint-3 ▾ ]  3/12 done · 5 doing · 4 todo       [ End sprint ]  [ New task ]
```

- **Switcher pill** `sprint-3 ▾`: opens a dropdown listing cycles by `position`
  desc — the active first (tagged "active"), then closed ones with their
  `closed_at` formatted. Selecting another cycle updates `?cycle=<id>` in the
  URL hash and the views re-fetch.
- **Inline summary**: state counts of the currently-visible cycle.
- **"End sprint" button**: only shown when viewing the active cycle AND caller
  is owner or admin. Opens the confirmation dialog.
- When viewing a **closed** cycle: header reads `sprint-2 (closed · 5d ago)`,
  no End/New buttons. Cards/bars are decorated subtly to read as read-only
  (existing state-based styling already does this).

### End-sprint dialog

```
┌───────────────────────────────────────────────────────┐
│ End sprint-3                                     [X]  │
├───────────────────────────────────────────────────────┤
│ Next sprint name:                                     │
│ [ sprint-4               ]                            │
│                                                       │
│ This will:                                            │
│ • Close sprint-3 (irreversible).                      │
│ • Move 5 doing + 4 todo tasks to sprint-4.            │
│ • Re-stamp 3 partial features (incl. 7 done           │
│   children) to sprint-4.                              │
│ • Leave 5 standalone done tasks in sprint-3.          │
│                                                       │
│ [ Cancel ]                          [ End sprint-3 ]  │
└───────────────────────────────────────────────────────┘
```

The four counts are computed live by the frontend from `this.tasks` before the
dialog opens. The primary button is destructive-styled (red) reflecting the
irreversibility.

### URL routing

Hash routing — consistent with the existing `#task=`, `#path=`, `#node=`
patterns: `#cycle=<cycle_id>`. Absent hash = active cycle. Deep-link to a
closed cycle works the same as any other.

### Settings → General

New field `Cycle label` below `Default view`: text input, defaults `sprint`.
Owner-only edit. The configured label drives every visible string ("End {label}",
"{label}-N", "Move to {label}-N+1"...).

### Settings → Members

Owner-picker control: dropdown of project members. Visible to admins; disabled
for non-admins. PATCH `/api/projects/{id}/owner`.

### Reactivity to SSE

`cycle.created` and `cycle.closed` events trigger a reload of the page's tasks
+ a refresh of the switcher dropdown. If the user happens to be viewing the
cycle that just closed, the URL hash flips to the new active cycle (with a
toast: "sprint-3 closed; viewing sprint-4 now") — alternative: keep them on
the closed view (now read-only). Decision: **keep them on the closed view**;
they explicitly clicked End and the dialog already told them what was about
to happen.

## Testing

### Backend integration tests (`internal/cycles/`)

- `TestEndCycle_BasicMove` — done stays in closing, todo/doing move to next, closed_at set.
- `TestEndCycle_CascadesPartialFeature` — feature with 2 done + 1 doing, all three children + the feature land in next.
- `TestEndCycle_LeavesFullyDoneFeatureAlone` — feature with all-done children stays.
- `TestEndCycle_OwnerGate` — non-owner caller blocked; admin OK; owner OK.
- `TestEndCycle_ConcurrentCallsSerialize` — two simultaneous close attempts → exactly one closes, the loser sees "no active cycle to close" or equivalent.

### Cascade trigger tests (`internal/tasks/`)

- `TestCascade_ReparentForcesCycle` — task in sprint-1 reparented under feature in sprint-2 → cycle_id becomes sprint-2.
- `TestCascade_CycleIdIgnoredWhenParented` — INSERT with explicit cycle_id ≠ parent's → trigger overrides.
- `TestCascade_NoParentLeavesCycleAlone` — INSERT with no parent respects supplied cycle_id.

### Project bootstrap tests

- Update `TestCreateProject_SeedsRolesAndPriorities` to also assert: a `sprint-1` cycle is created, every newly-created task lands in it, `projects.owner_user_id = creator`.

### MCP integration tests (`internal/web/mcp_*_integration_test.go`)

- `TestMCP_Cycles_LifecycleAndFilters` — list/current/end via MCP + tasks.list with cycle_id filter on a closed cycle.
- `TestMCP_Tasks_DefaultsToActiveCycle` — `tasks.list` without cycle_id returns active-only.
- `TestMCP_Cycles_NonOwnerCannotEnd` — outsider fixture, `cycles.end` fails.

### REST tests

- `TestApiCycles_GetCurrent`, `TestApiCycles_End` (success path), `TestApiCycles_End_NonOwner`.
- `TestApiOwner_AdminOnly` for `PATCH /owner`.

### Frontend smoke (manual)

- New project → header shows `sprint-1 (active)`.
- Create 5 tasks (1 done, 2 doing, 2 todo), pulse "End sprint" → dialog with correct counts → confirm → `sprint-2` active with the 4 non-done, `sprint-1` in dropdown as "closed".
- Switch to closed cycle → no "End" / "New" buttons, only the done task visible.
- Create a feature with 3 children (2 done, 1 doing), close → all 3 children + the feature in `sprint-2`.
- Settings → change `cycle_label` to "iteration" → header reads "iteration-2", End button reads "End iteration-2".

## Rollout / migration

Migration `00014_cycles.sql` (single, idempotent, transactional):

1. `CREATE TABLE cycles` + the `cycles_one_active_per_project` unique partial index.
2. `ALTER TABLE projects ADD COLUMN cycle_label text NOT NULL DEFAULT 'sprint'`.
3. `ALTER TABLE projects ADD COLUMN owner_user_id uuid REFERENCES users(id)`.
4. `UPDATE projects SET owner_user_id = created_by_user_id WHERE created_by_user_id IS NOT NULL`.
5. For projects with NULL `created_by_user_id` (shouldn't happen but defensive): fall back to the first admin user, or fail loudly.
6. `ALTER TABLE projects ALTER COLUMN owner_user_id SET NOT NULL`.
7. `INSERT INTO cycles (project_id, name, position) SELECT id, 'sprint-1', 1 FROM projects`.
8. `ALTER TABLE tasks ADD COLUMN cycle_id uuid REFERENCES cycles(id)`.
9. `UPDATE tasks t SET cycle_id = c.id FROM cycles c WHERE c.project_id = t.project_id AND c.position = 1`.
10. `ALTER TABLE tasks ALTER COLUMN cycle_id SET NOT NULL`.
11. `CREATE INDEX tasks_cycle_id_idx ON tasks(cycle_id)`.
12. Create cascade function + trigger.

### Goose Down

Drop trigger + function, drop columns, drop table. Acceptable destruction
only in development (would lose cycle history on a production rollback;
out-of-scope for pre-alpha).

### Generated code

New `internal/db/queries/cycles.sql` with: ListCycles, GetCycle,
GetActiveCycle, InsertCycle, CloseCycle, MoveTasksByCycle, MoveFeatureSubtree
(RECURSIVE). `make sqlc` regenerates dbq.

### Coordination notes

- The MCP `tasks.list` default change ("now returns active only") is a
  breaking change to integrations external to this repo. Pre-alpha
  acceptable; the skill bundle update lands in the same PR so any agent
  reading the skill on first connect picks up the new semantics.
- Frontend pages that fetch tasks (`board.js`, `gantt.js`) must include
  `cycle_id` in the fetch (active by default). This is coupled with the
  backend change and lands together.

## Out of scope (filable as follow-ups after this lands)

- Per-cycle metrics: burn-down, velocity, throughput.
- Drag tasks between cycles from the Kanban switcher.
- Cycle templates ("inherit these labels / default assignees from the previous
  cycle").
- Cycle goal / description field.
- Cycle-aware Gantt past-zone slicing (the current "ordered by actual_end"
  view stays; future iteration may add a "show only past tasks from this
  cycle" toggle).
- Cycle-bound MCP "subscribe to events" tool.

## Open questions deferred

None — all 8 questions from `ffcc42e1` are resolved by the locked decisions
above. The follow-up work surfaces (burn-down, drag-between, templates) are
acknowledged as out-of-scope, not unresolved.
