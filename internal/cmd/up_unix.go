//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// detachSession starts the command in a new session so SIGHUP from terminal
// close doesn't propagate. Unix-only.
func detachSession(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
