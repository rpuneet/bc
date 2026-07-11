# mycel — Agent Orchestration System
#
# Structure:
#   build-local-*    Host machine binaries (Go, TS)
#   build-docker-*   Docker images (bcd, db, agents)
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
.PHONY: build-local-bc build-local-mycel build-local-tui-bundle test-go test-go-race test-go-fast lint-go fmt-go vet-go coverage-go bench-go deps-go check-go scan-go
.PHONY: release-local-bc release-local-mycel install-local-bc install-local-mycel
# Docker
.PHONY: build-docker-daemon build-docker-db build-docker-bcdb
.PHONY: build-docker-agent-base build-docker-agent build-docker-agents build-docker-agent-infra build-docker-playwright stop-docker-playwright run-docker-playwright
# TS
.PHONY: build-local-tui build-local-web build-local-landing
.PHONY: test-ts test-tui test-web test-web-unit test-web-e2e test-landing
.PHONY: lint-ts lint-tui lint-web lint-landing
.PHONY: fmt-ts fmt-tui fmt-web fmt-landing
.PHONY: vet-ts vet-tui vet-web vet-landing
.PHONY: coverage-ts bench-ts deps-ts check-ts scan-ts
.PHONY: run-bc run-mycel run-web run-landing run-tui
# CI
.PHONY: ci-local ci-docker
# Clean
.PHONY: clean-local clean-deps

.DEFAULT_GOAL := help

# =============================================================================
# Variables
# =============================================================================

VERSION ?= $(shell date -u +%Y.%m.%d).$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_DIR ?= bin
GO ?= go

REGISTRY ?= mycel
LEGACY_REGISTRY ?= bc
IMAGE_TAG ?= latest
AGENT_PROVIDERS := claude gemini codex cursor openclaw

LDFLAGS_VERSION = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
LDFLAGS_RELEASE = -s -w $(LDFLAGS_VERSION)

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
build-docker: build-docker-db build-docker-daemon build-docker-playwright ## Build Docker images (db, bcd, playwright)

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

build-local-mycel: build-local-web build-local-tui-bundle ## Build mycel (embeds web UI, TUI bundle, server)
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f server/web/dist/index.html ]; then mkdir -p server/web/dist && echo "<!-- stub -->" > server/web/dist/index.html; fi
	$(GO) build -ldflags="$(LDFLAGS_VERSION)" -o $(BUILD_DIR)/mycel ./cmd/mycel

build-local-bc: build-local-mycel ## Deprecated alias for build-local-mycel

build-local-tui-bundle: ## Build single-file TUI bundle for embedding into mycel binary
	@mkdir -p internal/cmd/tui-bundle
	@if [ ! -f tui/node_modules/.package-lock.json ] && [ ! -d tui/node_modules/react-devtools-core ]; then \
		echo "Installing TUI dependencies..."; \
		cd tui && bun install; \
	fi
	cd tui && bun build --target=bun --minify \
		--external react-devtools-core --external yoga-wasm-web \
		--outfile=../internal/cmd/tui-bundle/index.js \
		src/index.tsx

# =============================================================================
# Build — Local TypeScript
# =============================================================================

build-local-ts: build-local-tui build-local-web build-local-landing ## Build all TS packages

build-local-tui: ## Build TUI
	cd tui && bun install && bun run build

build-local-web: ## Build web UI → server/web/dist/
	cd web && bun install && bun run build
	@rm -rf server/web/dist
	@cp -r web/dist server/web/dist

build-local-landing: ## Build landing page
	cd landing && bun install && bun run build

# =============================================================================
# Build — Docker
# =============================================================================

build-docker-daemon: ## Build bcd Docker image
	docker build -t $(REGISTRY)-daemon:$(IMAGE_TAG) -f docker/Dockerfile.bcd .

build-docker-db: ## Build bc-db (unified TimescaleDB) Docker image
	docker build -t $(REGISTRY)-bcdb:$(IMAGE_TAG) -f docker/Dockerfile.bcdb .

build-docker-bcdb: build-docker-db ## Alias: Build bcdb (Postgres) Docker image

build-docker-agent-base: ## Build agent base image
	docker build -t $(REGISTRY)-agent-base:$(IMAGE_TAG) -f docker/Dockerfile.base .
	docker tag $(REGISTRY)-agent-base:$(IMAGE_TAG) $(LEGACY_REGISTRY)-agent-base:$(IMAGE_TAG)

