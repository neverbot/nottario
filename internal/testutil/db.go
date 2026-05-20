// Package testutil wires integration tests against a real Postgres
// instance. Each test package gets its own freshly-migrated database
// keyed by an env var (TEST_DATABASE_URL) so the production binary
// never pulls in heavyweight test infrastructure.
package testutil

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db"
)

// EnvDSN is the env var read by NewPool to locate the test database.
// It must point at a Postgres role that can CREATE/DROP databases —
// the test helper provisions a fresh database per test package and
// drops it on cleanup.
const EnvDSN = "TEST_DATABASE_URL"

// NewPool returns a pool connected to a freshly-provisioned,
// fully-migrated database. The database is dropped when t finishes.
// If TEST_DATABASE_URL is not set, the test is skipped (so `go test
// ./...` stays a clean no-op without infrastructure).
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skipf("%s not set; skipping integration test", EnvDSN)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminDSN, dbName, err := provision(ctx, dsn, t.Name())
	if err != nil {
		t.Fatalf("provision test db: %v", err)
	}

	pool, err := db.Open(ctx, dbName)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("migrate test db: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := drop(ctx, adminDSN, dbName); err != nil {
			t.Logf("drop test db: %v", err)
		}
	})
	return pool
}

// provision opens the admin DSN, creates a uniquely-named database
// derived from the test name + a nanosecond suffix, and returns the
// DSN pointing at the new database. The original admin DSN (with its
// path stripped) is returned so cleanup can DROP DATABASE.
func provision(ctx context.Context, dsn, testName string) (adminDSN, newDSN string, err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = conn.Close(ctx) }()

	name := sanitize(testName) + "_" + nanoSuffix()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgIdent(name)); err != nil { //sqlcheck:ignore: identifier is whitelisted by sanitize()+pgIdent; DDL has no $N form
		return "", "", err
	}

	adminDSN = dsn
	uCopy := *u
	uCopy.Path = "/" + name
	newDSN = uCopy.String()
	return adminDSN, newDSN, nil
}

func drop(ctx context.Context, adminDSN, newDSN string) error {
	u, err := url.Parse(newDSN)
	if err != nil {
		return err
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	// Force-drop in case lingering connections still hold the db.
	_, _ = conn.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", dbName)
	_, err = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgIdent(dbName)) //sqlcheck:ignore: identifier is whitelisted by sanitize()+pgIdent; DDL has no $N form
	return err
}

func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	if out == "" {
		out = "test"
	}
	return "nott_t_" + out
}

func nanoSuffix() string {
	now := time.Now().UnixNano()
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for now > 0 {
		i--
		buf[i] = digits[now%10]
		now /= 10
	}
	return string(buf[i:])
}

// pgIdent quotes an identifier safely for use in DDL. The name is
// derived from the test name and a nanosecond suffix, so it is
// already constrained to [a-z0-9_]+, but we still defend against
// embedded quotes.
func pgIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
