//go:build !windows

package cron

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDefaultExec_ChildHasOwnProcessGroup verifies that a cron command runs in
// its own process group, so it cannot signal bcd by signaling its own group.
//
// Regression test for #2964 (cron build-and-deploy self-kills bcd).
func TestDefaultExec_ChildHasOwnProcessGroup(t *testing.T) {
	s := NewScheduler(nil, t.TempDir())
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The child prints its own pgid. ps -o pgid= -p $$ trims the value via tr.
	if err := s.defaultExec(ctx, "ps -o pgid= -p $$ | tr -d ' '", &out); err != nil {
		t.Fatalf("defaultExec: %v", err)
	}

	childPgid, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil {
		t.Fatalf("parse child pgid from %q: %v", out.String(), err)
	}

	parentPgid, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatalf("Getpgid(parent): %v", err)
	}

	if childPgid == parentPgid {
		t.Fatalf("child cron command shares bcd's process group (pgid=%d); "+
			"a `kill 0` in the cron command would terminate bcd. See #2964.",
			childPgid)
	}
}

// TestDefaultExec_SignalToChildGroupDoesNotReachParent simulates a cron job
// that broadcasts SIGTERM to its own process group, and asserts the bcd
// (test) process is not signaled.
//
// We register a SIGTERM handler before running, run a command that signals
// its own group, then verify no SIGTERM was delivered to us.
func TestDefaultExec_SignalToChildGroupDoesNotReachParent(t *testing.T) {
	gotSig := make(chan os.Signal, 1)
	// Install a handler that absorbs SIGTERM so the test process isn't killed
	// even if the isolation regresses — we want to fail the test, not crash.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case s := <-sigCh:
			gotSig <- s
		case <-time.After(2 * time.Second):
		}
	}()

	s := NewScheduler(nil, t.TempDir())
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// `kill -TERM 0` sends SIGTERM to every process in the caller's group.
	// If isolation works, only the child shell receives it; the test process
	// (the simulated bcd parent) does not. The child shell exits with signal
	// status when it receives SIGTERM, so defaultExec returns a non-nil
	// *exec.ExitError — that is the expected, working-as-intended outcome
	// here. We only need to assert the kill scenario actually ran (no
	// "command not found" or syntax error from the host shell).
	if err := s.defaultExec(ctx, "kill -TERM 0", &out); err != nil {
		// An exit error from the killed shell is expected. A different
		// error type would mean the kill never executed.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("defaultExec failed before kill could run: %v (output=%q)", err, out.String())
		}
	}

	select {
	case <-gotSig:
		t.Fatalf("parent received SIGTERM from child cron command — process group isolation is broken (see #2964)")
	case <-time.After(500 * time.Millisecond):
		// Good: parent was not signaled.
	}
}
