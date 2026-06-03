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

// mcpFixture is the minimum surface a per-tool-family test needs:
// a connected MCP session, the seeded user + project, and a context.
type mcpFixture struct {
	ctx       context.Context
	session   *sdk.ClientSession
	pool      poolHandle
	userID    string
	projectID string
}

// poolHandle is satisfied by *pgxpool.Pool but avoids a direct
// import in test files that don't need it.
type poolHandle interface {
	Close()
}

// newMCPFixture stands up the real router and connects an SDK MCP
// client over the streamable-HTTP transport. The bearer token is
// injected on every request via bearerTransport. The fixture cleans
// up its session, server and pool on test teardown.
func newMCPFixture(t *testing.T, githubID int64, login string) *mcpFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	u, _, err := identity.UpsertFromGithub(ctx, pool, githubID, login, login, "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "MCP "+login, "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "mcp", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	httpClient := &http.Client{
		Transport: bearerTransport{rt: http.DefaultTransport, token: token},
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "qa-test", Version: "v0"}, nil)
	transport := &sdk.StreamableClientTransport{
		Endpoint:             ts.URL + "/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return &mcpFixture{
		ctx:       ctx,
		session:   session,
		pool:      pool,
		userID:    u.ID.String(),
		projectID: p.ID.String(),
	}
}

// callJSON runs the named tool with args and decodes the first
// TextContent block into out. Fails the test on any error.
func (f *mcpFixture) callJSON(t *testing.T, name string, args map[string]any, out any) {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("CallTool %s: empty content", name)
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("CallTool %s: content[0] is %T", name, res.Content[0])
	}
	if out == nil {
		return
	}
	if err := json.Unmarshal([]byte(tc.Text), out); err != nil {
		t.Fatalf("decode %s body %q: %v", name, tc.Text, err)
	}
}

// callExpectErr runs the named tool and asserts the tool returned an
// error result (IsError=true). Returns the error text for further
// assertions.
func (f *mcpFixture) callExpectErr(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		// Some tools surface validation errors as protocol-level errors
		// rather than IsError. Either is fine: the call did not succeed.
		return err.Error()
	}
	if !res.IsError {
		t.Fatalf("CallTool %s expected error, got: %+v", name, res)
	}
	if len(res.Content) == 0 {
		return ""
	}
	tc, _ := res.Content[0].(*sdk.TextContent)
	if tc != nil {
		return tc.Text
	}
	return ""
}
