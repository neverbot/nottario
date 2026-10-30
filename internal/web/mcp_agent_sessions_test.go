package web

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPAgentSessionOnClaim(t *testing.T) {
	f := newMCPFixture(t, 9801, "sessclaim")
	pool := f.pgx(t)

	// The session fields are part of the tools' input schema (embedded
	// structs are flattened).
	tools, err := f.session.ListTools(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		switch tool.Name {
		case "nottario.tasks.create", "nottario.tasks.claim", "nottario.tasks.claim_next":
			props := inputProps(t, tool)
			for _, k := range []string{"session", "session_label", "project_id"} {
				if _, ok := props[k]; !ok {
					t.Errorf("%s: input schema lacks %q", tool.Name, k)
				}
			}
		case "nottario.tasks.get", "nottario.tasks.next":
			if _, ok := inputProps(t, tool)["session"]; ok {
				t.Errorf("%s must not take a session", tool.Name)
			}
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "move docs", "claim": true,
		"session": "s-1", "session_label": "move docs",
	}, &created)

	// The server is stateless, so the client name comes from the
	// User-Agent header of the test transport ("qa-test/0").
	var label, client, key string
	var tokenSet bool
	if err := pool.QueryRow(f.ctx,
		`SELECT s.label, s.client, s.session_key, s.token_id IS NOT NULL
		 FROM tasks t JOIN agent_sessions s ON s.id = t.active_session_id
		 WHERE t.id = $1`, created.ID).Scan(&label, &client, &key, &tokenSet); err != nil {
		t.Fatalf("read active session: %v", err)
	}
	if label != "move docs" || client != "qa-test" || key != "s-1" || !tokenSet {
		t.Fatalf("session = %q %q %q token=%v", label, client, key, tokenSet)
	}

	// Claiming another task with the same session reuses the row.
	var other struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "second",
	}, &other)
	f.callJSON(t, "nottario.tasks.claim", map[string]any{
		"project_id": f.projectID, "task_id": other.ID, "session": "s-1",
	}, nil)
	var n int
	if err := pool.QueryRow(f.ctx, `SELECT count(DISTINCT active_session_id) FROM tasks WHERE id = ANY($1::uuid[])`,
		[]string{created.ID, other.ID}).Scan(&n); err != nil || n != 1 {
		t.Fatalf("distinct sessions = %d err=%v", n, err)
	}

	// A claim without a session leaves it unset.
	var bare struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "bare", "claim": true,
	}, &bare)
	var isNull bool
	if err := pool.QueryRow(f.ctx, `SELECT active_session_id IS NULL FROM tasks WHERE id = $1`, bare.ID).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("bare claim active_session_id null=%v err=%v", isNull, err)
	}

	// Leaving doing clears the session.
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": created.ID, "state": "todo",
	}, nil)
	if err := pool.QueryRow(f.ctx, `SELECT active_session_id IS NULL FROM tasks WHERE id = $1`, created.ID).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("set_state todo: active_session_id null=%v err=%v", isNull, err)
	}
	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": other.ID, "state": "done",
	}, nil)
	if err := pool.QueryRow(f.ctx, `SELECT active_session_id IS NULL FROM tasks WHERE id = $1`, other.ID).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("set_state done: active_session_id null=%v err=%v", isNull, err)
	}
}

func TestMCPAgentSessionClaimNextWontDoAndLabel(t *testing.T) {
	f := newMCPFixture(t, 9802, "sessnext")
	pool := f.pgx(t)
	sessionOf := func(id string) (key, label string, has bool) {
		t.Helper()
		var k, l *string
		if err := pool.QueryRow(f.ctx,
			`SELECT s.session_key, s.label FROM tasks t LEFT JOIN agent_sessions s ON s.id = t.active_session_id WHERE t.id = $1`,
			id).Scan(&k, &l); err != nil {
			t.Fatal(err)
		}
		if k == nil {
			return "", "", false
		}
		return *k, *l, true
	}

	var made struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "next one"}, &made)

	var next struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		SessionError string `json:"session_error"`
	}
	f.callJSON(t, "nottario.tasks.claim_next", map[string]any{
		"project_id": f.projectID, "session": "s-2", "session_label": "first label",
	}, &next)
	if next.Task.ID != made.ID || next.SessionError != "" {
		t.Fatalf("claim_next = %+v", next)
	}
	if k, l, ok := sessionOf(made.ID); !ok || k != "s-2" || l != "first label" {
		t.Fatalf("session after claim_next = %q %q %v", k, l, ok)
	}

	// A later call with the same key replaces the label.
	f.callJSON(t, "nottario.tasks.claim", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "session": "s-2", "session_label": "new label",
	}, nil)
	if _, l, _ := sessionOf(made.ID); l != "new label" {
		t.Fatalf("label = %q, want replaced", l)
	}

	f.callJSON(t, "nottario.tasks.set_state", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "state": "wont_do",
	}, nil)
	if _, _, ok := sessionOf(made.ID); ok {
		t.Fatal("wont_do must clear the active session")
	}
}

func TestMCPAgentSessionRejectedBeforeWrite(t *testing.T) {
	f := newMCPFixture(t, 9803, "sessbad")
	pool := f.pgx(t)
	count := func(q string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(f.ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	var made struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "todo one"}, &made)
	tasksBefore := count(`SELECT count(*) FROM tasks`)

	f.callExpectErr(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "never", "claim": true, "session": "   ",
	})
	f.callExpectErr(t, "nottario.tasks.claim_next", map[string]any{"project_id": f.projectID, "session": "   "})
	f.callExpectErr(t, "nottario.tasks.claim", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "session": "   ",
	})

	if n := count(`SELECT count(*) FROM tasks`); n != tasksBefore {
		t.Fatalf("tasks = %d, want %d: create wrote despite the bad session", n, tasksBefore)
	}
	if n := count(`SELECT count(*) FROM tasks WHERE state = 'doing'`); n != 0 {
		t.Fatalf("%d tasks in doing, nothing must be claimed", n)
	}
	if n := count(`SELECT count(*) FROM agent_sessions`); n != 0 {
		t.Fatalf("%d sessions written", n)
	}
}

func TestMCPAgentSessionClearedOnReassign(t *testing.T) {
	f := newMCPFixture(t, 9804, "sessassign")
	pool := f.pgx(t)
	var made struct {
		ID string `json:"id"`
	}
	f.callJSON(t, "nottario.tasks.create", map[string]any{
		"project_id": f.projectID, "title": "held", "claim": true, "session": "s-3",
	}, &made)
	hasSession := func() bool {
		t.Helper()
		var has bool
		if err := pool.QueryRow(f.ctx, `SELECT active_session_id IS NOT NULL FROM tasks WHERE id = $1`, made.ID).Scan(&has); err != nil {
			t.Fatal(err)
		}
		return has
	}

	// Re-assigning to the same user keeps the session.
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "assignee_user_id": "me",
	}, nil)
	if !hasSession() {
		t.Fatal("same assignee must keep the session")
	}
	// Editing something else keeps it too.
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "title": "held again",
	}, nil)
	if !hasSession() {
		t.Fatal("a title edit must keep the session")
	}
	f.callJSON(t, "nottario.tasks.update", map[string]any{
		"project_id": f.projectID, "task_id": made.ID, "assignee_user_id": "none",
	}, nil)
	if hasSession() {
		t.Fatal("unassigning must clear the session")
	}
}

func inputProps(t *testing.T, tool *sdk.Tool) map[string]any {
	t.Helper()
	m, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("%s: input schema is %T", tool.Name, tool.InputSchema)
	}
	props, _ := m["properties"].(map[string]any)
	return props
}
