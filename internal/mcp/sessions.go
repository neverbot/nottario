package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// sessionFields are embedded in the inputs of the tools that mark
// work as a conversation's: claim, claim_next and create {claim}.
type sessionFields struct {
	Session      string `json:"session,omitempty" jsonschema:"your conversation id: $CLAUDE_CODE_SESSION_ID in Claude Code; the same value for the whole conversation"`
	SessionLabel string `json:"session_label,omitempty" jsonschema:"short name for what this conversation is doing, shown to the human"`
}

// touchSession records the session when one is given and returns its
// id; nil when the input carries none.
func touchSession(ctx context.Context, req *sdk.CallToolRequest, d Deps, pid uuid.UUID, c identity.Caller, f sessionFields) (*uuid.UUID, error) {
	if f.Session == "" {
		return nil, nil
	}
	ref := agents.Ref{ProjectID: pid, UserID: c.UserID, Key: f.Session, Label: f.SessionLabel}
	if c.Source == identity.SourceToken {
		tid := c.TokenID
		ref.TokenID = &tid
	}
	ref.Client = clientName(req)
	s, err := agents.Touch(ctx, d.Pool, ref)
	if err != nil {
		return nil, err
	}
	return &s.ID, nil
}

// markActiveSession records the session on a task that was just
// claimed. A failure is reported as an error that says the claim stood.
func markActiveSession(ctx context.Context, req *sdk.CallToolRequest, d Deps, pid, taskID uuid.UUID, c identity.Caller, f sessionFields) error {
	sid, err := touchSession(ctx, req, d, pid, c, f)
	if err == nil && sid != nil {
		err = tasks.SetActiveSession(ctx, d.Pool, taskID, *sid)
	}
	if err != nil {
		return fmt.Errorf("the task was claimed, but recording the session failed: %w", err)
	}
	return nil
}

// clientName names the MCP client. The server runs stateless, so the
// initialize handshake is not seen by later calls and ClientInfo is
// normally empty; the product token of the User-Agent header
// ("claude-code/2.1" gives "claude-code") stands in for it.
func clientName(req *sdk.CallToolRequest) string {
	if req == nil {
		return ""
	}
	if req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
			return p.ClientInfo.Name
		}
	}
	if ex := req.GetExtra(); ex != nil && ex.Header != nil {
		ua := strings.TrimSpace(ex.Header.Get("User-Agent"))
		if i := strings.IndexAny(ua, "/ "); i > 0 {
			ua = ua[:i]
		}
		return ua
	}
	return ""
}
