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

.PHONY: help build test run tidy docker

help:
	@echo "Targets:"
	@echo "  build   - build the nottario binary"
	@echo "  test    - run the test suite"
	@echo "  run     - build and run locally (requires DATABASE_URL)"
	@echo "  tidy    - go mod tidy"
	@echo "  docker  - build the docker image"

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o ./bin/nottario ./cmd/nottario

test:
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
