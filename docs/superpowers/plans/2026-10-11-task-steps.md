# Task steps implementation plan

> For agentic workers: execute task by task, in order. Each task ends
> with `make check` green and one commit. Nottario tracking: feature
> `b27cbe47`, backend `cbcbd396` (tasks 1-6), UI `887f4f18` (task 7).

**Goal:** every task carries an ordered checklist of short steps (text +
done). Agents add steps as they discover work and tick them as they
go; humans and agents see progress as `2/8`.

**Why:** subtasks are too heavy for small actions (role, assignee,
state, dependencies, commits), and comments cannot say what is still
missing. Steps fill the gap between the two.

**Decisions (from the user, 2026-10-11):**

- A task can be closed with pending steps. The MCP response then lists
  them and tells the agent to inform the human which ones were left
  undone. The web lets it close too.
- Discarding a step means deleting it. Steps have exactly two states,
  done or not done. Any richer lifecycle belongs to tasks.
- The kanban card shows the progress (e.g. `2/8`) when a task has steps.
- Steps always come back with the full task (`tasks.get`, web detail);
  list rows and slim shapes carry only the counts.
- Every UI change is designed and reviewed with `/impeccable`.

**Limits:** step text 1-200 characters after trimming, single line
(newlines rejected); at most 50 steps per task. Both enforced in the
domain package and by table constraints where SQL can express them.

---

## Data model

`task_steps` is its own table, not an array on `tasks`: two agents can
tick different steps at once without rewriting each other's array, each
step has a stable id to address it by, and every change carries who
did it.

```sql
CREATE TABLE public.task_steps (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            uuid NOT NULL REFERENCES public.tasks(id) ON DELETE CASCADE,
    position           integer NOT NULL,
    text               text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 200),
    done               boolean NOT NULL DEFAULT false,
    done_at            timestamptz,
    done_by_user_id    uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_by_token_id uuid REFERENCES public.api_tokens(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT task_steps_done_at CHECK (done = (done_at IS NOT NULL))
);
CREATE INDEX task_steps_task_position_idx ON public.task_steps (task_id, position);
```

`position` is dense 0..n-1 per task, rewritten inside a transaction
with the task row locked (`GetTaskForUpdate`) on add, remove and move,
so concurrent edits serialise per task. `created_by_token_id` refers
to `public.api_tokens(id)` with `ON DELETE SET NULL`, like the tasks
table does.

Realtime: extend the existing `public.notify_event()` trigger function (see
`00002_task_comment_events.sql`) with a `task_steps` branch that, like
`task_comments`, resolves `project_id` from the owning task and emits
`{table, op, project_id, task_id}`; attach INSERT/UPDATE/DELETE
triggers. The board already refreshes on `task_comments` events for an
open task; steps reuse that path.

---

## Task 1: migration

**Files:** create `internal/db/migrations/0012_task_steps.sql` (goose
Up/Down; Down drops triggers, then table).

- [ ] Write the migration (table, index, constraint, trigger branch,
      three triggers). The trigger function is replaced with
      `CREATE OR REPLACE FUNCTION`; copy the current body from the
      latest migration that defines it and add the `task_steps` branch.
- [ ] Down restores the previous function body and drops the table.
- [ ] Verify only with `internal/testutil.NewPool` (fresh database);
      never touch the live dev DB.

## Task 2: sqlc queries

**Files:** create `internal/db/queries/steps.sql`; run `make sqlc`;
commit generated `internal/db/dbq/steps.sql.go`.

Queries (named args, casts as per claude.md conventions):

- `ListSteps :many` by task, ordered by position.
- `CountSteps :one` by task (for the 50 cap).
- `InsertStep :one` (task_id, position, text, created_by_*).
- `UpdateStepText :one`, `SetStepDone :one` (sets done, done_at,
  done_by_user_id, updated_at; clears done_at / done_by when undone).
- `DeleteStep :one` returning position.
- `ShiftStepPositions :exec` (close the gap after delete / open one on
  move), `SetStepPosition :exec`.
- `GetStep :one` (id, task_id) for ownership checks.
- `StepProgressForTasks :many` with `task_id = ANY(sqlc.arg('ids')::uuid[])`
  returning `task_id, count(*) FILTER (WHERE done) AS done, count(*) AS total`.
- `ListPendingSteps :many` (not done, by position) for the close notice.

## Task 3: domain package

**Files:** create `internal/tasks/steps.go` and
`internal/tasks/steps_integration_test.go`; modify `types.go`,
`enrich.go`, `repo.go` (list paths), `close.go`.

```go
type Step struct {
    ID        uuid.UUID  `json:"id"`
    Position  int        `json:"position"`
    Text      string     `json:"text"`
    Done      bool       `json:"done"`
    DoneAt    *time.Time `json:"done_at,omitempty"`
    DoneBy    *uuid.UUID `json:"done_by_user_id,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}

