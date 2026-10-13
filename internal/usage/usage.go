// Package usage records and reports how much MCP traffic each project's
// agents generate: calls and bytes in and out per day, token and tool.
// Bytes in an agent's context are what Nottario costs it, so this is
// the number a team watches to see which agents and tools spend most.
package usage

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Call is one MCP request worth recording.
type Call struct {
	ProjectID uuid.UUID
	TokenID   uuid.UUID
	Tool      string // tool name, or "tools/list"
	Failed    bool
	BytesIn   int
	BytesOut  int
}

// Record adds one call to today's (UTC) counters.
func Record(ctx context.Context, db dbq.DBTX, c Call) error {
	if c.ProjectID == uuid.Nil || c.TokenID == uuid.Nil || c.Tool == "" {
		return nil
	}
	var errs int64
	if c.Failed {
		errs = 1
	}
	return dbq.New(db).RecordMcpUsage(ctx, dbq.RecordMcpUsageParams{
		ProjectID: c.ProjectID,
		TokenID:   c.TokenID,
		Tool:      c.Tool,
		Errors:    errs,
		BytesIn:   int64(c.BytesIn),
		BytesOut:  int64(c.BytesOut),
	})
}

// Counters are the totals for one tool or one token over a window.
type Counters struct {
	Calls    int64 `json:"calls"`
	Errors   int64 `json:"errors"`
	BytesIn  int64 `json:"bytes_in"`
	BytesOut int64 `json:"bytes_out"`
}

func (c *Counters) add(r dbq.ListMcpUsageRowsRow) {
	c.Calls += r.Calls
	c.Errors += r.Errors
	c.BytesIn += r.BytesIn
	c.BytesOut += r.BytesOut
}

// ToolUsage is one tool's totals.
type ToolUsage struct {
	Tool string `json:"tool"`
	Counters
}

// TokenUsage is one token's totals, with its heaviest tools.
type TokenUsage struct {
	TokenID uuid.UUID   `json:"token_id"`
	Name    string      `json:"name"`
	UserID  uuid.UUID   `json:"user_id"`
	Revoked bool        `json:"revoked"`
	Tools   []ToolUsage `json:"tools"`
	Counters
}

// Summary is a project's usage over the last Days days.
type Summary struct {
	Days   int          `json:"days"`
	Since  string       `json:"since"`
	Total  Counters     `json:"total"`
	Tools  []ToolUsage  `json:"tools"`
	Tokens []TokenUsage `json:"tokens"`
}

// Summarise returns a project's usage over the last days days (today
// included), per tool and per token, heaviest output first.
func Summarise(ctx context.Context, db dbq.DBTX, projectID uuid.UUID, days int, now time.Time) (*Summary, error) {
	if days < 1 {
		days = 1
	}
	since := now.UTC().AddDate(0, 0, -(days - 1))
	rows, err := dbq.New(db).ListMcpUsageRows(ctx, dbq.ListMcpUsageRowsParams{
		ProjectID: projectID,
		Since:     pgtype.Date{Time: time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	s := &Summary{Days: days, Since: since.Format("2006-01-02"), Tools: []ToolUsage{}, Tokens: []TokenUsage{}}
	tools := map[string]*ToolUsage{}
	tokens := map[uuid.UUID]*TokenUsage{}
	for _, r := range rows {
		s.Total.add(r)
		tu, ok := tools[r.Tool]
		if !ok {
			tu = &ToolUsage{Tool: r.Tool}
			tools[r.Tool] = tu
		}
		tu.add(r)
		tk, ok := tokens[r.TokenID]
		if !ok {
			tk = &TokenUsage{TokenID: r.TokenID, Name: r.TokenName, UserID: r.TokenUserID, Revoked: r.TokenRevoked, Tools: []ToolUsage{}}
			tokens[r.TokenID] = tk
		}
		tk.add(r)
		one := ToolUsage{Tool: r.Tool}
		one.add(r)
		tk.Tools = append(tk.Tools, one)
	}
	for _, t := range tools {
		s.Tools = append(s.Tools, *t)
	}
	for _, t := range tokens {
		sortTools(t.Tools)
		s.Tokens = append(s.Tokens, *t)
	}
	sortTools(s.Tools)
	sort.Slice(s.Tokens, func(i, j int) bool {
		return s.Tokens[i].BytesOut+s.Tokens[i].BytesIn > s.Tokens[j].BytesOut+s.Tokens[j].BytesIn
	})
	return s, nil
}

func sortTools(ts []ToolUsage) {
	sort.Slice(ts, func(i, j int) bool {
		a, b := ts[i].BytesOut+ts[i].BytesIn, ts[j].BytesOut+ts[j].BytesIn
		if a != b {
			return a > b
		}
		return ts[i].Tool < ts[j].Tool
	})
}

// Retention is how long daily rows are kept.
const Retention = 180 * 24 * time.Hour

// Prune deletes rows older than Retention.
func Prune(ctx context.Context, db dbq.DBTX, now time.Time) (int64, error) {
	cut := now.UTC().Add(-Retention)
	return dbq.New(db).DeleteMcpUsageBefore(ctx, pgtype.Date{Time: time.Date(cut.Year(), cut.Month(), cut.Day(), 0, 0, 0, 0, time.UTC), Valid: true})
}

// RunPruner deletes expired rows once at start and then daily until ctx
// ends.
func RunPruner(ctx context.Context, db dbq.DBTX, logf func(msg string, args ...any)) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if _, err := Prune(ctx, db, time.Now()); err != nil && ctx.Err() == nil {
			logf("mcp usage prune failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
