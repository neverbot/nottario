package selfupdate

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_ClampsIntervalToMinimum(t *testing.T) {
	p := New(Config{Interval: 5 * time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if got := p.Interval(); got != MinInterval {
		t.Fatalf("Interval() = %v, want clamp to %v", got, MinInterval)
	}
}

func TestNew_DefaultsWhenZeroConfig(t *testing.T) {
	p := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if p.Upstream() != DefaultUpstream {
		t.Fatalf("Upstream() = %q, want %q", p.Upstream(), DefaultUpstream)
	}
	if p.Interval() != MinInterval {
		t.Fatalf("Interval() = %v, want clamp of zero to %v", p.Interval(), MinInterval)
	}
}

func TestFetchLatestSHA_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/neverbot/nottario/commits/master" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sha":"abcdef1234567890"}`))
	}))
	defer srv.Close()

	p := New(Config{
		Upstream: "neverbot/nottario",
		Interval: MinInterval,
		BaseURL:  srv.URL,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	p.checkOnce(context.Background())
	sha, at, errStr := p.State().Snapshot()
	if sha != "abcdef1234567890" {
		t.Fatalf("sha = %q, want abcdef1234567890", sha)
	}
	if at.IsZero() {
		t.Fatalf("checkedAt is zero, want set")
	}
	if errStr != "" {
		t.Fatalf("lastErr = %q, want empty", errStr)
	}
}

func TestFetchLatestSHA_NetworkErrorKeepsPriorState(t *testing.T) {
	// First serve a success to seed state, then swap to 500 and confirm
	// the sha stays and lastErr populates.
	respCode := http.StatusOK
	body := `{"sha":"first"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(respCode)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(Config{
		Upstream: "neverbot/nottario",
		Interval: MinInterval,
		BaseURL:  srv.URL,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	p.checkOnce(context.Background())
	sha1, _, _ := p.State().Snapshot()
	if sha1 != "first" {
		t.Fatalf("seed sha = %q, want first", sha1)
	}

	respCode = http.StatusInternalServerError
	body = "boom"
	p.checkOnce(context.Background())
	sha2, _, errStr := p.State().Snapshot()
	if sha2 != "first" {
		t.Fatalf("sha after failure = %q, want unchanged first", sha2)
	}
	if errStr == "" {
		t.Fatalf("lastErr empty after failed check, want populated")
	}
}

func TestNotifier_FiresOnTransitionOnly(t *testing.T) {
	respCode := http.StatusOK
	body := `{"sha":"first"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(respCode)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var calls int
	p := New(Config{
		BaseURL:  srv.URL,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Notifier: func() { calls++ },
	})
	// First check: empty → "first". One call.
	p.checkOnce(context.Background())
	if calls != 1 {
		t.Fatalf("calls after first success = %d, want 1", calls)
	}
	// No-op re-check: same SHA, no error. Zero additional calls.
	p.checkOnce(context.Background())
	if calls != 1 {
		t.Fatalf("calls after no-op recheck = %d, want 1", calls)
	}
	// Server flips SHA. One additional call.
	body = `{"sha":"second"}`
	p.checkOnce(context.Background())
	if calls != 2 {
		t.Fatalf("calls after sha flip = %d, want 2", calls)
	}
	// Error transition: fresh failure. One additional call.
	respCode = http.StatusInternalServerError
	body = "boom"
	p.checkOnce(context.Background())
	if calls != 3 {
		t.Fatalf("calls after error = %d, want 3", calls)
	}
	// Same error again: no call.
	p.checkOnce(context.Background())
	if calls != 3 {
		t.Fatalf("calls after repeat error = %d, want 3", calls)
	}
}

func TestFetchLatestSHA_MissingSHAIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := New(Config{
		BaseURL: srv.URL,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	p.checkOnce(context.Background())
	sha, _, errStr := p.State().Snapshot()
	if sha != "" {
		t.Fatalf("sha = %q, want empty on missing-sha response", sha)
	}
	if errStr == "" {
		t.Fatalf("lastErr empty, want populated")
	}
}