build-docker-agent: build-docker-agent-base ## Build default agent image (claude)
	docker build -t $(REGISTRY)-agent-claude:$(IMAGE_TAG) -f docker/Dockerfile.claude .
	docker tag $(REGISTRY)-agent-claude:$(IMAGE_TAG) $(LEGACY_REGISTRY)-agent-claude:$(IMAGE_TAG)

build-docker-agent-%: build-docker-agent-base ## Build agent image for provider
	docker build -t $(REGISTRY)-agent-$*:$(IMAGE_TAG) -f docker/Dockerfile.$* .
	docker tag $(REGISTRY)-agent-$*:$(IMAGE_TAG) $(LEGACY_REGISTRY)-agent-$*:$(IMAGE_TAG)

build-docker-agents: build-docker-agent-base ## Build all agent images
	@for p in $(AGENT_PROVIDERS); do \
		echo "Building $(REGISTRY)-agent-$$p..."; \
		docker build -t $(REGISTRY)-agent-$$p:$(IMAGE_TAG) -f docker/Dockerfile.$$p . || exit 1; \
		docker tag $(REGISTRY)-agent-$$p:$(IMAGE_TAG) $(LEGACY_REGISTRY)-agent-$$p:$(IMAGE_TAG); \
	done

build-docker-agent-infra: build-docker-agent ## Build infra agent image (extends claude)
	docker build -t $(REGISTRY)-agent-infra:$(IMAGE_TAG) -f docker/Dockerfile.infra .
	docker tag $(REGISTRY)-agent-infra:$(IMAGE_TAG) $(LEGACY_REGISTRY)-agent-infra:$(IMAGE_TAG)

build-docker-playwright: ## Build Playwright MCP Docker image (separate from main build)
	docker build -t bc-playwright:latest -f docker/Dockerfile.playwright .

stop-docker-playwright: ## Stop and remove Playwright container
	docker stop bc-playwright 2>/dev/null || true
	docker rm bc-playwright 2>/dev/null || true

run-docker-playwright: stop-docker-playwright ## Run Playwright MCP container (VNC :6080, MCP :3000)
	docker run -d --name bc-playwright \
		--init --ipc=host \
		-p 3000:3000 -p 6080:6080 \
		-v bc-shared-tmp:/tmp/bc-shared \
		-e DISPLAY=:99 \
		--restart unless-stopped \
		bc-playwright:latest
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
	$(GO) test -race $$($(GO) list ./... | grep -v -F "$$(printf 'github.com/gh-curious-otter/bc/pkg/tmux\ngithub.com/gh-curious-otter/bc/pkg/secret\ngithub.com/gh-curious-otter/bc/pkg/doctor\ngithub.com/gh-curious-otter/bc/internal/cmd')")

test-ts: test-tui test-web test-landing ## Run all TS tests

test-tui: ## Run TUI tests
	cd tui && bun install && CI=true bun test

test-web: ## Run web UI tests
	cd web && bun install && bun run test

test-web-unit: test-web ## Alias for test-web (vitest unit suite)

test-web-e2e: ## Run web e2e tests (needs running bcd)
	cd web && bunx playwright test --config=e2e/playwright.config.ts

test-landing: ## Run landing tests (no-op: no tests configured yet)
	@echo "⚠ landing: no tests configured (skipping)"

coverage-go: ## Go test coverage
	$(GO) test -race -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

bench-go: ## Go benchmarks
	$(GO) test -bench=. -benchmem -count=1 ./...

coverage-ts: ## TS test coverage
	cd tui && bun test --coverage || true
	cd web && bun run test -- --coverage 2>/dev/null || true

bench-ts: ## TS benchmarks (no-op)
	@true

# =============================================================================
# Lint & Format
# =============================================================================

lint-go: ## Lint Go code
	golangci-lint run ./...

fmt-go: ## Format Go code
	find . -name '*.go' -not -path './.bc/*' -not -path './vendor/*' | xargs gofmt -s -w

vet-go: ## Vet Go code
	$(GO) vet ./...

lint-ts: lint-tui lint-web lint-landing ## Lint all TS
lint-tui: ; cd tui && bun run lint
lint-web: ; cd web && bun run lint
lint-landing: ; cd landing && bun run lint

