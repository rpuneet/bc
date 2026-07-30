package deps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// bcCodeServerContainer is the docker container name.
const bcCodeServerContainer = "bc-code-server"

// bcCodeServerImage is the upstream image used to run VS Code in the browser.
const bcCodeServerImage = "codercom/code-server:latest"

// bcCodeServerPort is the host port mapping; the container listens on 8080
// internally (code-server default).
const bcCodeServerPort = "8100"

// BCCodeServer wraps a code-server container that mounts the currently
// active repo root at /home/coder/workspace.
//
// The repoRoot is mutable because the active repo can change while
// bcd is running — callers update it via SetRepoRoot.
type BCCodeServer struct {
	runner   execRunner
	repoRoot string
	mu       sync.RWMutex
}

// NewBCCodeServer constructs the dependency bound to repoRoot.
func NewBCCodeServer(repoRoot string) *BCCodeServer {
	return &BCCodeServer{runner: defaultExec, repoRoot: repoRoot}
}

// NewBCCodeServerWithRunner is used by tests.
func NewBCCodeServerWithRunner(repoRoot string, r execRunner) *BCCodeServer {
	if r == nil {
		r = defaultExec
	}
	return &BCCodeServer{runner: r, repoRoot: repoRoot}
}

// SetRepoRoot updates the directory that will be bind-mounted on the
// next Start. It does NOT restart an already-running container.
func (d *BCCodeServer) SetRepoRoot(path string) {
	d.mu.Lock()
	d.repoRoot = path
	d.mu.Unlock()
}

// RepoRoot returns the currently configured repo root.
func (d *BCCodeServer) RepoRoot() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.repoRoot
}

// ID implements Dependency.
func (*BCCodeServer) ID() string { return "bc-code-server" }

// DisplayName implements Dependency.
func (*BCCodeServer) DisplayName() string { return "bc-code-server" }

// Description implements Dependency.
func (*BCCodeServer) Description() string {
	return "VS Code in the browser, bind-mounted to the active repo"
}

// Deprecated implements Dependency.
func (*BCCodeServer) Deprecated() bool { return false }

// Status reports the container state. Unknown if docker is unreachable.
func (d *BCCodeServer) Status(ctx context.Context) (State, error) {
	out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", bcCodeServerContainer)
	if err != nil {
		text := strings.ToLower(string(out))
		if strings.Contains(text, "no such") {
			return StateStopped, nil
		}
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

// Start launches (or re-creates) the code-server container.
//
// Since the bind-mounted directory depends on the active repo, any
// existing container is removed so the new mount takes effect.
func (d *BCCodeServer) Start(ctx context.Context) error {
	root := d.RepoRoot()
	if root == "" {
		return errors.New("bc-code-server: repo root not configured")
	}

	// Remove any prior container so a stale mount doesn't linger.
	_, _ = d.runner.Run(ctx, "docker", "rm", "-f", bcCodeServerContainer)

	args := []string{
		"run", "-d",
		"--name", bcCodeServerContainer,
		"-p", bcCodeServerPort + ":8080",
		"-v", fmt.Sprintf("%s:/home/coder/workspace", root),
		bcCodeServerImage,
		"--auth=none",
	}
	if out, err := d.runner.Run(ctx, "docker", args...); err != nil {
		return fmt.Errorf("docker run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stop stops and removes the container so Start can pick up a new mount.
func (d *BCCodeServer) Stop(ctx context.Context) error {
	if out, err := d.runner.Run(ctx, "docker", "stop", bcCodeServerContainer); err != nil {
		text := strings.ToLower(string(out))
		if !strings.Contains(text, "no such") {
			return fmt.Errorf("docker stop: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	// Remove so a later Start can remount if the repo changed.
	_, _ = d.runner.Run(ctx, "docker", "rm", bcCodeServerContainer)
	return nil
}

// Logs returns the last `tail` lines from `docker logs`.
func (d *BCCodeServer) Logs(ctx context.Context, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, err := d.runner.Run(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", tail), bcCodeServerContainer)
	if err != nil {
		return nil, fmt.Errorf("docker logs: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return splitLines(string(out)), nil
}
