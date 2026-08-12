//go:build unix

package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// probeDockerCmd builds a `docker <args…>` command that cannot outlive
// parent+dockerProbeTimeout. CommandContext alone is not enough for a
// wedged Docker Desktop: the CLI can ignore SIGTERM (or leave child
// sleepers under a shell wrapper), and Wait then blocks on pipes forever.
// We put the probe in its own process group, SIGKILL the group when the
// deadline hits, and set WaitDelay so Wait cannot outlive the kill by
// more than a beat.
func probeDockerCmd(parent context.Context, args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, dockerProbeTimeout)
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // trusted binary + fixed args
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid = process group (Setpgid above).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd, ctx, cancel
}

// probeDocker runs a short `docker <args…>` discarding output.
func probeDocker(parent context.Context, args ...string) error {
	cmd, ctx, cancel := probeDockerCmd(parent, args...)
	defer cancel()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("docker daemon not available: probe timed out after %s: %w", dockerProbeTimeout, err)
		}
		return fmt.Errorf("docker daemon not available: %w", err)
	}
	return nil
}

// probeDockerOutput is probeDocker that returns stdout.
func probeDockerOutput(parent context.Context, args ...string) ([]byte, error) {
	cmd, _, cancel := probeDockerCmd(parent, args...)
	defer cancel()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}
