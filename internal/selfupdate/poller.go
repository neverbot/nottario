// Package selfupdate polls a GitHub repository for the current
// master commit SHA and exposes the result so an admin-only banner
// in the web UI can notify the operator that a newer image is
// available. The poller itself never touches the container — it is
// purely informational.
//
// Disabled with SELF_UPDATE_CHECK_ENABLED=false. When enabled, one
// goroutine wakes up on start, performs an immediate check, and then
// re-checks every SELF_UPDATE_CHECK_INTERVAL. Failures are logged at
// warn level and the loop continues; the previous good sha stays
// visible via Snapshot until the next successful check.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// MinInterval clamps SELF_UPDATE_CHECK_INTERVAL from below to avoid
// hammering the GitHub API. Anonymous requests share a 60 req/h/IP
// bucket; one check per hour leaves plenty of headroom for anything
// else on the host.
const MinInterval = time.Hour

// DefaultInterval is the check cadence when the env var is unset.
const DefaultInterval = 24 * time.Hour

// DefaultUpstream is the "owner/repo" checked when
// SELF_UPDATE_UPSTREAM is unset.
const DefaultUpstream = "neverbot/nottario"

// State holds the last-known result from the poller. Zero-value
// State is safe: LatestSHA empty + CheckedAt zero means "no
// successful check yet"; the endpoint reports update_available=false
// in that case.
type State struct {
	mu        sync.RWMutex
	latestSHA string
	checkedAt time.Time
	lastErr   string
}

// Snapshot returns the current view of the state. Safe for concurrent
// callers.
func (s *State) Snapshot() (latestSHA string, checkedAt time.Time, lastErr string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestSHA, s.checkedAt, s.lastErr
}

func (s *State) setSuccess(sha string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestSHA = sha
	s.checkedAt = at
	s.lastErr = ""
}

func (s *State) setError(err string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkedAt = at
	s.lastErr = err
}

// Poller checks a GitHub repository for its current master SHA on a
// fixed interval. Construct with New and drive with Start.
type Poller struct {
	upstream string
	interval time.Duration
	httpc    *http.Client
	baseURL  string // override in tests
	state    *State
	logger   *slog.Logger
	now      func() time.Time
}

// Config wires the poller. Interval below MinInterval is clamped up.
// Empty Upstream falls back to DefaultUpstream.
type Config struct {
	Upstream string
	Interval time.Duration
	Logger   *slog.Logger
	// BaseURL overrides the GitHub API host — set only in tests.
	BaseURL string
	// Now overrides the clock; nil = time.Now.
	Now func() time.Time
}

// New builds a poller. It does NOT start; call Start(ctx) once
// after New.
func New(c Config) *Poller {
	if c.Upstream == "" {
		c.Upstream = DefaultUpstream
	}
	if c.Interval < MinInterval {
		c.Interval = MinInterval
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.github.com"
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return &Poller{
		upstream: c.Upstream,
		interval: c.Interval,
		httpc:    &http.Client{Timeout: 5 * time.Second},
		baseURL:  c.BaseURL,
		state:    &State{},
		logger:   c.Logger,
		now:      c.Now,
	}
}

// State exposes the poller's state so the endpoint can read the
// current sha + checked_at.
func (p *Poller) State() *State { return p.state }

// Upstream returns the configured "owner/repo" so the endpoint can
// echo it back for debugging.
func (p *Poller) Upstream() string { return p.upstream }

// Interval returns the configured check cadence (post-clamp).
func (p *Poller) Interval() time.Duration { return p.interval }

// Start blocks until ctx is cancelled. Runs one check immediately
// on entry, then ticks. Safe to call from a goroutine.
func (p *Poller) Start(ctx context.Context) {
	p.logger.Info("self-update poller enabled",
		"upstream", p.upstream, "interval", p.interval.String())
	p.checkOnce(ctx)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.checkOnce(ctx)
		}
	}
}

func (p *Poller) checkOnce(ctx context.Context) {
	sha, err := p.fetchLatestSHA(ctx)
	now := p.now()
	if err != nil {
		p.logger.Warn("self-update check failed", "err", err)
		p.state.setError(err.Error(), now)
		return
	}
	p.state.setSuccess(sha, now)
}

// commitsResponse is the subset of the GitHub commits API we need.
type commitsResponse struct {
	SHA string `json:"sha"`
}

func (p *Poller) fetchLatestSHA(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/master", p.baseURL, p.upstream)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nottario-selfupdate")
	resp, err := p.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github returned %d: %s", resp.StatusCode, string(body))
	}
	var out commitsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.SHA == "" {
		return "", errors.New("github response missing sha")
	}
	return out.SHA, nil
}
