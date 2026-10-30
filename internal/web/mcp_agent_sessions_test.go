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
	// User-Agent header of the test transport.
	var label, client, key string
	var tokenSet bool
	if err := pool.QueryRow(f.ctx,
		`SELECT s.label, s.client, s.session_key, s.token_id IS NOT NULL
		 FROM tasks t JOIN agent_sessions s ON s.id = t.active_session_id
		 WHERE t.id = $1`, created.ID).Scan(&label, &client, &key, &tokenSet); err != nil {
		t.Fatalf("read active session: %v", err)
	}
	if label != "move docs" || client != "Go-http-client" || key != "s-1" || !tokenSet {
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
	for _, state := range []string{"todo", "done"} {
		id := created.ID
		if state == "done" {
			id = other.ID
		}
		f.callJSON(t, "nottario.tasks.set_state", map[string]any{
			"project_id": f.projectID, "task_id": id, "state": state,
		}, nil)
		if err := pool.QueryRow(f.ctx, `SELECT active_session_id IS NULL FROM tasks WHERE id = $1`, id).Scan(&isNull); err != nil || !isNull {
			t.Fatalf("set_state %s: active_session_id null=%v err=%v", state, isNull, err)
		}
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
