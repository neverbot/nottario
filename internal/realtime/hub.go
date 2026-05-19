// Package realtime delivers database mutations to the browser via
// Server-Sent Events. Mutations enter via Postgres LISTEN/NOTIFY (a
// trigger calls pg_notify('nottario_events', payload)). One goroutine
// holds a long-lived LISTEN on that channel and fans events out to
// every connected subscriber that cares about the affected project.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// channel is the Postgres NOTIFY channel name. The migration creates
// triggers that publish to it.
const channel = "nottario_events"

// Event is the parsed form of one notification payload.
type Event struct {
	Type      string     `json:"type"`
	Op        string     `json:"op,omitempty"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
	TaskID    *uuid.UUID `json:"task_id,omitempty"`
	Scope     string     `json:"scope,omitempty"`
	Path      string     `json:"path,omitempty"`
}

// Hub is the in-process event bus. Subscribers receive a buffered
// channel; if a subscriber falls behind and its buffer fills, the
// hub drops the slowest event (we prefer fresh state over backlog).
type Hub struct {
	logger *slog.Logger

	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

type subscriber struct {
	projectID uuid.UUID
	ch        chan Event
}

// New constructs a Hub. The logger may be nil.
func New(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{logger: logger, subs: make(map[*subscriber]struct{})}
}

// Subscribe registers a new SSE subscriber listening for events in
// projectID. The returned channel receives Event values; the
// returned cancel function deregisters the subscriber.
func (h *Hub) Subscribe(projectID uuid.UUID) (<-chan Event, func()) {
	s := &subscriber{
		projectID: projectID,
		ch:        make(chan Event, 64),
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[s]; ok {
			delete(h.subs, s)
			close(s.ch)
		}
		h.mu.Unlock()
	}
	return s.ch, cancel
}

// publish fans an event out to interested subscribers. Non-blocking.
func (h *Hub) publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if ev.ProjectID == nil || *ev.ProjectID != s.projectID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// subscriber is slow; drop the event to avoid blocking the
			// listener goroutine.
			h.logger.Warn("dropping event for slow subscriber",
				"type", ev.Type, "project", s.projectID)
		}
	}
}

// Run holds a long-lived LISTEN on the Postgres channel and feeds
// events into the hub. It returns when ctx is cancelled or when the
// underlying connection errors permanently.
func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool) error {
	for {
		err := h.runOnce(ctx, pool)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}
		h.logger.Error("listener loop failed; retrying", "err", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *Hub) runOnce(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	h.logger.Info("realtime listener attached", "channel", channel)

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if err := json.Unmarshal([]byte(n.Payload), &ev); err != nil {
			h.logger.Warn("malformed notify payload", "payload", n.Payload, "err", err)
			continue
		}
		h.publish(ev)
	}
}
