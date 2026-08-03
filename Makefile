# mycel — Agent Orchestration System
#
# Structure:
#   build-local-*    Host machine binaries (Go, TS)
#   build-docker-*   Docker images (daemon, db, agents)
#   test-*           Tests
#   lint-*           Linters
#   check-*          Quality gates (lint + test)
#   run-*            Dev servers (foreground)
#   ci-*             CI pipelines
#
# Usage:
#   make build            Build everything (local + docker)
#   make build-local      Build local binaries only
#   make build-docker     Build Docker images only
#   make test             Run all tests
#   make check            Full quality gate
#   make clean            Remove artifacts

# =============================================================================
# .PHONY
# =============================================================================

.PHONY: help version
# Top-level
.PHONY: build build-local build-docker test lint fmt vet check clean deps release install
# Go
.PHONY: build-local-mycel build-local-desktop test-go test-go-race test-go-fast lint-go fmt-go vet-go coverage-go bench-go deps-go check-go scan-go
.PHONY: release-local-mycel install-local-mycel
# Docker
.PHONY: build-docker-daemon build-docker-db
.PHONY: build-docker-agent-base build-docker-agent build-docker-agents build-docker-agent-infra build-docker-playwright stop-docker-playwright run-docker-playwright
# TS
.PHONY: build-local-web build-local-landing
.PHONY: test-ts test-web test-web-unit test-web-e2e test-landing
.PHONY: lint-ts lint-web lint-landing
.PHONY: fmt-ts fmt-web fmt-landing
.PHONY: vet-ts vet-web vet-landing
.PHONY: coverage-ts bench-ts deps-ts check-ts scan-ts
.PHONY: run-mycel run-web run-landing
# CI
.PHONY: ci-local ci-docker
# Clean
.PHONY: clean-local clean-deps

.DEFAULT_GOAL := help

# =============================================================================
# Variables
# =============================================================================

# Semver derived from git tags — see scripts/version.sh for the exact shapes.
# Source builds and release builds deliberately produce the same format so that
# `mycel version`, /api/health and the About page's update check never have to
# know which kind of build they are looking at.
# Evaluated once, when make starts, rather than on first use: a build writes into
# the tree (server/web/dist, wails.json) and a recursively-expanded VERSION would
# be computed after those writes, letting a build's own side effects decide
# whether it calls itself dirty. `ifeq` keeps `make VERSION=... ` working.
ifeq ($(origin VERSION),undefined)
VERSION := $(shell sh scripts/version.sh)
endif
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_DIR ?= bin
GO ?= go

REGISTRY ?= mycel
IMAGE_TAG ?= latest
AGENT_PROVIDERS := claude agy codex cursor openclaw pi

LDFLAGS_VERSION = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# CFBundleShortVersionString has to be a plain X.Y.Z, so the macOS bundle gets
# the numeric core while the precise build string still reaches the app through
# -X main.version. The release workflow reduces its tag the same way, through the
# same script, so the two cannot disagree about an edge case.
VERSION_CORE = $(shell sh scripts/version.sh --core '$(VERSION)')

# Official mycel builds embed the registered Google "Desktop app" OAuth
# client so Gmail "Sign in with Google" works zero-setup. GOOGLE_OAUTH_CLIENT_ID
# / GOOGLE_OAUTH_CLIENT_SECRET are sourced from the environment (maintainer
# exports them locally, or CI reads them from repo Actions secrets); when
# unset both ldflags args are empty and the built binary behaves exactly as
# before (one-click sign-in reports unconfigured, manual paste path still
# works). Nothing secret is ever committed — see pkg/gateway/gmail/oauth.go.
GMAIL_PKG := github.com/rpuneet/mycel/pkg/gateway/gmail
LDFLAGS_GMAIL = -X '$(GMAIL_PKG).defaultGoogleClientID=$(GOOGLE_OAUTH_CLIENT_ID)' -X '$(GMAIL_PKG).defaultGoogleClientSecret=$(GOOGLE_OAUTH_CLIENT_SECRET)'

LDFLAGS_RELEASE = -s -w $(LDFLAGS_VERSION) $(LDFLAGS_GMAIL)

