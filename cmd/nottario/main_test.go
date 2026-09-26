package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/testutil"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setEnv(t *testing.T, dsn string) {
	t.Helper()
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("PUBLIC_URL", "http://127.0.0.1:0")
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "cid")
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET", "secret")
	t.Setenv("SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	// Port 0: the kernel picks one, and run() reports it back.
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("NOTTARIO_BACKUP_DIR", "")
	t.Setenv("SELF_UPDATE_CHECK_ENABLED", "false")
}

// The binary's start-up path: read the environment, open the pool,
// run migrations, wire every subsystem and serve. Nothing else
// exercises this file, and a mistake here is a container that boots
// into a 500 or does not boot at all.
func TestRun_BootsAndServes(t *testing.T) {
	dsn := testutil.NewDSN(t)
	setEnv(t, dsn)

	ctx, cancel := context.WithCancel(t.Context())
	addrs := make(chan net.Addr, 1)
	errs := make(chan error, 1)
	go func() { errs <- run(ctx, quietLogger(), func(a net.Addr) { addrs <- a }) }()

	var addr net.Addr
	select {
	case addr = <-addrs:
	case err := <-errs:
		t.Fatalf("run exited before serving: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("the server never came up")
	}
	base := "http://" + addr.String()

	// Health, version and the embedded UI all served by the same
	// process that just migrated the database.
	for _, tc := range []struct{ path, want string }{
		{"/healthz", "ok"},
		{"/version", ""},
		{"/skill", "nottario"},
		{"/static/styles.css", ""},
	} {
		resp, err := http.Get(base + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d: %s", tc.path, resp.StatusCode, body)
		}
		if tc.want != "" && !strings.Contains(strings.ToLower(string(body)), tc.want) {
			t.Errorf("GET %s returned %.60s, expected it to mention %q", tc.path, body, tc.want)
		}
	}

	// Migrations really ran against the fresh database.
	resp, err := http.Get(base + "/api/projects")
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous /api/projects = %d, want 401 from a working server", resp.StatusCode)
	}

	// A signal (here, a cancelled context) shuts it down cleanly.
	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Errorf("shutdown returned %v, want a clean exit", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the server did not shut down")
	}
}

// Start-up failures have to be loud and specific: a container that
// exits with a vague message is a container nobody can fix.
func TestRun_RefusesToStartOnBadConfig(t *testing.T) {
	cases := []struct {
		name, unset, set, value, want string
	}{
		{name: "no database url", unset: "DATABASE_URL", want: "DATABASE_URL"},
		{name: "no session key", unset: "SESSION_KEY", want: "SESSION_KEY"},
		{name: "session key too short", set: "SESSION_KEY", value: base64.StdEncoding.EncodeToString([]byte("short")), want: "32 bytes"},
		{name: "session key not base64", set: "SESSION_KEY", value: "!!!not base64!!!", want: "base64"},
		{name: "no oauth client", unset: "GITHUB_OAUTH_CLIENT_ID", want: "GITHUB_OAUTH_CLIENT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, "postgres://nobody@127.0.0.1:1/none")
			if tc.unset != "" {
				t.Setenv(tc.unset, "")
			}
			if tc.set != "" {
				t.Setenv(tc.set, tc.value)
			}
			err := run(t.Context(), quietLogger(), nil)
			if err == nil {
				t.Fatal("started with a broken configuration")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			// Exit code 2 is reserved for configuration problems.
			if !isConfigError(err) {
				t.Errorf("%v is not reported as a configuration error", err)
			}
		})
	}
}

func isConfigError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "config: ")
}

// A database that is not there is a different failure from a bad
// environment, and must not be mistaken for one.
func TestRun_ReportsAnUnreachableDatabase(t *testing.T) {
	setEnv(t, "postgres://nobody:nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	err := run(t.Context(), quietLogger(), nil)
	if err == nil {
		t.Fatal("started without a database")
	}
	if isConfigError(err) {
		t.Errorf("an unreachable database was reported as a config error: %v", err)
	}
	if !strings.Contains(err.Error(), "db open") && !strings.Contains(err.Error(), "db migrate") {
		t.Errorf("error does not say the database is the problem: %v", err)
	}
}
