//go:build !windows

package cron

import (
	"os/exec"
	"syscall"
)

// isolateProcessGroup configures cmd to run in a new process group.
//
// On Unix, signals sent to a process group (e.g. via `kill 0`, `kill -- -$pgid`,
// or terminal-driven SIGTERM/SIGINT) are delivered to every process that shares
// the group. By default a child started with exec.Command inherits its parent's
// process group, so a cron command that signals its own group will also signal
// bcd. Setting Setpgid=true with Pgid=0 makes the child the leader of a fresh
// process group, isolating it from bcd.
func isolateProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pgid = 0
}