_CYAN  := \033[36m
_GREEN := \033[32m
_RED   := \033[31m
_RESET := \033[0m
_BOLD  := \033[1m

# =============================================================================
# Help
# =============================================================================

help: ## Show all targets
	@echo "mycel — Agent Orchestration System ($(VERSION))"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo ""

version: ## Show version info
	@echo "Version: $(VERSION)  Commit: $(COMMIT)  Date: $(DATE)"

# =============================================================================
# Top-level aggregates
# =============================================================================

build: build-local build-docker ## Build everything (local + docker)
build-local: build-local-go build-local-ts ## Build local binaries (go + ts)
build-docker: build-docker-db build-docker-daemon build-docker-playwright ## Build Docker images (db, daemon, playwright)

test: test-go test-ts ## Run all tests
lint: lint-go lint-ts ## Run all linters
fmt: fmt-go fmt-ts ## Format all code
vet: vet-go vet-ts ## Vet all code
check: check-go check-ts ## Full quality gate
deps: deps-go deps-ts ## Install all dependencies
release: release-local-mycel ## Build release binaries (stripped)
install: install-local-mycel ## Install mycel to $GOPATH/bin
clean: clean-local ## Remove all build artifacts

# =============================================================================
# Build — Local Go
# =============================================================================

build-local-go: build-local-mycel ## Build all Go binaries

build-local-mycel: build-local-web ## Build mycel (embeds web UI, server)
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f server/web/dist/index.html ]; then mkdir -p server/web/dist && echo "<!-- stub -->" > server/web/dist/index.html; fi
	$(GO) build -ldflags="$(LDFLAGS_VERSION) $(LDFLAGS_GMAIL)" -o $(BUILD_DIR)/mycel ./cmd/mycel


# wails reads the bundle version from wails.json rather than a flag, so the
# committed placeholder is swapped for the real one and restored afterwards —
# without this a locally built .app advertises the placeholder in Finder and
# About This Mac while the binary inside reports the true version. The release
# workflow does the same substitution.
build-local-desktop: build-local-web ## Build desktop app for the host OS (requires wails CLI)
	cd desktop && cp wails.json wails.json.orig && \
		trap 'mv -f wails.json.orig wails.json' EXIT INT TERM && \
		sed -i.bak 's/"productVersion": "[^"]*"/"productVersion": "$(VERSION_CORE)"/' wails.json && rm -f wails.json.bak && \
		wails build -ldflags "$(LDFLAGS_VERSION) $(LDFLAGS_GMAIL)"


# =============================================================================
# Build — Local TypeScript
# =============================================================================

build-local-ts: build-local-web build-local-landing ## Build all TS packages


build-local-web: ## Build web UI → server/web/dist/
	cd web && bun install && bun run build
	@mkdir -p server/web
	@rm -rf server/web/dist
	@cp -r web/dist server/web/dist
	# server/web/dist/placeholder.txt is tracked (see .gitignore) so that a fresh
	# checkout has a non-empty directory for //go:embed to accept. The rm above
	# takes it with the rest of dist, and leaving it deleted marks the worktree
	# dirty — which then shows up in the version string of every later build, so a
	# clean checkout of a tag stops reporting itself as that release.
	@printf 'placeholder\n' > server/web/dist/placeholder.txt

build-local-landing: ## Build landing page
	cd landing && bun install && bun run build

# =============================================================================
# Build — Docker
# =============================================================================

build-docker-daemon: ## Build daemon Docker image
	docker build -t $(REGISTRY)-daemon:$(IMAGE_TAG) -f docker/Dockerfile.daemon .

build-docker-db: ## Build mycel-db (unified TimescaleDB) Docker image
	docker build -t $(REGISTRY)-db:$(IMAGE_TAG) -f docker/Dockerfile.db .


build-docker-agent-base: ## Build agent base image
	docker build -t $(REGISTRY)-agent-base:$(IMAGE_TAG) -f docker/Dockerfile.base .

build-docker-agent: build-docker-agent-base ## Build default agent image (claude)
	docker build -t $(REGISTRY)-agent-claude:$(IMAGE_TAG) -f docker/Dockerfile.claude .

build-docker-agent-%: build-docker-agent-base ## Build agent image for provider
	docker build -t $(REGISTRY)-agent-$*:$(IMAGE_TAG) -f docker/Dockerfile.$* .

build-docker-agents: build-docker-agent-base ## Build all agent images
	@for p in $(AGENT_PROVIDERS); do \
		echo "Building $(REGISTRY)-agent-$$p..."; \
		docker build -t $(REGISTRY)-agent-$$p:$(IMAGE_TAG) -f docker/Dockerfile.$$p . || exit 1; \
	done

build-docker-agent-infra: build-docker-agent ## Build infra agent image (extends claude)
	docker build -t $(REGISTRY)-agent-infra:$(IMAGE_TAG) -f docker/Dockerfile.infra .

build-docker-playwright: ## Build Playwright MCP Docker image (separate from main build)
	docker build -t mycel-playwright:latest -f docker/Dockerfile.playwright .

stop-docker-playwright: ## Stop and remove Playwright container
	docker stop mycel-playwright 2>/dev/null || true
	docker rm mycel-playwright 2>/dev/null || true

run-docker-playwright: stop-docker-playwright ## Run Playwright MCP container (VNC :6080, MCP :3000)
	docker run -d --name mycel-playwright \
		--init --ipc=host \
		-p 3000:3000 -p 6080:6080 \
		-v mycel-shared-tmp:/tmp/mycel-shared \
		-e DISPLAY=:99 \
		--restart unless-stopped \
		mycel-playwright:latest
	@echo "  Playwright MCP: http://localhost:3000/sse"
	@echo "  VNC viewer:     http://localhost:6080"

# =============================================================================
# Test
# =============================================================================

test-go: ## Run Go tests with race detector
	$(GO) test -race ./...

test-go-race: test-go ## Alias for test-go (always uses -race)

test-go-fast: ## Run Go tests excluding slow packages
	# NOTE: Keep SLOW list in sync with .github/workflows/ci.yml "Run fast tests" step
	$(GO) test -race $$($(GO) list ./... | grep -v -F "$$(printf 'github.com/rpuneet/mycel/pkg/tmux\ngithub.com/rpuneet/mycel/pkg/secret\ngithub.com/rpuneet/mycel/pkg/doctor\ngithub.com/rpuneet/mycel/internal/cmd')")

test-ts: test-web test-landing ## Run all TS tests


test-web: ## Run web UI tests
	cd web && bun install && bun run test

test-web-unit: test-web ## Alias for test-web (vitest unit suite)

test-web-e2e: ## Run web e2e tests (needs a running daemon)
	cd web && bunx playwright test --config=e2e/playwright.config.ts

test-landing: ## Run landing tests (no-op: no tests configured yet)
	@echo "⚠ landing: no tests configured (skipping)"

coverage-go: ## Go test coverage
	$(GO) test -race -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

bench-go: ## Go benchmarks
	$(GO) test -bench=. -benchmem -count=1 ./...

coverage-ts: ## TS test coverage
	cd web && bun run test -- --coverage 2>/dev/null || true

bench-ts: ## TS benchmarks (no-op)
	@true

# =============================================================================
# Lint & Format
# =============================================================================

lint-go: ## Lint Go code
	golangci-lint run ./...

fmt-go: ## Format Go code
	find . -name '*.go' -not -path './.mycel/*' -not -path './vendor/*' | xargs gofmt -s -w

vet-go: ## Vet Go code
	$(GO) vet ./...

lint-ts: lint-web lint-landing ## Lint all TS
lint-web: ; cd web && bun run lint
lint-landing: ; cd landing && bun run lint

fmt-ts: fmt-web fmt-landing ## Format all TS
fmt-web: ; cd web && bunx prettier --write "src/**/*.{ts,tsx,css}"
fmt-landing: ; cd landing && bunx prettier --write "src/**/*.{ts,tsx,css}"

vet-ts: vet-web vet-landing ## Typecheck all TS
vet-web: ; cd web && bunx tsc -b --noEmit
vet-landing: ; cd landing && bunx tsc --noEmit

# =============================================================================
# Check & CI
# =============================================================================

check-go: vet-go lint-go test-go ## Go quality gate
check-ts: vet-ts lint-ts test-ts ## TS quality gate

ci-local: ## Full CI pipeline locally
	@printf "\n$(_BOLD)mycel CI$(_RESET) ($(VERSION))\n\n"
	@FAIL=0; \
	printf "$(_CYAN)[go]$(_RESET) deps\n";    $(MAKE) --no-print-directory deps-go       || FAIL=1; \
	printf "$(_CYAN)[go]$(_RESET) check\n";   $(MAKE) --no-print-directory check-go      || FAIL=1; \
	printf "$(_CYAN)[go]$(_RESET) build\n";   $(MAKE) --no-print-directory build-local-go || FAIL=1; \
	printf "\n"; \
	printf "$(_CYAN)[ts]$(_RESET) deps\n";    $(MAKE) --no-print-directory deps-ts       || FAIL=1; \
	printf "$(_CYAN)[ts]$(_RESET) check\n";   $(MAKE) --no-print-directory check-ts      || FAIL=1; \
	printf "$(_CYAN)[ts]$(_RESET) build\n";   $(MAKE) --no-print-directory build-local-ts || FAIL=1; \
	printf "\n"; \
	if [ $$FAIL -eq 0 ]; then printf "$(_GREEN)$(_BOLD)CI PASSED$(_RESET)\n\n"; \
	else printf "$(_RED)$(_BOLD)CI FAILED$(_RESET)\n\n"; exit 1; fi

ci-docker: ## Build all Docker images
	@printf "\n$(_BOLD)mycel Docker CI$(_RESET)\n\n"
	@FAIL=0; \
	printf "$(_CYAN)[docker]$(_RESET) db\n";       $(MAKE) --no-print-directory build-docker-db         || FAIL=1; \
	printf "$(_CYAN)[docker]$(_RESET) daemon\n";      $(MAKE) --no-print-directory build-docker-daemon       || FAIL=1; \
	printf "$(_CYAN)[docker]$(_RESET) agents\n";   $(MAKE) --no-print-directory build-docker-agents    || FAIL=1; \
	printf "\n"; \
	if [ $$FAIL -eq 0 ]; then printf "$(_GREEN)$(_BOLD)Docker CI PASSED$(_RESET)\n\n"; \
	else printf "$(_RED)$(_BOLD)Docker CI FAILED$(_RESET)\n\n"; exit 1; fi

# =============================================================================
# Release
# =============================================================================

release-local-mycel: ## Build optimized mycel binary (embeds web UI)
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f server/web/dist/index.html ]; then mkdir -p server/web/dist && echo "<!-- stub -->" > server/web/dist/index.html; fi
	$(GO) build -ldflags="$(LDFLAGS_RELEASE)" -o $(BUILD_DIR)/mycel ./cmd/mycel
	# LDFLAGS_RELEASE already includes LDFLAGS_GMAIL (see Variables section).


# =============================================================================
# Run (dev, foreground)
# =============================================================================

run-mycel: ## Run mycel CLI from source
	$(GO) run ./cmd/mycel


run-web: ## Run web UI dev server
	cd web && bun run dev

run-landing: ## Run landing dev server
	cd landing && bun run dev


build-landing-prod: ## Production build for landing page (Cloudflare Pages)
	cd landing && bun install && bun run build

# =============================================================================
# Install
# =============================================================================

install-local-mycel: build-local-mycel ## Install mycel to $GOPATH/bin
	cp $(BUILD_DIR)/mycel $(shell $(GO) env GOPATH)/bin/


# =============================================================================
# Dependencies
# =============================================================================

deps-go: ## Go dependencies
	$(GO) mod download && $(GO) mod tidy

deps-ts: ## TS dependencies
	cd web && bun install
	cd landing && bun install

# =============================================================================
# Security
# =============================================================================

scan-go: ## Go vulnerability scan
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

scan-ts: ## TS dependency audit
	cd web && bun audit || true
	cd landing && bun audit || true

# =============================================================================
# Clean
# =============================================================================

clean-local: ## Remove build artifacts
	rm -rf $(BUILD_DIR)/ dist/ coverage.out coverage.html
	rm -rf web/dist server/web/dist landing/.next landing/out

clean-deps: clean-local ## Remove artifacts + node_modules
	rm -rf web/node_modules landing/node_modules
