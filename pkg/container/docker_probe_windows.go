//go:build windows

package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Windows lacks process-group SIGKILL; bound probes with Context + WaitDelay.
func probeDockerCmd(parent context.Context, args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, dockerProbeTimeout)
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // trusted binary + fixed args
	cmd.WaitDelay = time.Second
	return cmd, ctx, cancel
}

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
