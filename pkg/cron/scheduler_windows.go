//go:build windows

package cron

import "os/exec"

// isolateProcessGroup is a no-op on Windows: signal semantics differ and bcd
// is not supported on Windows in production. The function exists so the
// platform-neutral scheduler code can call it unconditionally.
func isolateProcessGroup(_ *exec.Cmd) {}
