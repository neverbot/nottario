// Package realtime_test exercises the SSE fan-out hub and the
// LISTEN/NOTIFY plumbing. Hub publish/Subscribe paths run as pure
// unit tests (no DB). The LISTEN round-trip and the SSE HTTP handler
// each open a freshly-migrated database via internal/testutil.
package realtime_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/cycles"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// silentLogger discards every log line so failing tests don't drown
// in expected Warn output (slow-subscriber drops).
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

func TestHub_SubscribeDeliversOnlyMatchingProject(t *testing.T) {
	hub := realtime.New(silentLogger())
	pidA := uuid.New()
	pidB := uuid.New()

	chA, cancelA := hub.Subscribe(pidA)
	defer cancelA()
	chB, cancelB := hub.Subscribe(pidB)
	defer cancelB()

	// Direct publish via the test-only helper.
	realtime.PublishForTest(hub, realtime.Event{Type: "tasks.created", ProjectID: &pidA})

	select {
	case ev := <-chA:
		if ev.Type != "tasks.created" {
			t.Errorf("A got %q", ev.Type)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("A did not receive the event")
	}

	// B must NOT receive it.
	select {
	case ev := <-chB:
		t.Fatalf("B received event meant for A: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestHub_CancelClosesChannelAndDrops(t *testing.T) {
	hub := realtime.New(silentLogger())
	pid := uuid.New()
	ch, cancel := hub.Subscribe(pid)

	cancel()
	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for closed channel signal")
	}

	// Second cancel is a no-op (must not panic).
	cancel()

	// Publishing after cancel is also a no-op.
	realtime.PublishForTest(hub, realtime.Event{Type: "x", ProjectID: &pid})
}

func TestHub_SlowSubscriberDoesNotBlockOthers(t *testing.T) {
	hub := realtime.New(silentLogger())
	pid := uuid.New()

	// Slow: never drain. Its buffer will fill and subsequent
	// publishes drop for it (the hub logs a Warn).
	_, cancelSlow := hub.Subscribe(pid)
	defer cancelSlow()
	fast, cancelFast := hub.Subscribe(pid)
	defer cancelFast()

	// Publish one event at a time, draining fast immediately so the
	// next iteration always has room — this isolates the assertion
	// from goroutine scheduling. Slow gets the first 64 events (its
	// buffer fills) and drops the rest; fast receives all 128
	// because the publish iterates subs independently.
	const N = 128
	got := 0
	for i := 0; i < N; i++ {
		realtime.PublishForTest(hub, realtime.Event{Type: "x", ProjectID: &pid})
		select {
		case <-fast:
			got++
		case <-time.After(time.Second):
			t.Fatalf("fast subscriber stalled at i=%d", i)
		}
	}
	if got != N {
		t.Errorf("fast subscriber missed events: got %d, want %d", got, N)
	}
}

func TestHub_NilProjectIDIsIgnored(t *testing.T) {
	hub := realtime.New(silentLogger())
	pid := uuid.New()
	ch, cancel := hub.Subscribe(pid)
	defer cancel()

	// Event without a project_id never reaches any project subscriber.
	realtime.PublishForTest(hub, realtime.Event{Type: "global"})

	select {
	case ev := <-ch:
		t.Fatalf("received broadcast on a project subscriber: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// runHubInBackground starts hub.Run on a fresh DB-backed pool. When
// the returned stop is called, Run unwinds cleanly.
func runHubInBackground(t *testing.T, hub *realtime.Hub) (context.Context, *pgxpool.Pool, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	pool := testutil.NewPool(t)
	done := make(chan error, 1)
	go func() { done <- hub.Run(ctx, pool) }()
	// Give Run a beat to LISTEN before tests publish via pg_notify.
	time.Sleep(150 * time.Millisecond)

	stop := func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("hub.Run returned: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("hub.Run did not return after cancel")
		}
	}
	return ctx, pool, stop
}

func TestHub_RunDeliversPgNotify(t *testing.T) {
	hub := realtime.New(silentLogger())
	_, pool, stop := runHubInBackground(t, hub)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Seed a user + project so we have a real project_id to scope by.
	u, _, err := identity.UpsertFromGithub(ctx, pool, 1234, "realt", "Realt", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "RT", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	ch, cancelSub := hub.Subscribe(p.ID)
	defer cancelSub()

	// Publish a raw notify with a JSON payload shaped like a real event.
	payload := fmt.Sprintf(`{"type":"tasks.created","op":"INSERT","project_id":%q,"scope":"tasks"}`, p.ID)
	if _, err := pool.Exec(ctx, "SELECT pg_notify('nottario_events', $1)", payload); err != nil {
		t.Fatalf("pg_notify: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Type != "tasks.created" || ev.ProjectID == nil || *ev.ProjectID != p.ID {
			t.Errorf("event mismatch: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for NOTIFY round-trip")
	}

	// A subscriber on a DIFFERENT project never sees the event.
	other := uuid.New()
	otherCh, otherCancel := hub.Subscribe(other)
	defer otherCancel()
	_, _ = pool.Exec(ctx, "SELECT pg_notify('nottario_events', $1)", payload)
	select {
	case ev := <-otherCh:
		t.Fatalf("other-project subscriber leaked event: %+v", ev)
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestHub_TaskClaimedEventReachesSubscribers(t *testing.T) {
	hub := realtime.New(silentLogger())
	_, pool, stop := runHubInBackground(t, hub)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 4242, "claimer", "Claimer", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "ClaimRT", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID

	by := tasks.Authorship{UserID: &u.ID}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "Pickme", TargetRoleID: &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, cancelSub := hub.Subscribe(p.ID)
	defer cancelSub()

	if _, err := tasks.Claim(ctx, pool, tk.ID, u.ID); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	deadline := time.After(3 * time.Second)
	gotClaimed := false
	for !gotClaimed {
		select {
		case ev := <-ch:
			if ev.Type == "task.claimed" {
				if ev.TaskID == nil || *ev.TaskID != tk.ID {
					t.Errorf("task.claimed task_id mismatch: %+v", ev)
				}
				if ev.AssigneeUserID == nil || *ev.AssigneeUserID != u.ID {
					t.Errorf("task.claimed assignee mismatch: %+v", ev)
				}
				if ev.ClaimedAt == nil {
					t.Errorf("task.claimed missing claimed_at: %+v", ev)
				}
				gotClaimed = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for task.claimed event")
		}
	}
}

// TestHub_TaskCommentCreatedEventReachesSubscribers exercises the
// `task_comments_notify_insert` DB trigger added in migration 00002.
// Without it an open task-detail dialog cannot learn about a comment
// posted by a different client and stays stale until the user
// reloads the page.
func TestHub_TaskCommentCreatedEventReachesSubscribers(t *testing.T) {
	hub := realtime.New(silentLogger())
	_, pool, stop := runHubInBackground(t, hub)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 5600, "commenter", "Commenter", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "CommentRT", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID

	by := tasks.Authorship{UserID: &u.ID}
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "Comment me", TargetRoleID: &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, cancelSub := hub.Subscribe(p.ID)
	defer cancelSub()

	if _, err := tasks.AddComment(ctx, pool, tk.ID, "first post", by); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type != "task.comment.created" {
				continue
			}
			if ev.TaskID == nil || *ev.TaskID != tk.ID {
				t.Errorf("task.comment.created task_id mismatch: %+v", ev)
			}
			if ev.ProjectID == nil || *ev.ProjectID != p.ID {
				t.Errorf("task.comment.created project_id mismatch: %+v", ev)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for task.comment.created event")
		}
	}
}

func TestHub_CycleClosedEventReachesSubscribers(t *testing.T) {
	hub := realtime.New(silentLogger())
	_, pool, stop := runHubInBackground(t, hub)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 5500, "cyclesub", "CycSub", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "EvtProj", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	ch, cancelSub := hub.Subscribe(p.ID)
	defer cancelSub()

	if _, err := cycles.EndCycle(ctx, pool, cycles.EndCycleParams{ProjectID: p.ID},
		cycles.Authorship{UserID: &u.ID}); err != nil {
		t.Fatalf("EndCycle: %v", err)
	}

	deadline := time.After(3 * time.Second)
	seenClosed, seenCreated := false, false
	for !(seenClosed && seenCreated) {
		select {
		case ev := <-ch:
			if ev.Type == "cycle.closed" {
				seenClosed = true
			}
			if ev.Type == "cycle.created" {
				seenCreated = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting for cycle events (closed=%v, created=%v)", seenClosed, seenCreated)
		}
	}
}

func TestHub_RunIgnoresMalformedPayload(t *testing.T) {
	hub := realtime.New(silentLogger())
	ctx, pool, stop := runHubInBackground(t, hub)
	defer stop()

	pid := uuid.New()
	ch, cancel := hub.Subscribe(pid)
	defer cancel()

	// Garbage payload — the listener loop should log and continue,
	// not crash.
	if _, err := pool.Exec(ctx, "SELECT pg_notify('nottario_events', $1)", "not json"); err != nil {
		t.Fatalf("pg_notify: %v", err)
	}

	// Subsequent valid payload still arrives.
	valid := fmt.Sprintf(`{"type":"docs.updated","project_id":%q}`, pid)
	if _, err := pool.Exec(ctx, "SELECT pg_notify('nottario_events', $1)", valid); err != nil {
		t.Fatalf("pg_notify valid: %v", err)
	}
	select {
	case ev := <-ch:
		if ev.Type != "docs.updated" {
			t.Errorf("expected docs.updated, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener didn't recover from malformed payload")
	}
}

// ---- SSE handler ----

func TestSSE_RejectsUnauthenticated(t *testing.T) {
	pool := testutil.NewPool(t)
	hub := realtime.New(silentLogger())
	resolver := identity.NewResolver(pool, []byte("0123456789abcdef0123456789abcdef"), false)

	srv := httptest.NewServer(realtime.SSEHandler(hub, pool, resolver))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?project_id=" + uuid.New().String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestSSE_RequiresProjectID(t *testing.T) {
	pool := testutil.NewPool(t)
	hub := realtime.New(silentLogger())
	key := []byte("0123456789abcdef0123456789abcdef")
	resolver := identity.NewResolver(pool, key, false)

	ctx := context.Background()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 9876, "u", "U", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	sess, err := identity.NewSession(ctx, pool, u.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookieVal := identity.EncodeCookie(sess.ID, key)

	srv := httptest.NewServer(realtime.SSEHandler(hub, pool, resolver))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: cookieVal})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestSSE_NonMemberForbidden(t *testing.T) {
	pool := testutil.NewPool(t)
	hub := realtime.New(silentLogger())
	key := []byte("0123456789abcdef0123456789abcdef")
	resolver := identity.NewResolver(pool, key, false)

	ctx := context.Background()
	owner, _, err := identity.UpsertFromGithub(ctx, pool, 1, "owner", "Owner", "")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	// Owner becomes admin if first user; switch off so the second user
	// genuinely has no access. The simplest way is to log in with the
	// SECOND user; the first one always lands admin.
	outsider, _, err := identity.UpsertFromGithub(ctx, pool, 2, "outsider", "Outsider", "")
	if err != nil {
		t.Fatalf("outsider: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "P", "", "", "", owner.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	sess, err := identity.NewSession(ctx, pool, outsider.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookieVal := identity.EncodeCookie(sess.ID, key)

	srv := httptest.NewServer(realtime.SSEHandler(hub, pool, resolver))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?project_id="+p.ID.String(), nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: cookieVal})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", resp.StatusCode)
	}
}

func TestSSE_StreamsInitialOKAndEvents(t *testing.T) {
	pool := testutil.NewPool(t)
	hub := realtime.New(silentLogger())
	key := []byte("0123456789abcdef0123456789abcdef")
	resolver := identity.NewResolver(pool, key, false)

	ctx := context.Background()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 7, "rt", "RT", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "RT", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	sess, err := identity.NewSession(ctx, pool, u.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookieVal := identity.EncodeCookie(sess.ID, key)

	srv := httptest.NewServer(realtime.SSEHandler(hub, pool, resolver))
	defer srv.Close()

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"?project_id="+p.ID.String(), nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: cookieVal})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type: %q", ct)
	}

	rdr := bufio.NewReader(resp.Body)
	// Initial ": ok" comment.
	line, err := rdr.ReadString('\n')
	if err != nil {
		t.Fatalf("read initial: %v", err)
	}
	if !strings.HasPrefix(line, ": ok") {
		t.Errorf("initial line %q", line)
	}

	// Fire an event through the hub. The SSE goroutine has already
	// subscribed by the time we got past the initial comment.
	go func() {
		time.Sleep(50 * time.Millisecond)
		realtime.PublishForTest(hub, realtime.Event{Type: "tasks.created", ProjectID: &p.ID})
	}()

	// Read until we see a `data:` line.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err = rdr.ReadString('\n')
		if err != nil {
			t.Fatalf("read line: %v", err)
		}
		if strings.HasPrefix(line, "data:") {
			if !strings.Contains(line, "tasks.created") {
				t.Errorf("data line missing event type: %q", line)
			}
			return
		}
	}
	t.Fatal("did not receive `data:` line within deadline")
}