type StepProgress struct {
    Done  int `json:"done"`
    Total int `json:"total"`
}
```

- `ListSteps(ctx, db, taskID) ([]Step, error)`.
- `AddSteps(ctx, pool, taskID, texts []string, by Authorship) ([]Step, error)`:
  validates every text (trim, 1-200, no newline) before writing; one
  transaction, task row locked; rejects when the total would exceed 50
  with an error that says how many slots are left.
- `UpdateStep(ctx, pool, taskID, stepID, text *string, done *bool, by)`.
- `RemoveStep(ctx, pool, taskID, stepID)` closes the position gap.
- `MoveStep(ctx, pool, taskID, stepID, position int)` clamps to 0..n-1.
- Every function checks that the step belongs to `taskID` (a step id
  from another task answers "step not found"), mirroring
  `RequireInProject` for tasks.
- `Task` gains `Steps *StepProgress json:"steps,omitempty"`, filled by
  a batched `enrichTaskStepProgress` next to `enrichTaskViaMCP` for
  every list/get path, so the board and `tasks.list` get counts in one
  extra query per page. Tasks without steps leave it nil.
- `CloseResult` gains `PendingSteps []Step`, read inside the close
  transaction after the state change. `SetState` to `done` returns the
  same through whatever result struct it exposes (add one if needed).

Tests (real DB via `testutil.NewPool`): add/list order; cap at 50 with
remaining-slots message; text validation; tick sets done_at/done_by and
untick clears them; remove and move keep positions dense; a step of
task A cannot be touched through task B; progress counts on list rows;
close with pending steps returns them and still closes; concurrent
`AddSteps` on one task keeps positions unique (two goroutines).

## Task 4: web API

**Files:** create `internal/web/api_task_steps.go` and
`internal/web/api_task_steps_test.go`; modify `server.go`,
`api_tasks.go`.

Routes (all through the existing `guard`, which already runs
`withProjectSlug`, the scope guard and `withTaskInProject`):

- `POST   /api/projects/{id}/tasks/{task_id}/steps` body `{texts: [...]}` → 201 `{steps}` (the full list after the add).
- `PATCH  /api/projects/{id}/tasks/{task_id}/steps/{step_id}` body `{text?, done?}` → 200 step.
- `DELETE /api/projects/{id}/tasks/{task_id}/steps/{step_id}` → 204.
- `PUT    /api/projects/{id}/tasks/{task_id}/steps/{step_id}/position` body `{position}` → 200 `{steps}`.

Handlers call `requireProjectAccess` themselves (defence in depth, as
the other task handlers do). The task detail response gains `steps`
(full list). Add the four routes to the route-coverage battery
(`project_scope_all_routes_test.go`, `project_slug_routes_test.go`,
cross-project test) so the scope and slug suites cover them.

## Task 5: MCP tools

**Files:** modify `internal/mcp/tools_tasks.go` (or a new
`tools_steps.go`); tests in `internal/web/mcp_task_steps_test.go`.

- `nottario.tasks.add_steps {project_id, task_id, texts[]}` → `{steps}`.
- `nottario.tasks.update_step {project_id, task_id, step_id, text?, done?}` → step.
- `nottario.tasks.remove_step {project_id, task_id, step_id}` → `{removed: true}`.
- `nottario.tasks.move_step {project_id, task_id, step_id, position}` → `{steps}`.
- `nottario.tasks.get` always returns `steps` (like `related`).
- `tasks.list` rows and the slim task shape carry `steps: {done, total}`
  when the task has steps.
- `nottario.tasks.close` and `set_state` to `done`: when steps are
  pending, the response adds `pending_steps` (id + text) and
  `notice`: "This task was closed with N pending steps. Tell the human
  which steps were left undone." The task is closed regardless.
- All tools call `requireProjectAccess` and `tasks.RequireInProject`
  like the other task tools.

Tests: each tool's happy path, a foreign step id, the cap, the close
notice present with pending steps and absent without them.

## Task 6: skill, docs, changelog

**Files:** `internal/skill/files/domains/tasks.md`,
`internal/skill/files/skill.md`, `docs/site/content/mcp.md`,
`docs/site/content/whats-new.md`, `.claude/claude.md` if it gains a
rule (then sync `context/claude.md` in Nottario).

Skill text to cover, plainly and with reasons:

- When to use a step: an action worth remembering that needs no owner,
  role, dependency, commits or discussion of its own. If it needs any
  of those, it is a subtask.
- When starting a task, write its steps if the work has more than one
  distinct action. When new work appears mid-task, add a step instead
  of a long comment.
- Tick each step when it is done, not all at the end; keep step text
  short and imperative.
- Before closing, check the steps. Closing with pending steps is
  allowed, but tell the human which ones were left undone (the close
  response says so too). Delete steps that turned out to be unneeded
  rather than leaving them pending.

Bundle version changes; agents get the update notice automatically.

## Task 7: UI (with /impeccable)

**Files:** `internal/web/static/pages/board.js` (dialog section and
card), possibly a new `internal/web/static/components/task-steps.js`.

Run `/impeccable` first (product register, GitHub-like invariants
from claude.md) to shape the design, then implement and have it review
the result. Scope:

- Dialog section "Steps · n of m": list with checkboxes, add input
  (Enter adds, paste of several lines adds several), inline edit,
  delete, reorder (keyboard accessible). Empty state invites the first
  step without taking space when there are none.
- Kanban card: progress `2/8` when the task has steps.
- Optimistic tick with rollback on error; live refresh from the
  realtime `task_steps` events.
- Respect `prefers-reduced-motion`; explicit `box-sizing` in any new
  shadow root.
- Rebuild the container (`docker compose up -d --build nottario`) and
  verify on `http://localhost:8080` with real tasks.

## Done when

- `make check` green after every task; each task committed separately.
- Smoke on localhost (UI) and, after deploy, on the remote MCP:
  add steps, tick, close with pending steps and see the notice.
- Commits linked to `cbcbd396` / `887f4f18`; feature rolls up.
