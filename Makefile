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

.PHONY: help build test run tidy docker lint check tools sqlc

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
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt -l reports unformatted files; run 'gofmt -w .'"; exit 1; }
	$(GO) vet ./...
	$(MAKE) lint
	$(MAKE) sqlc-check
	$(GO) test ./...

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
