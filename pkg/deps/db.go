package deps

import (
	"context"
	"fmt"
	"strings"
)

// dbContainer is the Docker container name used for the mycel-db TimescaleDB
// service. It matches the container lazy-started by `mycel up` for timescale storage
// (see internal/cmd/up.go) so start/stop interact with the
// already-running instance if any.
const dbContainer = "mycel-db"

// dbImage is the image tag built by `make build-docker-db`.
const dbImage = "mycel-db:latest"

// DB wraps the mycel-db (unified TimescaleDB) Docker container. It is a
// singleton per host — one container shared by all workspaces — matching
// the design in docs/proposals/multi-workspace-and-code-tab.md §7.2.1.
type DB struct {
	runner execRunner
}

// NewDB constructs a DB dependency using the default exec runner.
func NewDB() *DB {
	return &DB{runner: defaultExec}
}

// NewDBWithRunner is the constructor used in tests to inject a mock
// exec runner.
func NewDBWithRunner(r execRunner) *DB {
	if r == nil {
		r = defaultExec
	}
	return &DB{runner: r}
}

// ID implements Dependency.
func (*DB) ID() string { return "mycel-db" }

// DisplayName implements Dependency.
func (*DB) DisplayName() string { return "mycel-db" }

// Description implements Dependency.
func (*DB) Description() string {
	return "Unified TimescaleDB for metrics, costs, and events"
}

// Deprecated implements Dependency.
func (*DB) Deprecated() bool { return false }

// Status inspects the container and reports running/stopped/unknown.
func (d *DB) Status(ctx context.Context) (State, error) {
	out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", dbContainer)
	if err != nil {
		// Most common case: container does not exist yet.
		text := strings.ToLower(string(out))
		if strings.Contains(text, "no such") || strings.Contains(text, "error: no such") {
			return StateStopped, nil
		}
		// Docker daemon unreachable or similar. Report unknown rather
		// than bubbling the error so the UI can still render.
		return StateUnknown, nil
	}
	switch strings.TrimSpace(string(out)) {
	case "true":
		return StateRunning, nil
	case "false":
		return StateStopped, nil
	default:
		return StateUnknown, nil
	}
}

// Start ensures the mycel-db container is running. If a container with the
// expected name already exists (running or stopped), it is reused via
// `docker start`; otherwise a fresh `docker run` is issued.
func (d *DB) Start(ctx context.Context) error {
	// Probe whether the container already exists.
	if out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", dbContainer); err == nil {
		if strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		// Exists but stopped — start it.
		if out2, sErr := d.runner.Run(ctx, "docker", "start", dbContainer); sErr != nil {
			return fmt.Errorf("docker start: %w (%s)", sErr, strings.TrimSpace(string(out2)))
		}
		return nil
	}
	// Does not exist — create and run.
	args := []string{
		"run", "-d",
		"--name", dbContainer,
		"-p", "5432:5432",
		"-e", "POSTGRES_PASSWORD=mycel",
		"-v", "mycel-db-data:/var/lib/postgresql/data",
		"--restart", "always",
		dbImage,
	}
	if out, err := d.runner.Run(ctx, "docker", args...); err != nil {
		return fmt.Errorf("docker run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stop stops the mycel-db container but does not remove it so data is retained
// on disk via the named volume.
func (d *DB) Stop(ctx context.Context) error {
	if out, err := d.runner.Run(ctx, "docker", "stop", dbContainer); err != nil {
		text := strings.ToLower(string(out))
		if strings.Contains(text, "no such") {
			return nil
		}
		return fmt.Errorf("docker stop: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Logs returns the last `tail` lines from `docker logs`.
func (d *DB) Logs(ctx context.Context, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, err := d.runner.Run(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", tail), dbContainer)
	if err != nil {
		return nil, fmt.Errorf("docker logs: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return splitLines(string(out)), nil
}

// splitLines turns multi-line output into a []string with empty trailing
// entry trimmed. Handy for Logs implementations.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines
}
