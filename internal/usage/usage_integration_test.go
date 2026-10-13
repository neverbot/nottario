package usage_test

import (
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
	"github.com/neverbot/nottario/internal/usage"
)

func TestRecordSummariseAndPrune(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 14801, "usage-unit", "U", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "Usage unit", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, tok, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []usage.Call{
		{ProjectID: p.ID, TokenID: tok.ID, Tool: "nottario.tasks.get", BytesIn: 10, BytesOut: 300},
		{ProjectID: p.ID, TokenID: tok.ID, Tool: "nottario.tasks.get", BytesIn: 10, BytesOut: 200, Failed: true},
		{ProjectID: p.ID, TokenID: tok.ID, Tool: "nottario.docs.stat", BytesIn: 5, BytesOut: 50},
	} {
		if err := usage.Record(ctx, pool, c); err != nil {
			t.Fatal(err)
		}
	}
	s, err := usage.Summarise(ctx, pool, p.ID, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Total.Calls != 3 || s.Total.BytesOut != 550 || s.Total.Errors != 1 {
		t.Errorf("total = %+v", s.Total)
	}
	if len(s.Tools) != 2 || s.Tools[0].Tool != "nottario.tasks.get" || s.Tools[0].Calls != 2 || s.Tools[0].BytesIn != 20 {
		t.Errorf("tools = %+v", s.Tools)
	}
	if len(s.Tokens) != 1 || s.Tokens[0].Name != "agent" || len(s.Tokens[0].Tools) != 2 {
		t.Errorf("tokens = %+v", s.Tokens)
	}
	// Today's rows survive a prune; a prune far in the future drops them.
	if n, err := usage.Prune(ctx, pool, time.Now()); err != nil || n != 0 {
		t.Errorf("prune now: %d %v", n, err)
	}
	if n, err := usage.Prune(ctx, pool, time.Now().Add(usage.Retention+48*time.Hour)); err != nil || n != 2 {
		t.Errorf("prune later: %d %v", n, err)
	}
}
