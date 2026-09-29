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

// A browser keeps ONE stream for all its tabs, and those tabs can be on
// different projects, so a stream may follow several. Each one is
// checked, a token cannot reach past its own project, and the list has
// a ceiling.
func TestApiEvents_OneStreamForSeveralProjects(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	key := []byte("test-session-key")

	// First user becomes admin; this one must not be, or the membership
	// checks would be skipped.
	if _, _, err := identity.UpsertFromGithub(ctx, pool, 14111, "sse-admin", "Admin", ""); err != nil {
		t.Fatalf("UpsertFromGithub admin: %v", err)
	}
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14112, "sse-multi", "Multi", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	a, _ := identity.CreateProject(ctx, pool, "Stream A", "", "", "", u.ID)
	b, _ := identity.CreateProject(ctx, pool, "Stream B", "", "", "", u.ID)
	other, _, _ := identity.UpsertFromGithub(ctx, pool, 14113, "sse-stranger", "Stranger", "")
	c, _ := identity.CreateProject(ctx, pool, "Not Theirs", "", "", "", other.ID)

	hub := realtime.New(nil)
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false), Hub: hub}))
	t.Cleanup(ts.Close)
	sess, _ := identity.NewSession(ctx, pool, u.ID, "t", "127.0.0.1")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}

	status := func(query, auth string) int {
		t.Helper()
		reqCtx, cancel := context.WithCancel(t.Context())
		defer cancel()
		req, _ := http.NewRequestWithContext(reqCtx, "GET", ts.URL+"/events"+query, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		} else {
			req.AddCookie(cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	both := "?project_id=" + a.ID.String() + "&project_id=" + b.ID.String()
	if got := status(both, ""); got != http.StatusOK {
		t.Errorf("two projects the user belongs to: %d, want 200", got)
	}
	if got := status(both+"&project_id="+c.ID.String(), ""); got != http.StatusForbidden {
		t.Errorf("slipping in a project the user is not in: %d, want 403", got)
	}

	// A token bound to A must not follow B, even though its owner can.
	token, _, err := identity.IssueToken(ctx, pool, u.ID, a.ID, "scoped", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if got := status("?project_id="+a.ID.String(), "Bearer "+token); got != http.StatusOK {
		t.Errorf("token on its own project: %d, want 200", got)
	}
	if got := status(both, "Bearer "+token); got != http.StatusForbidden {
		t.Errorf("token following a project outside its scope: %d, want 403", got)
	}

	var many strings.Builder
	for i := 0; i < 33; i++ {
		many.WriteString("&project_id=" + a.ID.String())
	}
	if got := status("?"+many.String()[1:], ""); got != http.StatusBadRequest {
		t.Errorf("an unbounded project list: %d, want 400", got)
	}

	// And the events really arrive from both projects on one stream.
	streamCtx, stop := context.WithCancel(t.Context())
	defer stop()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", ts.URL+"/events"+both, nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	aID, bID, cID, tid := a.ID, b.ID, c.ID, uuid.New()
	hub.Publish(realtime.Event{Type: "in.c", ProjectID: &cID, TaskID: &tid})
	hub.Publish(realtime.Event{Type: "in.a", ProjectID: &aID, TaskID: &tid})
	hub.Publish(realtime.Event{Type: "in.b", ProjectID: &bID, TaskID: &tid})

	seen := make(chan string, 4)
	go func() {
		for {
			s, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(s, "data: ") {
				var ev realtime.Event
				if json.Unmarshal([]byte(strings.TrimPrefix(s, "data: ")), &ev) == nil {
					seen <- ev.Type
				}
			}
		}
	}()
	var got []string
	for len(got) < 2 {
		select {
		case typ := <-seen:
			got = append(got, typ)
		case <-time.After(5 * time.Second):
			t.Fatalf("events received: %v, want in.a and in.b", got)
		}
	}
	if got[0] != "in.a" || got[1] != "in.b" {
		t.Errorf("stream delivered %v, want [in.a in.b] and nothing from project C", got)
	}
}
