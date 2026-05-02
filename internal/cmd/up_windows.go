//go:build windows

package cmd

import "os/exec"

// detachSession is a no-op on Windows (no Setsid equivalent).
func detachSession(cmd *exec.Cmd) {}