fmt-ts: fmt-tui fmt-web fmt-landing ## Format all TS
fmt-tui: ; cd tui && bunx prettier --write "src/**/*.{ts,tsx}"
fmt-web: ; cd web && bunx prettier --write "src/**/*.{ts,tsx,css}"
fmt-landing: ; cd landing && bunx prettier --write "src/**/*.{ts,tsx,css}"

vet-ts: vet-tui vet-web vet-landing ## Typecheck all TS
vet-tui: ; cd tui && bun run typecheck
vet-web: ; cd web && bunx tsc -b --noEmit
vet-landing: ; cd landing && bunx tsc --noEmit

# =============================================================================
# Check & CI
# =============================================================================

check-go: vet-go lint-go test-go ## Go quality gate
check-ts: vet-ts lint-ts test-ts ## TS quality gate

ci-local: ## Full CI pipeline locally
	@printf "\n$(_BOLD)bc CI$(_RESET) ($(VERSION))\n\n"
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
	@printf "\n$(_BOLD)bc Docker CI$(_RESET)\n\n"
	@FAIL=0; \
	printf "$(_CYAN)[docker]$(_RESET) db\n";       $(MAKE) --no-print-directory build-docker-db         || FAIL=1; \
	printf "$(_CYAN)[docker]$(_RESET) bcd\n";      $(MAKE) --no-print-directory build-docker-daemon       || FAIL=1; \
	printf "$(_CYAN)[docker]$(_RESET) agents\n";   $(MAKE) --no-print-directory build-docker-agents    || FAIL=1; \
	printf "\n"; \
	if [ $$FAIL -eq 0 ]; then printf "$(_GREEN)$(_BOLD)Docker CI PASSED$(_RESET)\n\n"; \
	else printf "$(_RED)$(_BOLD)Docker CI FAILED$(_RESET)\n\n"; exit 1; fi

# =============================================================================
# Release
# =============================================================================

release-local-mycel: build-local-tui-bundle ## Build optimized mycel binary (embeds web + TUI)
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f server/web/dist/index.html ]; then mkdir -p server/web/dist && echo "<!-- stub -->" > server/web/dist/index.html; fi
	$(GO) build -ldflags="$(LDFLAGS_RELEASE)" -o $(BUILD_DIR)/mycel ./cmd/mycel

release-local-bc: release-local-mycel ## Deprecated alias for release-local-mycel

# =============================================================================
# Run (dev, foreground)
# =============================================================================

run-mycel: ## Run mycel CLI from source
	$(GO) run ./cmd/mycel

run-bc: run-mycel ## Deprecated alias for run-mycel

run-web: ## Run web UI dev server
	cd web && bun run dev

run-landing: ## Run landing dev server
	cd landing && bun run dev

run-tui: build-local-tui ## Run TUI dev mode
	cd tui && bun run dev

build-landing-prod: ## Production build for landing page (Cloudflare Pages)
	cd landing && bun install && bun run build

# =============================================================================
# Install
# =============================================================================

install-local-mycel: build-local-mycel ## Install mycel to $GOPATH/bin
	cp $(BUILD_DIR)/mycel $(shell $(GO) env GOPATH)/bin/

install-local-bc: install-local-mycel ## Deprecated alias for install-local-mycel

# =============================================================================
# Dependencies
# =============================================================================

deps-go: ## Go dependencies
	$(GO) mod download && $(GO) mod tidy

deps-ts: ## TS dependencies
	cd tui && bun install
	cd web && bun install
	cd landing && bun install

# =============================================================================
# Security
# =============================================================================

scan-go: ## Go vulnerability scan
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

scan-ts: ## TS dependency audit
	cd tui && bun audit || true
	cd web && bun audit || true
	cd landing && bun audit || true

# =============================================================================
# Clean
# =============================================================================

clean-local: ## Remove build artifacts
	rm -rf $(BUILD_DIR)/ dist/ coverage.out coverage.html
	rm -rf tui/dist tui/dist-bundle web/dist server/web/dist landing/.next landing/out
	rm -rf internal/cmd/tui-bundle/index.js

clean-deps: clean-local ## Remove artifacts + node_modules
	rm -rf tui/node_modules web/node_modules landing/node_modules
