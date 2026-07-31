package deps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// codeServerContainer is the docker container name.
const codeServerContainer = "mycel-code-server"

// codeServerImage is the upstream image used to run VS Code in the browser.
const codeServerImage = "codercom/code-server:latest"

// codeServerPort is the host port mapping; the container listens on 8080
// internally (code-server default).
const codeServerPort = "8100"

// CodeServer wraps a code-server container that mounts the currently
// active repo root at /home/coder/workspace.
//
// The repoRoot is mutable because the active repo can change while
// the daemon is running — callers update it via SetRepoRoot.
type CodeServer struct {
	runner   execRunner
	repoRoot string
	mu       sync.RWMutex
}

// NewCodeServer constructs the dependency bound to repoRoot.
func NewCodeServer(repoRoot string) *CodeServer {
	return &CodeServer{runner: defaultExec, repoRoot: repoRoot}
}

// NewCodeServerWithRunner is used by tests.
func NewCodeServerWithRunner(repoRoot string, r execRunner) *CodeServer {
	if r == nil {
		r = defaultExec
	}
	return &CodeServer{runner: r, repoRoot: repoRoot}
}

// SetRepoRoot updates the directory that will be bind-mounted on the
// next Start. It does NOT restart an already-running container.
func (d *CodeServer) SetRepoRoot(path string) {
	d.mu.Lock()
	d.repoRoot = path
	d.mu.Unlock()
}

// RepoRoot returns the currently configured repo root.
func (d *CodeServer) RepoRoot() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.repoRoot
}

// ID implements Dependency.
func (*CodeServer) ID() string { return "mycel-code-server" }

// DisplayName implements Dependency.
func (*CodeServer) DisplayName() string { return "mycel-code-server" }

// Description implements Dependency.
func (*CodeServer) Description() string {
	return "VS Code in the browser, bind-mounted to the active repo"
}

// Deprecated implements Dependency.
func (*CodeServer) Deprecated() bool { return false }

// Status reports the container state. Unknown if docker is unreachable.
func (d *CodeServer) Status(ctx context.Context) (State, error) {
	out, err := d.runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", codeServerContainer)
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
func (d *CodeServer) Start(ctx context.Context) error {
	root := d.RepoRoot()
	if root == "" {
		return errors.New("mycel-code-server: repo root not configured")
	}

	// Remove any prior container so a stale mount doesn't linger.
	_, _ = d.runner.Run(ctx, "docker", "rm", "-f", codeServerContainer)

	args := []string{
		"run", "-d",
		"--name", codeServerContainer,
		"-p", codeServerPort + ":8080",
		"-v", fmt.Sprintf("%s:/home/coder/workspace", root),
		codeServerImage,
		"--auth=none",
	}
	if out, err := d.runner.Run(ctx, "docker", args...); err != nil {
		return fmt.Errorf("docker run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stop stops and removes the container so Start can pick up a new mount.
func (d *CodeServer) Stop(ctx context.Context) error {
	if out, err := d.runner.Run(ctx, "docker", "stop", codeServerContainer); err != nil {
		text := strings.ToLower(string(out))
		if !strings.Contains(text, "no such") {
			return fmt.Errorf("docker stop: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	// Remove so a later Start can remount if the repo changed.
	_, _ = d.runner.Run(ctx, "docker", "rm", codeServerContainer)
	return nil
}

// Logs returns the last `tail` lines from `docker logs`.
func (d *CodeServer) Logs(ctx context.Context, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, err := d.runner.Run(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", tail), codeServerContainer)
	if err != nil {
		return nil, fmt.Errorf("docker logs: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return splitLines(string(out)), nil
}
