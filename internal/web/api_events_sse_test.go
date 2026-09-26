package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/testutil"
)

// /events is what keeps every open board in sync. The hub itself is
// tested in internal/realtime; what is untested is this route: who is
// allowed to open a stream, which project's events they receive, and
// whether the bytes on the wire are the event-stream shape an
// EventSource expects.
func TestApiEvents_StreamDeliversProjectEvents(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")

	member, _, err := identity.UpsertFromGithub(ctx, pool, 14101, "sse-member", "Member", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	// Not an admin: admins skip the membership check, so an admin
	// would prove nothing about the gate.
	outsider, _, err := identity.UpsertFromGithub(ctx, pool, 14102, "sse-outsider", "Outsider", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub outsider: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "SSE Project", "", "", "", member.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	other, err := identity.CreateProject(ctx, pool, "Someone Else", "", "", "", outsider.ID)
	if err != nil {
		t.Fatalf("CreateProject other: %v", err)
	}

	hub := realtime.New(nil)
	ts := httptest.NewServer(NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, key, false),
		Hub:      hub,
	}))
	t.Cleanup(ts.Close)

	cookieFor := func(u uuid.UUID) *http.Cookie {
		t.Helper()
		sess, err := identity.NewSession(ctx, pool, u, "test", "127.0.0.1")
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		return &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	}

	// Anonymous and non-member requests never reach the stream.
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		query  string
		want   int
	}{
		{"anonymous", nil, "?project_id=" + p.ID.String(), http.StatusUnauthorized},
		{"not a member of that project", cookieFor(outsider.ID), "?project_id=" + p.ID.String(), http.StatusForbidden},
		{"malformed project_id", cookieFor(member.ID), "?project_id=not-a-uuid", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(t.Context(), "GET", ts.URL+"/events"+tc.query, nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}

	// A member opens a stream and receives this project's events only.
	streamCtx, stopStream := context.WithCancel(t.Context())
	defer stopStream()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", ts.URL+"/events?project_id="+p.ID.String(), nil)
	req.AddCookie(cookieFor(member.ID))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q: a buffered stream is a stalled UI", cc)
	}

	reader := bufio.NewReader(resp.Body)
	// The handler opens with a comment line so the browser considers
	// the connection live before anything happens.
	opening, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read opening line: %v", err)
	}
	if !strings.HasPrefix(opening, ":") {
		t.Errorf("stream opened with %q, want a comment line", opening)
	}

	taskID := uuid.New()
	pid, otherPID := p.ID, other.ID
	// An event for another project must not reach this subscriber.
	hub.Publish(realtime.Event{Type: "task.updated", ProjectID: &otherPID, TaskID: &taskID})
	hub.Publish(realtime.Event{Type: "task.created", ProjectID: &pid, TaskID: &taskID, Op: "insert"})

	type line struct{ text string }
	got := make(chan line, 1)
	go func() {
		for {
			s, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(s, "data: ") {
				got <- line{strings.TrimSpace(strings.TrimPrefix(s, "data: "))}
				return
			}
		}
	}()

	select {
	case l := <-got:
		var ev realtime.Event
		if err := json.Unmarshal([]byte(l.text), &ev); err != nil {
			t.Fatalf("event is not json: %q", l.text)
		}
		if ev.Type != "task.created" {
			t.Errorf("first event delivered was %q — the other project's event leaked through", ev.Type)
		}
		if ev.ProjectID == nil || *ev.ProjectID != p.ID {
			t.Errorf("event carries project %v, want %s", ev.ProjectID, p.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event reached the stream")
	}
}
