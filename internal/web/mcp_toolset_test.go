package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

func listToolNames(t *testing.T, f *mcpFixture) map[string]bool {
	t.Helper()
	res, err := f.session.ListTools(f.ctx, &sdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	out := map[string]bool{}
	for _, tl := range res.Tools {
		out[tl.Name] = true
	}
	return out
}

// A connection narrowed with ?tools= and ?shell= sees, and can call,
// only its toolset; without parameters it sees everything.
func TestMCP_Toolset(t *testing.T) {
	full := listToolNames(t, newMCPFixture(t, 14601, "toolset-full"))
	for _, n := range []string{"nottario.arch.list_nodes", "nottario.docs.read", "nottario.docs.write", "nottario.tasks.get"} {
		if !full[n] {
			t.Errorf("default catalogue lacks %s", n)
		}
	}

	f := newMCPFixtureQuery(t, 14602, "toolset-narrow", "?tools=tasks,docs&shell=true")
	got := listToolNames(t, f)
	for n := range got {
		if !strings.HasPrefix(n, "nottario.tasks.") && !strings.HasPrefix(n, "nottario.docs.") && n != "nottario.whoami" {
			t.Errorf("narrowed catalogue carries %s", n)
		}
	}
	for _, n := range []string{"nottario.tasks.get", "nottario.docs.download_url", "nottario.docs.upload_url", "nottario.whoami"} {
		if !got[n] {
			t.Errorf("narrowed catalogue lacks %s", n)
		}
	}
	for _, n := range []string{"nottario.docs.read", "nottario.docs.write", "nottario.arch.list_nodes"} {
		if got[n] {
			t.Errorf("narrowed catalogue still shows %s", n)
		}
	}
	if _, err := f.session.CallTool(f.ctx, &sdk.CallToolParams{
		Name: "nottario.docs.read", Arguments: map[string]any{"project_id": f.projectID, "path": "x.md"},
	}); err == nil {
		t.Error("a hidden tool can still be called")
	}
	var who map[string]any
	f.callJSON(t, "nottario.whoami", map[string]any{}, &who)
}

func TestMCP_ToolsetRejectsUnknownDomain(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14603, "toolset-bad", "Bad", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Toolset bad", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)
	r := doRaw(t, "POST", ts.URL+"/mcp?tools=nope", "Bearer "+token, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(string(r.Body), "tasks") {
		t.Errorf("unknown domain: %d %s", r.StatusCode, r.Body)
	}
}
