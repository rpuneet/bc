package deps

import (
	"context"
	"fmt"
	"strings"
)

// bcDBContainer is the Docker container name used for the bc-db TimescaleDB
// service. It matches the container lazy-started by `mycel up` for timescale storage
// (see internal/cmd/up.go) so start/stop interact with the
// already-running instance if any.
const bcDBContainer = "bc-db"

// bcDBImage is the image tag built by `make build-docker-db`.
const bcDBImage = "bc-bcdb:latest"

// BCDB wraps the bc-db (unified TimescaleDB) Docker container. It is a
// singleton per host — one container shared by all workspaces — matching
// the design in docs/proposals/multi-workspace-and-code-tab.md §7.2.1.
type BCDB struct {
	runner execRunner
}

// NewBCDB constructs a BCDB dependency using the default exec runner.
func NewBCDB() *BCDB {
	return &BCDB{runner: defaultExec}
}

// NewBCDBWithRunner is the constructor used in tests to inject a mock
// exec runner.
func NewBCDBWithRunner(r execRunner) *BCDB {
	if r == nil {
		r = defaultExec
	}
	return &BCDB{runner: r}
}

// ID implements Dependency.
func (*BCDB) ID() string { return "bc-db" }

// DisplayName implements Dependency.
func (*BCDB) DisplayName() string { return "bc-db" }

// Description implements Dependency.
func (*BCDB) Description() string {
	return "Unified TimescaleDB for metrics, costs, and events"
}

// Deprecated implements Dependency.
func (*BCDB) Deprecated() bool { return false }

// Status inspects the container and reports running/stopped/unknown.
func (d *BCDB) Status(ctx context.Context) (State, error) {
	out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", bcDBContainer)
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

// Start ensures the bc-db container is running. If a container with the
// expected name already exists (running or stopped), it is reused via
// `docker start`; otherwise a fresh `docker run` is issued.
func (d *BCDB) Start(ctx context.Context) error {
	// Probe whether the container already exists.
	if out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", bcDBContainer); err == nil {
		if strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		// Exists but stopped — start it.
		if out2, sErr := d.runner.Run(ctx, "docker", "start", bcDBContainer); sErr != nil {
			return fmt.Errorf("docker start: %w (%s)", sErr, strings.TrimSpace(string(out2)))
		}
		return nil
	}
	// Does not exist — create and run.
	args := []string{
		"run", "-d",
		"--name", bcDBContainer,
		"-p", "5432:5432",
		"-e", "POSTGRES_PASSWORD=bc",
		"-v", "bc-db-data:/var/lib/postgresql/data",
		"--restart", "always",
		bcDBImage,
	}
	if out, err := d.runner.Run(ctx, "docker", args...); err != nil {
		return fmt.Errorf("docker run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stop stops the bc-db container but does not remove it so data is retained
// on disk via the named volume.
func (d *BCDB) Stop(ctx context.Context) error {
	if out, err := d.runner.Run(ctx, "docker", "stop", bcDBContainer); err != nil {
		text := strings.ToLower(string(out))
		if strings.Contains(text, "no such") {
			return nil
		}
		return fmt.Errorf("docker stop: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Logs returns the last `tail` lines from `docker logs`.
func (d *BCDB) Logs(ctx context.Context, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, err := d.runner.Run(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", tail), bcDBContainer)
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
