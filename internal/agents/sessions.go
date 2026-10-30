// Package agents records the agent conversations working in a
// project. A session is keyed by an id the client supplies, so the
// human can find the conversation again; some clients can reopen
// it from its id.
package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/neverbot/nottario/internal/db/dbq"
)

// Session is one agent conversation in a project.
type Session struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	UserID     uuid.UUID  `json:"user_id"`
	TokenID    *uuid.UUID `json:"-"`
	Key        string     `json:"key"`
	Label      string     `json:"label"`
	Client     string     `json:"client"`
	LastSeenAt time.Time  `json:"last_seen_at"`
}

const (
	// MaxKeyLen is the longest session key, in characters.
	MaxKeyLen = 128
	// MaxLabelLen is the longest session label, in characters.
	MaxLabelLen = 80
	// MaxClientLen is the longest client name, in characters.
	MaxClientLen = 80
)

// NormaliseKey trims a session key and checks its length. Callers use
// it to reject a bad key before they write anything.
func NormaliseKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if n := utf8.RuneCountInString(key); n < 1 || n > MaxKeyLen {
		return "", fmt.Errorf("session must be 1 to %d characters", MaxKeyLen)
	}
	return key, nil
}

// Ref identifies a session to create or refresh.
type Ref struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	TokenID   *uuid.UUID
	Key       string
	Label     string
	Client    string
}

func fromRow(r dbq.AgentSession) Session {
	return Session{
		ID:         r.ID,
		ProjectID:  r.ProjectID,
		UserID:     r.UserID,
		TokenID:    r.TokenID,
		Key:        r.SessionKey,
		Label:      r.Label,
		Client:     r.Client,
		LastSeenAt: r.LastSeenAt.Time,
	}
}

func truncRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Touch creates or refreshes the session and returns it.
func Touch(ctx context.Context, db dbq.DBTX, r Ref) (*Session, error) {
	key, err := NormaliseKey(r.Key)
	if err != nil {
		return nil, err
	}
	row, err := dbq.New(db).UpsertAgentSession(ctx, dbq.UpsertAgentSessionParams{
		ProjectID:  r.ProjectID,
		TokenID:    r.TokenID,
		UserID:     r.UserID,
		SessionKey: key,
		Label:      truncRunes(strings.TrimSpace(r.Label), MaxLabelLen),
		Client:     truncRunes(strings.TrimSpace(r.Client), MaxClientLen),
	})
	if err != nil {
		return nil, err
	}
	s := fromRow(dbq.AgentSession(row))
	return &s, nil
}

// ByIDs returns the sessions with these ids, keyed by id.
func ByIDs(ctx context.Context, db dbq.DBTX, ids []uuid.UUID) (map[uuid.UUID]Session, error) {
	out := make(map[uuid.UUID]Session, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := dbq.New(db).GetAgentSessionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = fromRow(dbq.AgentSession(r))
	}
	return out, nil
}

// Find returns the user's session with this key in the project, or nil
// when there is none. Unlike Touch it never creates one.
func Find(ctx context.Context, db dbq.DBTX, projectID, userID uuid.UUID, key string) (*Session, error) {
	key, err := NormaliseKey(key)
	if err != nil {
		return nil, err
	}
	row, err := dbq.New(db).GetAgentSessionByKey(ctx, dbq.GetAgentSessionByKeyParams{
		ProjectID: projectID, UserID: userID, SessionKey: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := fromRow(row)
	return &s, nil
}
