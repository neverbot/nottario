# Nottario — common developer commands.
#
# Run `make` with no arguments to see the available targets.

SHELL := /bin/bash
GO ?= go

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/neverbot/nottario/internal/version.Version=$(VERSION) \
	-X github.com/neverbot/nottario/internal/version.Commit=$(COMMIT) \
	-X github.com/neverbot/nottario/internal/version.Date=$(DATE)

# Pinned tool versions so `make check` is reproducible across machines.
GOLANGCI_LINT_VERSION ?= v1.62.2
SQLC_VERSION          ?= v1.31.1

# Where 'go install' drops binaries (works inside and outside CI).
GOBIN ?= $(shell $(GO) env GOPATH)/bin

.PHONY: help build test run tidy docker lint check tools sqlc docs-build docs-serve docs-check js-check frontend-check frontend-format

help:
	@echo "Targets:"
	@echo "  build   - build the nottario binary"
	@echo "  test    - run the test suite"
	@echo "  lint    - golangci-lint with gosec G201/G202 (SQL injection guard)"
	@echo "  check   - fmt + vet + lint + test (the pre-commit gate)"
	@echo "  tools   - install the linters required by 'make lint'"
	@echo "  run     - build and run locally (requires DATABASE_URL)"
	@echo "  tidy    - go mod tidy"
	@echo "  docker  - build the docker image"

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o ./bin/nottario ./cmd/nottario

test:
	$(GO) test ./...

# Integration tests need a Postgres reachable as a privileged role (it
# CREATE/DROPs a fresh database per test package). Defaults to the
# `db` service started by `docker compose up -d db` and assumes the
# port mapping in compose.yml is left at 5432:5432. Override with
# TEST_DATABASE_URL when running elsewhere.
TEST_DATABASE_URL ?= postgres://nottario:nottario@localhost:5432/postgres?sslmode=disable
test-integration:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) $(GO) test ./...

tools:
	@command -v $(GOBIN)/golangci-lint >/dev/null 2>&1 \
		|| $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@command -v $(GOBIN)/sqlc >/dev/null 2>&1 \
		|| $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

# Regenerate type-safe Go from internal/db/queries/*.sql against the
# migrations schema. Run after editing any .sql file; commit the
# generated internal/db/dbq/* alongside the change.
sqlc: tools
	$(GOBIN)/sqlc generate

# Verifies the committed sqlc output is in sync with the .sql sources
# (CI-friendly: fails if someone changed a query but forgot to regenerate).
sqlc-check: tools
	$(GOBIN)/sqlc diff

lint: tools
	$(GOBIN)/golangci-lint run ./...
	$(GO) run ./internal/tools/sqlcheck ./...

# Pre-commit gate documented in .claude/claude.md. Run before every
# `git commit`. Order matches the doc: gofmt clean, vet clean, lint
# clean (incl. gosec G201/G202 against SQL string formatting), tests
# green. The SQL injection surface is covered by gosec G201 (format
# string) and G202 (string concatenation) — sqlvet is intentionally
# NOT used: its last release (v1.2.0) is unmaintained and crashes on
# modern Go. Tier 2 of the SQL safety feature (sqlc migration) will
# eliminate the underlying hand-written SQL surface entirely.
#
# Integration tests need a Postgres reachable as a privileged role and
# would silently t.Skip without TEST_DATABASE_URL — turning the "gate"
# into a false green for anything that touches the DB. We pass the
# Makefile's default DSN (the dev `db` compose service) so that, with
# the container up, integration tests actually run; without it, they
# fail loud at connect instead of being skipped. CI sets its own DSN.
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt -l reports unformatted files; run 'gofmt -w .'"; exit 1; }
	$(GO) vet ./...
	$(MAKE) lint
	$(MAKE) sqlc-check
	$(MAKE) docs-check
	$(MAKE) js-check
	$(MAKE) frontend-check
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) $(GO) test ./...

# Frontend syntax gate. The project ships vanilla JS / Lit without a
# build step, so a stray backtick inside a `css\`...\`` comment or any
# similar typo otherwise surfaces only at runtime in the browser. We
# run `node --check` (parse-only, no execution, no deps installed)
# over every `.js` under internal/web/static so the same class of bug
# fails the gate locally and in CI. The neighbouring package.json
# declares `"type": "module"` so Node parses each file as ESM.
js-check:
	@command -v node >/dev/null 2>&1 || { echo "node is required for js-check (install Node 20+)"; exit 1; }
	@find internal/web/static -name '*.js' -not -path '*/vendor/*' -print0 \
		| xargs -0 -I{} node --check {}

# Frontend lint + format gate via Biome. The Rust binary is cached
# under `~/.npm/_npx/` after the first invocation, so no global
# install is needed and the repo carries no `node_modules`. The check
# variant fails on lint errors; warnings stay visible but do not
# block the gate. The format variant rewrites files in place.
frontend-check:
	@command -v npx >/dev/null 2>&1 || { echo "npx is required for frontend-check (install Node 20+)"; exit 1; }
	npx --yes @biomejs/biome check internal/web/static

frontend-format:
	@command -v npx >/dev/null 2>&1 || { echo "npx is required for frontend-format (install Node 20+)"; exit 1; }
	npx --yes @biomejs/biome check --write internal/web/static

# Documentation site (cmd/nottario-docs + docs/site/content).
# `docs-build` produces a working static site under docs/site/dist.
# `docs-serve` runs a local HTTP server over it for previewing.
# `docs-check` validates the markdown corpus and runs in CI as part
# of `make check`.
docs-build:
	$(GO) run ./cmd/nottario-docs --in docs/site/content --out docs/site/dist

docs-serve: docs-build
	@python3 -m http.server -d docs/site/dist 8000

docs-check:
	$(GO) run ./cmd/nottario-docs --check --in docs/site/content

run: build
	./bin/nottario

tidy:
	$(GO) mod tidy

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t nottario:$(VERSION) .
