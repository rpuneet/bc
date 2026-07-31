package deps

import (
	"context"
	"os/exec"
)

// execRunner is the narrow exec.Cmd surface used by the dependency
// implementations. Tests can inject a mock to assert which commands would
// run without actually spawning processes.
type execRunner interface {
	// Run executes cmd with args and returns combined stdout+stderr output.
	Run(ctx context.Context, cmd string, args ...string) ([]byte, error)
}

// realExec shells out via os/exec.
type realExec struct{}

func (realExec) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	//nolint:gosec // callers pass trusted argv (docker + flags)
	return exec.CommandContext(ctx, cmd, args...).CombinedOutput()
}

// defaultExec is the runner used by NewDB and NewCodeServer. Tests swap
// it in via the exported constructors that accept an explicit runner.
var defaultExec execRunner = realExec{}
