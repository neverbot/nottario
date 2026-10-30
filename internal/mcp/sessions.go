package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// sessionFields are embedded in the inputs of the tools that mark
// work as a conversation's: claim, claim_next, create {claim} and
// start_step.
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

// checkSession rejects a malformed session before anything is written.
// An absent session is fine: nothing is recorded.
func checkSession(f sessionFields) error {
	if f.Session == "" {
		return nil
	}
	_, err := agents.NormaliseKey(f.Session)
	return err
}

// markActiveSession records the session on a task that was just
// claimed. It returns the failure text, empty on success or when there
// is no session: the claim already happened, so a failure here must not
// turn the call into a tool error (agents retry errors, and a retry
// would claim or create a second task).
func markActiveSession(ctx context.Context, req *sdk.CallToolRequest, d Deps, pid, taskID uuid.UUID, c identity.Caller, f sessionFields) string {
	sid, err := touchSession(ctx, req, d, pid, c, f)
	if err == nil && sid != nil {
		var ok bool
		ok, err = tasks.SetActiveSession(ctx, d.Pool, taskID, *sid, c.UserID)
		if err == nil && !ok {
			return "the task left doing or changed hands before the session was recorded"
		}
	}
	if err != nil {
		return "the task was claimed, but recording the session failed: " + err.Error()
	}
	return ""
}

// withSessionError adds session_error to a JSON object payload; the
// payload is returned untouched when msg is empty.
func withSessionError(payload any, msg string) any {
	if msg == "" {
		return payload
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return payload
	}
	m["session_error"] = msg
	return m
}

// clientName names the MCP client. The server runs stateless, so the
// initialize handshake is not seen by later calls and ClientInfo is
// normally empty; the product token of the User-Agent header
// ("claude-code/2.1" gives "claude-code", reduced to [A-Za-z0-9._-]) stands in for it.
func clientName(req *sdk.CallToolRequest) string {
	if req == nil {
		return ""
	}
	if req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
			return safeToken(p.ClientInfo.Name)
		}
	}
	if ex := req.GetExtra(); ex != nil && ex.Header != nil {
		ua := strings.TrimSpace(ex.Header.Get("User-Agent"))
		if i := strings.IndexAny(ua, "/ "); i >= 0 {
			ua = ua[:i]
		}
		return safeToken(ua)
	}
	return ""
}

// safeToken reduces s to [A-Za-z0-9._-].
func safeToken(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return -1
	}, s)
}
