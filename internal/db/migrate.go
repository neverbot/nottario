package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending up migrations against the given pool.
// SQL migrations are embedded into the binary; Go migrations register
// themselves from this package (see migrate_*.go).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return withGoose(pool, func(sqlDB *sql.DB) error {
		if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
			return fmt.Errorf("goose up: %w", err)
		}
		return nil
	})
}

// MigrateTo moves the schema up or down to exactly version. Tests use
// it to set up the state a migration starts from.
func MigrateTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	return withGoose(pool, func(sqlDB *sql.DB) error {
		current, err := goose.GetDBVersionContext(ctx, sqlDB)
		if err != nil {
			return fmt.Errorf("goose version: %w", err)
		}
		if version < current {
			if err := goose.DownToContext(ctx, sqlDB, "migrations", version); err != nil {
				return fmt.Errorf("goose down to %d: %w", version, err)
			}
			return nil
		}
		if err := goose.UpToContext(ctx, sqlDB, "migrations", version); err != nil {
			return fmt.Errorf("goose up to %d: %w", version, err)
		}
		return nil
	})
}

func withGoose(pool *pgxpool.Pool, fn func(*sql.DB) error) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	return fn(sqlDB)
}
