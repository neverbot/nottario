// Package fixtures feeds sqlcheck's own test. It lives under
// testdata so `go build ./...` and the linters skip it: the unsafe
// calls below are the point, not an oversight.
//
// Every call the analyser must flag carries a `// WANT` comment on
// the line the diagnostic is reported at. The test compares that set
// against what the analyser actually reports, so a check that stops
// firing, or starts firing on safe code, fails loudly.
package fixtures

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const channel = "nottario_events"

// ---- unsafe: these must be flagged -------------------------------

func interpolatedName(ctx context.Context, pool *pgxpool.Pool, name string) {
	_, _ = pool.Exec(ctx, fmt.Sprintf("DELETE FROM users WHERE name = '%s'", name)) // WANT
}

func concatenatedID(ctx context.Context, pool *pgxpool.Pool, id string) {
	_, _ = pool.Query(ctx, "SELECT * FROM tasks WHERE id = '"+id+"'") // WANT
}

func interpolatedQueryRow(ctx context.Context, pool *pgxpool.Pool, table string) {
	_ = pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)) // WANT
}

func insideATransaction(ctx context.Context, pool *pgxpool.Pool, order string) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, _ = tx.Query(ctx, "SELECT id FROM tasks ORDER BY "+order) // WANT
}

// ---- safe: these must NOT be flagged -----------------------------

func placeholders(ctx context.Context, pool *pgxpool.Pool, id string) {
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
}

// The placeholder-index pattern: only integers reach the format.
func placeholderIndex(ctx context.Context, pool *pgxpool.Pool, idx int) {
	_, _ = pool.Query(ctx, fmt.Sprintf("SELECT id FROM tasks WHERE project_id = $%d", idx), idx)
}

// A compile-time constant is not runtime input.
func constantConcat(ctx context.Context, pool *pgxpool.Pool) {
	_, _ = pool.Exec(ctx, "LISTEN "+channel)
}

// The escape hatch, for SQL that cannot take placeholders.
func deliberateException(ctx context.Context, pool *pgxpool.Pool, dbName string) {
	_, _ = pool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)) //sqlcheck:ignore test helper
}

// Not pgx: an unrelated type with a Query method must not be touched.
type notPgx struct{}

func (notPgx) Query(ctx context.Context, q string) string { return q }

func unrelatedQueryMethod(ctx context.Context, search string) string {
	var client notPgx
	return client.Query(ctx, "find: "+search)
}
