package notifications_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mkNotifier wires a Notifier with a fresh pool and a silent logger.
// enabled controls the feature flag. A nil hub is fine — the Notifier
// tolerates it and simply skips the publish step.
func mkNotifier(t *testing.T, enabled bool) (*notifications.Notifier, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return notifications.New(pool, nil, logger, enabled), pool
}

// seedFixture creates: creator user, assignee user, third-party user,
// one project owned by creator, one task on the default cycle assigned
// to `assignee`. Returns the ids needed by the assertions.
type fixture struct {
	creator   *identity.User
	assignee  *identity.User
	third     *identity.User
	projectID uuid.UUID
	task      *tasks.Task
}

func seedFixture(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()
	ctx := t.Context()
	creator, _, err := identity.UpsertFromGithub(ctx, pool, 91001, "creator", "Creator", "")
	if err != nil {
		t.Fatalf("upsert creator: %v", err)
	}
	assignee, _, err := identity.UpsertFromGithub(ctx, pool, 91002, "assignee", "Assignee", "")
	if err != nil {
		t.Fatalf("upsert assignee: %v", err)
	}
	third, _, err := identity.UpsertFromGithub(ctx, pool, 91003, "third", "Third", "")
	if err != nil {
		t.Fatalf("upsert third: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "N", "", "", "", creator.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// Assignee must be a project member for tasks.Create to accept the
	// AssigneeUserID field. Grant them the first default role.
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("list roles: %v (roles=%d)", err, len(roles))
	}
	if err := identity.AddMembership(ctx, pool, assignee.ID, p.ID, roles[0].ID); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	creatorID := creator.ID
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:      p.ID,
		Title:          "Fix the thing",
		Type:           tasks.TypeTask,
		AssigneeUserID: &assignee.ID,
	}, tasks.Authorship{UserID: &creatorID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return &fixture{
		creator:   creator,
		assignee:  assignee,
		third:     third,
		projectID: p.ID,
		task:      tk,
	}
}

// countFor returns the number of notification rows for a user across
// all kinds (unread + read).
func countFor(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1`, userID).Scan(&n)
	if err != nil {
		t.Fatalf("countFor: %v", err)
	}
	return n
}

func TestNotifier_OnAssigneeChanged_NotifiesNewAssignee(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	prev := *fx.task
	prev.AssigneeUserID = nil // simulate previously unassigned
	newTask := *fx.task       // fx.task already has assignee set

	actor := fx.creator.ID
	n.OnAssigneeChanged(t.Context(), &prev, &newTask, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 1 {
		t.Errorf("assignee rows = %d, want 1", got)
	}
	if got := countFor(t, pool, fx.creator.ID); got != 0 {
		t.Errorf("actor (creator) rows = %d, want 0 (no self-notify)", got)
	}
	if got := countFor(t, pool, fx.third.ID); got != 0 {
		t.Errorf("third-party rows = %d, want 0", got)
	}
}

func TestNotifier_OnAssigneeChanged_SkipsSelfAssign(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	prev := *fx.task
	prev.AssigneeUserID = nil
	newTask := *fx.task
	newTask.AssigneeUserID = &fx.creator.ID // creator assigns themselves

	actor := fx.creator.ID
	n.OnAssigneeChanged(t.Context(), &prev, &newTask, &actor)

	if got := countFor(t, pool, fx.creator.ID); got != 0 {
		t.Errorf("self-assign rows = %d, want 0", got)
	}
}

func TestNotifier_OnAssigneeChanged_SkipsNoChange(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	prev := *fx.task
	newTask := *fx.task // same assignee, same state

	actor := fx.creator.ID
	n.OnAssigneeChanged(t.Context(), &prev, &newTask, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 0 {
		t.Errorf("same-assignee rows = %d, want 0 (no-op)", got)
	}
}

func TestNotifier_OnComment_NotifiesStakeholdersMinusActor(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	// Commenter is `third` — creator AND assignee should be notified.
	actor := fx.third.ID
	n.OnComment(t.Context(), fx.task, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 1 {
		t.Errorf("assignee rows = %d, want 1", got)
	}
	if got := countFor(t, pool, fx.creator.ID); got != 1 {
		t.Errorf("creator rows = %d, want 1", got)
	}
	if got := countFor(t, pool, fx.third.ID); got != 0 {
		t.Errorf("actor rows = %d, want 0", got)
	}
}

func TestNotifier_OnComment_DedupsAssigneeEqualsCreator(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	// Force assignee == creator by re-assigning.
	fx.task.AssigneeUserID = &fx.creator.ID

	// Commenter is `third`.
	actor := fx.third.ID
	n.OnComment(t.Context(), fx.task, &actor)

	if got := countFor(t, pool, fx.creator.ID); got != 1 {
		t.Errorf("creator=assignee rows = %d, want 1 (dedup)", got)
	}
}

func TestNotifier_OnStateChanged_FiresOnDone(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	// Assignee closes the task. Creator gets notified; assignee (actor)
	// does not.
	closed := *fx.task
	closed.State = tasks.StateDone
	actor := fx.assignee.ID
	n.OnStateChanged(t.Context(), &closed, tasks.StateDoing, &actor)

	if got := countFor(t, pool, fx.creator.ID); got != 1 {
		t.Errorf("creator rows = %d, want 1", got)
	}
	if got := countFor(t, pool, fx.assignee.ID); got != 0 {
		t.Errorf("actor rows = %d, want 0", got)
	}
}

func TestNotifier_OnStateChanged_SkipsTodoDoingTransition(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	moved := *fx.task
	moved.State = tasks.StateDoing
	actor := fx.creator.ID
	n.OnStateChanged(t.Context(), &moved, tasks.StateTodo, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 0 {
		t.Errorf("assignee rows for todo->doing = %d, want 0", got)
	}
}

func TestNotifier_Disabled_IsNoOp(t *testing.T) {
	n, pool := mkNotifier(t, false)
	fx := seedFixture(t, pool)

	actor := fx.creator.ID
	n.OnComment(t.Context(), fx.task, &actor)
	n.OnStateChanged(t.Context(), fx.task, tasks.StateTodo, &actor)
	prev := *fx.task
	prev.AssigneeUserID = nil
	n.OnAssigneeChanged(t.Context(), &prev, fx.task, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 0 {
		t.Errorf("disabled notifier still wrote %d rows", got)
	}
}

func TestNotifier_Preferences_OptOut(t *testing.T) {
	n, pool := mkNotifier(t, true)
	fx := seedFixture(t, pool)

	// Assignee opts out of task_commented.
	q := dbq.New(pool)
	if err := q.UpdatePreferences(t.Context(), dbq.UpdatePreferencesParams{
		UserID: fx.assignee.ID,
		Prefs:  []byte(`{"task_commented": false}`),
	}); err != nil {
		t.Fatalf("set prefs: %v", err)
	}

	// Third comments; assignee opted out → 0 rows for assignee; creator
	// still gets 1 (default true).
	actor := fx.third.ID
	n.OnComment(t.Context(), fx.task, &actor)

	if got := countFor(t, pool, fx.assignee.ID); got != 0 {
		t.Errorf("opted-out assignee rows = %d, want 0", got)
	}
	if got := countFor(t, pool, fx.creator.ID); got != 1 {
		t.Errorf("creator rows = %d, want 1", got)
	}
}

func TestNotifier_PublishesRealtime(t *testing.T) {
	pool := testutil.NewPool(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := realtime.New(logger)
	n := notifications.New(pool, hub, logger, true)
	fx := seedFixture(t, pool)

	ch, cancel := hub.Subscribe(fx.projectID)
	defer cancel()

	actor := fx.third.ID
	n.OnComment(t.Context(), fx.task, &actor)

	select {
	case ev := <-ch:
		if ev.Type != "notification" {
			t.Errorf("event type = %q, want %q", ev.Type, "notification")
		}
	default:
		t.Errorf("no realtime event published")
	}
}
