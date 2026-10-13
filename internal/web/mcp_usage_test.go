package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
)

type usageResp struct {
	Days  int `json:"days"`
	Total struct {
		Calls, Errors, BytesIn, BytesOut int64
	} `json:"total"`
	Tools []struct {
		Tool     string `json:"tool"`
		Calls    int64  `json:"calls"`
		Errors   int64  `json:"errors"`
		BytesIn  int64  `json:"bytes_in"`
		BytesOut int64  `json:"bytes_out"`
	} `json:"tools"`
	Tokens []struct {
		Name  string `json:"name"`
		Calls int64  `json:"calls"`
	} `json:"tokens"`
	Others *struct {
		Calls int64 `json:"calls"`
	} `json:"others"`
}

// Every MCP call is counted per tool and token, with the bytes the
// agent sent and received, and the project page reads the totals.
func TestMCPUsage_RecordedAndReported(t *testing.T) {
	f := newMCPFixture(t, 14701, "usage-meter")
	var created map[string]any
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "count me"}, &created)
	f.callJSON(t, "nottario.tasks.create", map[string]any{"project_id": f.projectID, "title": "and me"}, nil)
	f.callExpectErr(t, "nottario.tasks.get", map[string]any{"project_id": f.projectID, "task_id": uuid.NewString()})
	listToolNames(t, f)

	get := func(auth string) usageResp {
		t.Helper()
		r := doRaw(t, "GET", f.baseURL+"/api/projects/"+f.projectID+"/mcp-usage?days=7", auth, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("usage: %d %s", r.StatusCode, r.Body)
		}
		var out usageResp
		if err := json.Unmarshal(r.Body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := get("Bearer " + f.token)
	tools := map[string]int{}
	for i, tl := range got.Tools {
		tools[tl.Tool] = i
	}
	c, ok := tools["nottario.tasks.create"]
	if !ok || got.Tools[c].Calls != 2 || got.Tools[c].BytesIn == 0 || got.Tools[c].BytesOut == 0 {
		t.Errorf("tasks.create usage: %+v", got.Tools)
	}
	if g, ok := tools["nottario.tasks.get"]; !ok || got.Tools[g].Errors != 1 {
		t.Errorf("failed tasks.get not counted as an error: %+v", got.Tools)
	}
	if l, ok := tools["tools/list"]; !ok || got.Tools[l].BytesOut < 1000 {
		t.Errorf("tools/list catalogue not measured: %+v", got.Tools)
	}
	if got.Days != 7 || len(got.Tokens) != 1 || got.Tokens[0].Name != "mcp" || got.Total.Calls < 4 {
		t.Errorf("summary: %+v", got)
	}

	// Another member sees the per-tool totals but not this user's token,
	// which is summed under others.
	pool := f.pgx(t)
	other, _, err := identity.UpsertFromGithub(f.ctx, pool, 14702, "usage-other", "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	roles, err := identity.ListRoles(f.ctx, pool, uuid.MustParse(f.projectID))
	if err != nil || len(roles) == 0 {
		t.Fatal(err)
	}
	if err := identity.AddMembership(f.ctx, pool, other.ID, uuid.MustParse(f.projectID), roles[0].ID); err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := identity.IssueToken(f.ctx, pool, other.ID, uuid.MustParse(f.projectID), "theirs", nil)
	if err != nil {
		t.Fatal(err)
	}
	theirs := get("Bearer " + otherToken)
	if len(theirs.Tokens) != 0 || theirs.Others == nil || theirs.Others.Calls != got.Total.Calls || len(theirs.Tools) == 0 {
		t.Errorf("other member's view: %+v", theirs)
	}

	if r := doRaw(t, "GET", f.baseURL+"/api/projects/"+f.projectID+"/mcp-usage?days=0", "Bearer "+f.token, nil); r.StatusCode != http.StatusBadRequest {
		t.Errorf("days=0: %d", r.StatusCode)
	}
}
