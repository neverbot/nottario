// Package notifications produces per-user notification rows on the
// three task events documented in `context/notifications` and fans
// them out through the realtime hub so open browser sessions can
// refresh their unread count without polling.
//
// The Notifier is a thin coordinator: the domain code (tasks.Update,
// tasks.SetState, tasks.AddComment) stays free of notification
// concerns; each of their callers (HTTP handlers, MCP tool handlers)
// invokes the matching Notifier method AFTER the write commits. That
// keeps the write path fast, keeps the domain layer clean, and lets
// notification failures log-and-continue without rolling back the
// user-visible change.
//
// When Enabled is false the Notifier turns every method into a no-op
// so callers don't need to guard the call site — the config flag
// lives in one place.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/tasks"
)

// Kind names match the JSON keys of users.notification_preferences.
const (
	KindTaskAssigned  = "task_assigned"
	KindTaskCommented = "task_commented"
	KindTaskClosed    = "task_closed"
)

// Notifier writes notification rows and pings the realtime hub.
// Zero value is unusable; construct with New.
type Notifier struct {
	pool    *pgxpool.Pool
	hub     *realtime.Hub
	logger  *slog.Logger
	enabled bool
}

// New constructs a Notifier. When enabled is false every method is a
// no-op (helper for the NOTIFICATIONS_ENABLED kill switch).
func New(pool *pgxpool.Pool, hub *realtime.Hub, logger *slog.Logger, enabled bool) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{pool: pool, hub: hub, logger: logger, enabled: enabled}
}

// Enabled reports whether the Notifier will emit anything.
func (n *Notifier) Enabled() bool { return n != nil && n.enabled }

// OnAssigneeChanged emits a task_assigned notification for the new
// assignee (never for the old one, never for the actor themselves).
// old is what the task looked like BEFORE the write; task is the
// row AFTER the write. When old.AssigneeUserID equals
// task.AssigneeUserID this is a no-op.
func (n *Notifier) OnAssigneeChanged(ctx context.Context, prev, task *tasks.Task, actorUserID *uuid.UUID) {
	if !n.Enabled() || task == nil {
		return
	}
	newAssignee := task.AssigneeUserID
	oldAssignee := (*uuid.UUID)(nil)
	if prev != nil {
		oldAssignee = prev.AssigneeUserID
	}
	if uuidEq(newAssignee, oldAssignee) {
		return
	}
	if newAssignee == nil {
		return
	}
	if uuidEq(newAssignee, actorUserID) {
		return
	}
	body := fmt.Sprintf("You were assigned to %q", task.Title)
	n.emit(ctx, task, KindTaskAssigned, actorUserID, []uuid.UUID{*newAssignee}, body)
}

// OnComment emits a task_commented notification to the recipient set
// (assignee ∪ creator) minus the actor. The set is deduplicated so
// an assignee-is-creator scenario only produces one row.
func (n *Notifier) OnComment(ctx context.Context, task *tasks.Task, actorUserID *uuid.UUID) {
	if !n.Enabled() || task == nil {
		return
	}
	recipients := stakeholders(task, actorUserID)
	if len(recipients) == 0 {
		return
	}
	body := fmt.Sprintf("New comment on %q", task.Title)
	n.emit(ctx, task, KindTaskCommented, actorUserID, recipients, body)
}

// OnStateChanged emits a task_closed notification when the new state
// is done or wont_do (todo↔doing transitions do not notify). Fires
// only for the stakeholders minus the actor. prev is the state
// BEFORE the write; skipped if the state didn't actually change.
func (n *Notifier) OnStateChanged(ctx context.Context, task *tasks.Task, prevState tasks.State, actorUserID *uuid.UUID) {
	if !n.Enabled() || task == nil {
		return
	}
	if task.State == prevState {
		return
	}
	if task.State != tasks.StateDone && task.State != tasks.StateWontDo {
		return
	}
	recipients := stakeholders(task, actorUserID)
	if len(recipients) == 0 {
		return
	}
	verb := "closed"
	if task.State == tasks.StateWontDo {
		verb = "marked won't do"
	}
	body := fmt.Sprintf("%q was %s", task.Title, verb)
	n.emit(ctx, task, KindTaskClosed, actorUserID, recipients, body)
}

// stakeholders returns the union of assignee and creator, dedup'd
// and stripped of the actor.
func stakeholders(t *tasks.Task, actor *uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := []uuid.UUID{}
	add := func(id *uuid.UUID) {
		if id == nil {
			return
		}
		if uuidEq(id, actor) {
			return
		}
		if _, dup := seen[*id]; dup {
			return
		}
		seen[*id] = struct{}{}
		out = append(out, *id)
	}
	add(t.AssigneeUserID)
	add(t.CreatedByUserID)
	return out
}

func uuidEq(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// emit inserts one row per recipient (after honouring per-user
// preferences) and publishes a `notification` event on the realtime
// hub. Failures are logged and swallowed — a notification miss must
// not roll back the underlying task write.
func (n *Notifier) emit(ctx context.Context, task *tasks.Task, kind string, actor *uuid.UUID, recipients []uuid.UUID, body string) {
	q := dbq.New(n.pool)
	anyInserted := false
	for _, uid := range recipients {
		allowed, err := prefAllows(ctx, q, uid, kind)
		if err != nil {
			n.logger.Warn("notification prefs lookup failed",
				"err", err, "user_id", uid, "kind", kind)
			continue
		}
		if !allowed {
			continue
		}
		var taskID *uuid.UUID
		tid := task.ID
		taskID = &tid
		_, err = q.InsertNotification(ctx, dbq.InsertNotificationParams{
			UserID:      uid,
			ProjectID:   task.ProjectID,
			Kind:        kind,
			TaskID:      taskID,
			ActorUserID: actor,
			Body:        body,
		})
		if err != nil {
			n.logger.Warn("notification insert failed",
				"err", err, "user_id", uid, "kind", kind, "task_id", task.ID)
			continue
		}
		anyInserted = true
	}
	if anyInserted && n.hub != nil {
		pid := task.ProjectID
		n.hub.Publish(realtime.Event{
			Type:      "notification",
			ProjectID: &pid,
		})
	}
}

// prefAllows checks the user's notification_preferences JSONB. Any
// unset kind defaults to true (opt-out semantics). A malformed JSON
// blob logs and defaults to true so we fail-open on data drift.
func prefAllows(ctx context.Context, q *dbq.Queries, userID uuid.UUID, kind string) (bool, error) {
	raw, err := q.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return true, err
	}
	if len(raw) == 0 {
		return true, nil
	}
	var m map[string]bool
	if err := json.Unmarshal(raw, &m); err != nil {
		return true, nil
	}
	v, ok := m[kind]
	if !ok {
		return true, nil
	}
	return v, nil
}
