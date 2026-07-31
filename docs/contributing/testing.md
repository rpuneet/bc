# Testing Guide

This guide covers how to run tests, write new tests, and maintain coverage
in the bc codebase.

## Running Tests

### Quick Reference

| Command                | What it runs                                     |
|------------------------|--------------------------------------------------|
| `make test`            | All tests (Go + TypeScript)                      |
| `make test-go`         | Go tests with race detector                      |
| `make test-go-fast`    | Go tests excluding slow/E2E packages             |
| `make test-ts`         | All TypeScript tests (web + landing)             |
| `make test-web`        | Web dashboard tests (vitest)                     |
| `make test-web-e2e`    | Web E2E tests (Playwright, requires a running daemon) |
| `make test-landing`    | Landing page tests (Playwright)                  |
| `make coverage-go`     | Go coverage report with threshold check          |
| `make bench-go`        | Go benchmarks                                    |
| `make ci-local`        | Full CI pipeline locally                         |

### Running a Specific Go Test

```bash
go test -race -run TestAgentStart ./pkg/agent/
```

The `-race` flag enables the race detector and should always be used during
development.

### Running a Specific Web Test

```bash
cd web && bun run test -- --reporter=verbose src/components/AgentTable.test.tsx
```

### Full CI Pipeline Locally

```bash
make ci-local
```

This runs the complete quality gate: formatting, vetting, linting, and
testing across both Go and TypeScript. It matches what CI runs on pull
requests.

## Coverage

### Go Coverage

```bash
make coverage-go
```

This generates a `coverage.out` file and checks that total coverage meets
the **75% threshold** (configured via `COVERAGE_THRESHOLD` in the Makefile).
The build fails if coverage drops below this threshold.

To view a detailed HTML coverage report:

```bash
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

### TypeScript Coverage

```bash
make coverage-ts
```

Web coverage requires `@vitest/coverage-v8`.

## Where Tests Live

### Go Tests

Go tests live alongside the code they test, following Go convention:

| Location                         | What it tests                          |
|----------------------------------|----------------------------------------|
| `pkg/agent/*_test.go`            | Agent lifecycle, hooks, state machine  |
| `pkg/home/*_test.go`             | Home bootstrap, config parsing         |
| `pkg/channel/*_test.go`          | Channel CRUD, message delivery         |
| `pkg/secret/*_test.go`           | Encryption, secret store operations    |
| `pkg/cost/*_test.go`             | Source-direct cost computation         |
| `pkg/container/*_test.go`        | Docker backend, mount validation       |
| `pkg/tmux/*_test.go`             | Tmux backend                           |
| `internal/cmd/*_test.go`         | CLI command integration tests          |
| `server/handlers/*_test.go`      | HTTP handler unit tests                |
| `server/mcp/*_test.go`           | MCP protocol tests                     |
| `server/e2e_test.go`             | Server end-to-end tests                |
| `server/e2e_web_test.go`         | Web UI SSE/API e2e tests               |

### TypeScript Tests

| Location                         | What it tests                          |
|----------------------------------|----------------------------------------|
| `web/src/**/*.test.ts(x)`        | Web dashboard components (vitest)      |
| `web/e2e/`                       | Web E2E tests (Playwright)             |
| `landing/e2e/`                   | Landing page E2E tests (Playwright)    |

## Writing Go Tests

### Table-Driven Tests

Use table-driven tests as the default pattern:

```go
func TestValidateMount(t *testing.T) {
    tests := []struct {
        name      string
        mount     string
        root      string
        wantErr   bool
    }{
        {
            name:  "valid mount within repo",
            mount: "/repo/data:/data",
            root:  "/repo",
        },
        {
            name:    "path traversal rejected",
            mount:   "/repo/../etc/passwd:/etc/passwd",
            root:    "/repo",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateMount(tt.mount, tt.root)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateMount() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### TestMain Setup

Some packages require global state initialization. `internal/cmd/` and
`pkg/agent/` use `TestMain()` to set up `RoleCapabilities` and
`RoleHierarchy` maps:

```go
func TestMain(m *testing.M) {
    setupRoles()
    os.Exit(m.Run())
}
```

### HTTP Handler Tests

Use `httptest.NewServer` for end-to-end handler testing:

```go
func TestAgentHandler(t *testing.T) {
    svc := setupTestServices(t)
    handler := handlers.NewAgentHandler(svc.Agents, svc.Costs, svc.WS)

    mux := http.NewServeMux()
    handler.Register(mux)
    srv := httptest.NewServer(mux)
    defer srv.Close()

    resp, err := http.Get(srv.URL + "/api/agents")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("got status %d, want 200", resp.StatusCode)
    }
}
```

### Integration Tests

Integration tests that need a real home + repo use helper functions:

- `setupIntegrationHome()` — points `MYCEL_HOME` at a temp dir and
  bootstraps a git repo to anchor against.
- `seedAgents()` — populates the store with test agent state.

### E2E Tests

E2E tests that require live tmux sessions are in `pkg/agent/agent_e2e_test.go`
and `pkg/channel/channel_e2e_test.go`. These are included in `make test-go`
but excluded from `make test-go-fast` (which skips `internal/cmd`).

## Writing Web Dashboard Tests

### Vitest Unit Tests

Web component tests use vitest with React Testing Library:

```typescript
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { AgentTable } from "./AgentTable";

describe("AgentTable", () => {
  it("renders empty state", () => {
    render(<AgentTable agents={[]} />);
    expect(screen.getByText("No agents")).toBeInTheDocument();
  });
});
```

### Playwright E2E Tests

Web E2E tests live in `web/e2e/` and require a running daemon:

```bash
# Start the daemon first
make run-mycel

# Run e2e tests
make test-web-e2e
```

## Best Practices

1. **Always use `-race`** when running Go tests locally.
2. **Table-driven tests** for any function with more than two test cases.
3. **No magic numbers in tests** — use named constants or descriptive
   variables (the `mnd` linter is disabled for test files).
4. **Clean up temp files** — use `t.TempDir()` which auto-cleans.
5. **Context propagation** — pass `context.Background()` or
   `context.TODO()` in tests, never `nil`.
6. **Run `make check` before committing** — this catches lint errors,
   formatting issues, and test failures in one command.
