package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestMCP_StreamableHTTPTransport dials the bundled MCP server over
// its real streamable-HTTP transport (the same wire the Claude Code /
// Cursor / Claude Desktop clients use) and exercises a handful of
// tools end-to-end. The point is not to test every tool: that's the
// per-domain coverage. The point is to prove the transport, auth,
// session lifecycle and tool registration are all wired correctly.
//
// We use the upstream SDK's mcp.Client + StreamableClientTransport
// (no hand-rolled JSON-RPC), so any change to the SDK or to our
// Handler() wrapping is caught here.
func TestMCP_StreamableHTTPTransport(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Provision an authenticated identity: user + token + project +
	// memberships so whoami returns a non-empty memberships array.
	u, _, err := identity.UpsertFromGithub(ctx, pool, 13301, "mcp-tester", "MCP Tester", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	proj, err := identity.CreateProject(ctx, pool, "MCPProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	plaintext, _, err := identity.IssueToken(ctx, pool, u.ID, proj.ID, "mcp-token", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Stand up the real router; /mcp is mounted inside NewServer.
	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// HTTP client that injects the Bearer token on every request.
	// The SDK client uses the same client for the initial POST that
	// negotiates a session and for every follow-up call.
	httpClient := &http.Client{
		Transport: bearerTransport{
			rt:    http.DefaultTransport,
			token: plaintext,
		},
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "qa-test", Version: "v0"}, nil)
	transport := &sdk.StreamableClientTransport{
		Endpoint:   ts.URL + "/mcp",
		HTTPClient: httpClient,
		// Don't open the standalone SSE channel: we only want
		// request/response semantics here, and the server runs in
		// stateless mode anyway (no server-initiated messages).
		DisableStandaloneSSE: true,
	}
	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	// --- ListTools: catalogue should expose every registered tool. ---
	list, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	have := map[string]bool{}
	for _, tool := range list.Tools {
		have[tool.Name] = true
	}
	// A subset of tools that MUST be advertised. If the registration
	// gets accidentally removed, this test fails immediately.
	for _, must := range []string{
		"nottario.whoami",
		"nottario.projects.list",
		"nottario.tasks.list",
		"nottario.tasks.create",
		"nottario.tasks.claim_next",
		"nottario.docs.list",
		"nottario.arch.list_nodes",
	} {
		if !have[must] {
			t.Errorf("ListTools missing %s; advertised: %d tools", must, len(have))
		}
	}

	// --- CallTool: nottario.whoami should resolve our token's user. ---
	whoami, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name:      "nottario.whoami",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool whoami: %v", err)
	}
	if len(whoami.Content) == 0 {
		t.Fatalf("whoami result has no content")
	}
	// Tool results come back as a slice of Content blocks; the JSON
	// payload sits in the first TextContent block. Parse it.
	tc, ok := whoami.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("whoami content[0] is %T, want *sdk.TextContent", whoami.Content[0])
	}
	var who map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &who); err != nil {
		t.Fatalf("decode whoami text %q: %v", tc.Text, err)
	}
	if who["github_login"] != "mcp-tester" {
		t.Fatalf("whoami github_login=%v want mcp-tester", who["github_login"])
	}
	if who["user_id"] != u.ID.String() {
		t.Fatalf("whoami user_id=%v want %s", who["user_id"], u.ID)
	}

	// --- CallTool: nottario.tasks.create then nottario.tasks.get. ---
	created, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name: "nottario.tasks.create",
		Arguments: map[string]any{
			"project_id": proj.ID.String(),
			"title":      "via MCP integration test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool tasks.create: %v", err)
	}
	createdText, _ := created.Content[0].(*sdk.TextContent)
	var createdTask map[string]any
	if err := json.Unmarshal([]byte(createdText.Text), &createdTask); err != nil {
		t.Fatalf("decode created task: %v body=%s", err, createdText.Text)
	}
	if createdTask["title"] != "via MCP integration test" {
		t.Fatalf("created title mismatch: %+v", createdTask)
	}
	if createdTask["state"] != "todo" {
		t.Fatalf("created state=%v want todo", createdTask["state"])
	}
}

// bearerTransport is a tiny http.RoundTripper that adds an
// Authorization header to every outbound request. The SDK does not
// expose a hook for per-request headers, so wrapping the transport
// is the supported way to inject auth.
type bearerTransport struct {
	rt    http.RoundTripper
	token string
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.rt.RoundTrip(r)
}
