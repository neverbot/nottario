package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// scopeFixture provisions ONE user who is a member of TWO projects (A
// and B). The MCP session is authenticated with a token bound to A;
// every call against B must be rejected with a ProjectScopeError-style
// message that names both project ids.
type scopeFixture struct {
	ctx      context.Context
	session  *sdk.ClientSession
	pool     poolHandle
	userID   string
	projectA string
	projectB string
}

func newScopeFixture(t *testing.T) *scopeFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	u, _, err := identity.UpsertFromGithub(ctx, pool, 99001, "scope-user", "Scope User", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	pA, err := identity.CreateProject(ctx, pool, "ScopeA", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject A: %v", err)
	}
	pB, err := identity.CreateProject(ctx, pool, "ScopeB", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject B: %v", err)
	}
	// The creator is owner of both — they're a member of both by
	// construction (CreateProject seeds the owner).

	// Issue a token bound to A.
	tokenA, _, err := identity.IssueToken(ctx, pool, u.ID, pA.ID, "mcp-A", nil)
	if err != nil {
		t.Fatalf("IssueToken A: %v", err)
	}

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	httpClient := &http.Client{
		Transport: bearerTransport{rt: http.DefaultTransport, token: tokenA},
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "qa-scope", Version: "v0"}, nil)
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

	return &scopeFixture{
		ctx:      ctx,
		session:  session,
		pool:     pool,
		userID:   u.ID.String(),
		projectA: pA.ID.String(),
		projectB: pB.ID.String(),
	}
}

func (f *scopeFixture) callExpectErr(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
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

func (f *scopeFixture) callJSON(t *testing.T, name string, args map[string]any, out any) {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if res.IsError {
		body := ""
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*sdk.TextContent); ok {
				body = tc.Text
			}
		}
		t.Fatalf("CallTool %s unexpected error: %s", name, body)
	}
	if out == nil {
		return
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("CallTool %s: content[0] is %T", name, res.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), out); err != nil {
		t.Fatalf("decode %s body %q: %v", name, tc.Text, err)
	}
}

// assertScopeError fails when the error text does not look like a
// scope-mismatch error (must contain both project IDs).
func (f *scopeFixture) assertScopeError(t *testing.T, where, msg string) {
	t.Helper()
	if !strings.Contains(msg, f.projectA) || !strings.Contains(msg, f.projectB) {
		t.Fatalf("%s: expected scope error mentioning both %s and %s, got %q", where, f.projectA, f.projectB, msg)
	}
	if !strings.Contains(strings.ToLower(msg), "scoped to project") {
		t.Fatalf("%s: expected scope error phrasing, got %q", where, msg)
	}
}

// TestMCP_TokenProjectScope_Enforcement is the regression battery for
// the per-token project-scope invariant. One token bound to A must
// not be able to reach project B's surface, even when the underlying
// user is a member of B.
func TestMCP_TokenProjectScope_Enforcement(t *testing.T) {
	f := newScopeFixture(t)

	// --- tasks family ---
	t.Run("tasks_list_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.tasks.list", map[string]any{
			"project_id": f.projectB,
		})
		f.assertScopeError(t, "tasks.list", msg)
	})
	t.Run("tasks_list_succeeds_for_own_project", func(t *testing.T) {
		var out map[string]any
		f.callJSON(t, "nottario.tasks.list", map[string]any{"project_id": f.projectA}, &out)
	})

	// --- docs family ---
	t.Run("docs_list_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.docs.list", map[string]any{
			"scope":      "project",
			"project_id": f.projectB,
		})
		f.assertScopeError(t, "docs.list", msg)
	})

	// --- arch family ---
	t.Run("arch_list_nodes_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.arch.list_nodes", map[string]any{
			"project_id": f.projectB,
		})
		f.assertScopeError(t, "arch.list_nodes", msg)
	})

	// --- search ---
	t.Run("search_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.search", map[string]any{
			"project_id": f.projectB,
			"query":      "anything",
		})
		f.assertScopeError(t, "search", msg)
	})

	// --- cycles family ---
	t.Run("cycles_list_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.cycles.list", map[string]any{
			"project_id": f.projectB,
		})
		f.assertScopeError(t, "cycles.list", msg)
	})

	// --- projects family ---
	t.Run("projects_get_rejects_other_project", func(t *testing.T) {
		msg := f.callExpectErr(t, "nottario.projects.get", map[string]any{
			"project_id": f.projectB,
		})
		f.assertScopeError(t, "projects.get", msg)
	})

	t.Run("projects_list_returns_only_bound_project", func(t *testing.T) {
		var out struct {
			Projects []map[string]any `json:"projects"`
		}
		f.callJSON(t, "nottario.projects.list", map[string]any{}, &out)
		if len(out.Projects) != 1 {
			t.Fatalf("expected exactly 1 project, got %d: %+v", len(out.Projects), out.Projects)
		}
		id, _ := out.Projects[0]["id"].(string)
		if id != f.projectA {
			t.Fatalf("expected project A %s, got %s", f.projectA, id)
		}
	})

	// --- whoami ---
	t.Run("whoami_filters_memberships_to_bound_project", func(t *testing.T) {
		var out struct {
			Memberships []map[string]any `json:"memberships"`
		}
		f.callJSON(t, "nottario.whoami", map[string]any{}, &out)
		if len(out.Memberships) == 0 {
			t.Fatalf("expected at least one membership, got none")
		}
		for _, m := range out.Memberships {
			pid, _ := m["project_id"].(string)
			if pid != f.projectA {
				t.Fatalf("expected memberships filtered to %s, found %s", f.projectA, pid)
			}
		}
	})
}

// TestHTTP_TokenProjectScope_Enforcement covers the HTTP API surface:
// a Bearer token bound to A targeting /api/projects/B/... must be
// rejected with 403 and a clear scope-mismatch message.
func TestHTTP_TokenProjectScope_Enforcement(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 99002, "http-scope", "HTTP Scope", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	pA, err := identity.CreateProject(ctx, pool, "HTTPScopeA", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject A: %v", err)
	}
	pB, err := identity.CreateProject(ctx, pool, "HTTPScopeB", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject B: %v", err)
	}
	tokenA, _, err := identity.IssueToken(ctx, pool, u.ID, pA.ID, "http-A", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Target B with the token bound to A.
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/projects/"+pB.ID.String()+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body["error"], pA.ID.String()) || !strings.Contains(body["error"], pB.ID.String()) {
		t.Fatalf("expected error mentioning both projects, got %q", body["error"])
	}

	// Sanity: same call against A succeeds.
	req2, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/projects/"+pA.ID.String()+"/tasks", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenA)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("Do A: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 against A, got %d", resp2.StatusCode)
	}
}
