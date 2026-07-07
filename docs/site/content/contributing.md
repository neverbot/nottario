---
title: Contributing
section: Reference
nav_order: 3
---

# Contributing

Nottario is a single Go binary with a vanilla web frontend, no
build step. Getting a change from clone to green PR takes about
five minutes of setup once the toolchain is in place.

## Toolchain

- **Go** (matching the version in `go.mod`).
- **Docker** and **Docker Compose** — for the local Postgres, and
  for the end-to-end smoke of the container image.
- **Node** — only invoked transiently via `npx` for Biome; no
  `node_modules` in the repo.

Pinned tool versions live in the `Makefile`. `make tools` installs
everything the check gate needs (goose, sqlc, golangci-lint) into
`$(go env GOBIN)`.

## Run the local stack

```
docker compose up -d --build nottario
```

Brings up the container at `http://localhost:8080` plus its own
Postgres 16 in the `db-data` volume. See
[Getting started](/getting-started/) for the OAuth App and `.env`
wiring — the same setup applies to a dev checkout.

After editing Go or `internal/web/static/`:

```
docker compose up -d --build nottario
```

The frontend is loaded straight off disk from the running
container, so a page refresh in the browser is enough after a
rebuild.

## The `make check` gate

Every commit runs the same chain that CI enforces:

```
TEST_DATABASE_URL='postgres://nottario:nottario@localhost:5432/postgres?sslmode=disable' \
  make check
```

The chain, in order:

1. `gofmt -l .` — must list no files. Fix with `gofmt -w .`.
2. `go vet ./...` — must pass clean.
3. `make lint` — `golangci-lint` (gosec, govet, staticcheck,
   ineffassign, unused, errcheck, gofmt) **plus** the in-tree
   `internal/tools/sqlcheck` analyzer that flags any `fmt.Sprintf`
   or string concatenation of runtime values feeding pgx's
   `Query`/`Exec`/`QueryRow`.
4. `make sqlc-check` — `sqlc diff` confirms `internal/db/dbq/*.sql.go`
   matches `internal/db/queries/*.sql`. Regenerate with `make sqlc`
   after editing queries; commit both.
5. `make docs-check` — smoke-builds this docs site so a broken
   shortcode or template lands here, not on the deployed site.
6. `make js-check` — `node --check` (parse-only) over every `.js`
   in `internal/web/static/`. Catches structural breakage like
   unbalanced brackets or template literals.
7. `make frontend-check` — Biome (lint + format) via
   `npx --yes @biomejs/biome`. Config in `biome.json` at the repo
   root; `make frontend-format` rewrites files in place.
8. `go test ./...` — every package, including concurrency and
   integration tests.

Never bypass with `--no-verify` or by skipping packages — the
sqlcheck analyzer is the actual SQL-injection guard.

## SQL: the sqlc workflow

All new queries land in `internal/db/queries/<domain>.sql`. Each
starts with a comment header:

```sql
-- name: ListUserRoleIDsInProject :many
SELECT role_id FROM membership_roles
WHERE user_id = $1 AND project_id = $2;
```

The `:kind` is one of `one`, `many`, `exec`, `execrows`. Regenerate
with `make sqlc`; commit the generated `internal/db/dbq/*.sql.go`
alongside the SQL. CI runs `sqlc diff` (step 4 of `make check`) and
a drift fails the gate.

Conventions:

- **Named args over positional** when a query has more than ~3
  parameters or any optional input. Use `sqlc.arg('name')::<type>`
  for required, `sqlc.narg('name')::<type>` for optional; always
  cast so Postgres can infer the type. Don't mix positional and
  named in the same query.
- **Optional filters** are expressed inline —
  `(sqlc.narg('foo')::text IS NULL OR col = sqlc.narg('foo')::text)`
  — rather than building SQL dynamically in Go.
- **UUID overrides** in `sqlc.yaml` map `uuid` → `uuid.UUID` and
  nullable `uuid` → `*uuid.UUID`. Don't reach for `pgtype.UUID`.
- **Nullable text/timestamps** surface as `pgtype.Text` /
  `pgtype.Timestamptz`. Convert at the package boundary; keep
  `pgtype` out of domain types.
- **Transactions**: `dbq.New(tx)` binds queries to an open `pgx.Tx`.
  Always `defer func() { _ = tx.Rollback(ctx) }()` immediately after
  `Begin`.
- **No raw pgx fallback.** If something feels hard to express in
  sqlc, add a named query — don't drop back to inline SQL. The
  `sqlcheck` analyzer will flag it.

Advisory locks (`pg_advisory_xact_lock`), `FOR UPDATE [SKIP LOCKED]`
and `WITH RECURSIVE` cycle checks all live inside sqlc queries
today — see `dependencies.sql`, `tasks.sql`, `arch.sql` for the
patterns.

## Migrations

Goose files under `internal/db/migrations/`, embedded into the
binary at build time. Booting the container is the migration.
Every migration ships an `+goose Up` and a `+goose Down` block;
Down is best-effort but expected to leave a valid schema.

## Frontend

Vanilla ES modules + Lit web components under
`internal/web/static/`, served straight off disk by the running
container. No bundler, no build step. Every Lit component's shadow
DOM is isolated: global `styles.css` rules do not penetrate, so
each component needs its own `box-sizing: border-box` in its
shadow styles.

## Docs

You are reading them. Markdown under
`docs/site/content/*.md` is compiled to HTML by
`cmd/nottario-docs`; the templates live in
`cmd/nottario-docs/templates/`. `make docs-check` builds the site
in check mode; `make docs-serve` runs a local preview.

Whats-new entries land in
`docs/site/content/whats-new.md` — one bullet per user-visible
change, past tense, dated headings newest first. Small bug fixes,
refactors and dependency bumps don't need an entry; `git log`
carries those.

## Commits

Single-line [Conventional Commits](https://www.conventionalcommits.org/).
No body, no trailers, no `Co-Authored-By`. The primary branch is
`master`.
