package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Step limits. A step is one short line; anything longer, or a list
// longer than this, is a task that wants subtasks instead.
const (
	MaxStepLen      = 200
	MaxStepsPerTask = 50
)

// ErrStepNotFound is returned when a step id does not exist or belongs
// to a different task. Both read the same, like a task outside the
// caller's project.
var ErrStepNotFound = errors.New("step not found")

// Step is one item of a task's checklist: a short action that needs no
// owner, role, dependencies or commits of its own.
type Step struct {
	ID           uuid.UUID  `json:"id"`
	Position     int        `json:"position"`
	Text         string     `json:"text"`
	Done         bool       `json:"done"`
	DoneAt       *time.Time `json:"done_at,omitempty"`
	DoneByUserID *uuid.UUID `json:"done_by_user_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// StepProgress is how many of a task's steps are done.
type StepProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

func stepFromRow(r dbq.ListTaskStepsRow) Step {
	return Step{
		ID:           r.ID,
		Position:     int(r.Position),
		Text:         r.Text,
		Done:         r.Done,
		DoneAt:       timestampPtr(r.DoneAt),
		DoneByUserID: r.DoneByUserID,
		CreatedAt:    r.CreatedAt.Time,
	}
}

// normaliseStepText trims a step and checks it is one line of 1 to
// MaxStepLen characters.
func normaliseStepText(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", errInvalid("step text is empty")
	case strings.ContainsAny(s, "\r\n"):
		return "", errInvalid("step text must be a single line; add several steps instead")
	case len([]rune(s)) > MaxStepLen:
		return "", errInvalid(fmt.Sprintf("step text is longer than %d characters; a step that needs more is a subtask", MaxStepLen))
	}
	return s, nil
}

// ListSteps returns the steps of a task in order. Never nil.
func ListSteps(ctx context.Context, db dbq.DBTX, taskID uuid.UUID) ([]Step, error) {
	rows, err := dbq.New(db).ListTaskSteps(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]Step, 0, len(rows))
	for _, r := range rows {
		out = append(out, stepFromRow(r))
	}
	return out, nil
}

// PendingSteps returns the steps of a task that are not done, in order.
// Never nil.
func PendingSteps(ctx context.Context, db dbq.DBTX, taskID uuid.UUID) ([]Step, error) {
	all, err := ListSteps(ctx, db, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]Step, 0, len(all))
	for _, s := range all {
		if !s.Done {
			out = append(out, s)
		}
	}
	return out, nil
}

// withTaskLocked runs fn in a transaction holding the task row lock, so
// concurrent step edits on one task serialise and positions stay dense.
func withTaskLocked(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, fn func(tx pgx.Tx, q *dbq.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	if _, err := q.GetTaskForUpdate(ctx, taskID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := fn(tx, q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddSteps appends steps to the end of a task's list, in the order
// given, and returns the whole list. Every text is checked before
// anything is written, so a bad entry adds none of them.
func AddSteps(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, texts []string, by Authorship) ([]Step, error) {
	if len(texts) == 0 {
		return nil, errInvalid("no steps given")
	}
	clean := make([]string, 0, len(texts))
	for i, t := range texts {
		s, err := normaliseStepText(t)
		if err != nil {
			if len(texts) > 1 {
				return nil, errInvalid(fmt.Sprintf("step %d: %s", i+1, err))
			}
			return nil, err
		}
		clean = append(clean, s)
	}
	var out []Step
	err := withTaskLocked(ctx, pool, taskID, func(tx pgx.Tx, q *dbq.Queries) error {
		n, err := q.CountTaskSteps(ctx, taskID)
		if err != nil {
			return err
		}
		if int(n)+len(clean) > MaxStepsPerTask {
			return errInvalid(fmt.Sprintf("a task holds at most %d steps; this one has %d, so %d more fit", MaxStepsPerTask, n, MaxStepsPerTask-int(n)))
		}
		for i, s := range clean {
			if _, err := q.InsertTaskStep(ctx, dbq.InsertTaskStepParams{
				TaskID:           taskID,
				Position:         n + int32(i), //nolint:gosec // bounded by MaxStepsPerTask
				Text:             s,
				CreatedByUserID:  by.UserID,
				CreatedByTokenID: by.TokenID,
			}); err != nil {
				return err
			}
		}
		out, err = ListSteps(ctx, tx, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateStepParams names what to change; nil leaves it alone.
type UpdateStepParams struct {
	Text *string
	Done *bool
}

// UpdateStep edits a step's text, ticks or unticks it, or both.
func UpdateStep(ctx context.Context, pool *pgxpool.Pool, taskID, stepID uuid.UUID, p UpdateStepParams, by Authorship) (*Step, error) {
	if p.Text == nil && p.Done == nil {
		return nil, errInvalid("nothing to change: give text, done or both")
	}
	var text pgtype.Text
	if p.Text != nil {
		s, err := normaliseStepText(*p.Text)
		if err != nil {
			return nil, err
		}
		text = pgtype.Text{String: s, Valid: true}
	}
	var done pgtype.Bool
	if p.Done != nil {
		done = pgtype.Bool{Bool: *p.Done, Valid: true}
	}
	r, err := dbq.New(pool).UpdateTaskStep(ctx, dbq.UpdateTaskStepParams{
		Text:        text,
		Done:        done,
		ActorUserID: by.UserID,
		ID:          stepID,
		TaskID:      taskID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStepNotFound
	}
	if err != nil {
		return nil, err
	}
	s := stepFromRow(dbq.ListTaskStepsRow(r))
	return &s, nil
}

// RemoveStep deletes a step and closes the gap it leaves. Discarding a
// step is deleting it; steps have no other way out.
func RemoveStep(ctx context.Context, pool *pgxpool.Pool, taskID, stepID uuid.UUID) error {
	return withTaskLocked(ctx, pool, taskID, func(_ pgx.Tx, q *dbq.Queries) error {
		pos, err := q.DeleteTaskStep(ctx, dbq.DeleteTaskStepParams{ID: stepID, TaskID: taskID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStepNotFound
		}
		if err != nil {
			return err
		}
		return q.ShiftTaskSteps(ctx, dbq.ShiftTaskStepsParams{
			Delta: -1, TaskID: taskID, Lo: pos + 1, Hi: MaxStepsPerTask,
		})
	})
}

// MoveStep puts a step at position (0-based, clamped to the list) and
// returns the whole list in its new order.
func MoveStep(ctx context.Context, pool *pgxpool.Pool, taskID, stepID uuid.UUID, position int) ([]Step, error) {
	var out []Step
	err := withTaskLocked(ctx, pool, taskID, func(tx pgx.Tx, q *dbq.Queries) error {
		cur, err := q.GetTaskStep(ctx, dbq.GetTaskStepParams{ID: stepID, TaskID: taskID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStepNotFound
		}
		if err != nil {
			return err
		}
		n, err := q.CountTaskSteps(ctx, taskID)
		if err != nil {
			return err
		}
		to := int32(max(0, min(position, int(n)-1))) //nolint:gosec // clamped to the list length
		from := cur.Position
		switch {
		case to < from:
			err = q.ShiftTaskSteps(ctx, dbq.ShiftTaskStepsParams{Delta: 1, TaskID: taskID, Lo: to, Hi: from - 1})
		case to > from:
			err = q.ShiftTaskSteps(ctx, dbq.ShiftTaskStepsParams{Delta: -1, TaskID: taskID, Lo: from + 1, Hi: to})
		}
		if err != nil {
			return err
		}
		if to != from {
			if err := q.SetTaskStepPosition(ctx, dbq.SetTaskStepPositionParams{Position: to, ID: stepID}); err != nil {
				return err
			}
		}
		out, err = ListSteps(ctx, tx, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// enrichTaskStepProgress fills Steps on every task that has any, in one
// query for the whole slice.
func enrichTaskStepProgress(ctx context.Context, db dbq.DBTX, tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	rows, err := dbq.New(db).TaskStepProgress(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]*StepProgress, len(rows))
	for _, r := range rows {
		byID[r.TaskID] = &StepProgress{Done: int(r.Done), Total: int(r.Total)}
	}
	for _, t := range tasks {
		t.Steps = byID[t.ID]
	}
	return nil
}
