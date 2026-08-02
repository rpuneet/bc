package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCLIVersion covers the Tools table showing mangled version banners: the
// old behavior truncated raw `--version` output at 80 characters, so `curl`
// rendered as "curl 8.7.1 (x86_64-apple-darwin25.0) libcurl/8.7.1
// (SecureTransport) LibreSSL/3." — a sentence cut mid-token. Banners below are
// real output captured from a developer machine.
func TestCLIVersion(t *testing.T) {
	for _, tc := range []struct {
		name, banner, want string
	}{
		{"aws", "aws-cli/2.36.14 Python/3.14.6 Darwin/25.5.0 source/arm64\n", "2.36.14"},
		{"curl", "curl 8.7.1 (x86_64-apple-darwin25.0) libcurl/8.7.1 (SecureTransport) LibreSSL/3.3.6\n", "8.7.1"},
		{"bun already bare", "1.3.6\n", "1.3.6"},
		{"git", "git version 2.39.5\n", "2.39.5"},
		{"go", "go version go1.24.2 darwin/arm64\n", "1.24.2"},
		{"jq", "jq-1.7.1\n", "1.7.1"},
		{"python", "Python 3.13.1\n", "3.13.1"},
		{"docker with trailing build", "Docker version 27.3.1, build ce12230\n", "27.3.1"},
		{"node", "v22.14.0\n", "22.14.0"},
		// Two-part versions have no semver token; keep the version, not the banner.
		{"tmux", "tmux 3.5a\n", "3.5a"},
		// The full macOS banner: a later line carries "darwin11.3.0", which must
		// not outrank the two-part version make actually leads with.
		{
			"gnu make full banner",
			"GNU Make 3.81\n" +
				"Copyright (C) 2006  Free Software Foundation, Inc.\n" +
				"This is free software; see the source for copying conditions.\n" +
				"There is NO warranty; not even for MERCHANTABILITY or FITNESS FOR A\n" +
				"PARTICULAR PURPOSE.\n\n" +
				"This program built for i386-apple-darwin11.3.0\n",
			"3.81",
		},
		// A warning ahead of the version must not become the answer.
		{"warning precedes version", "warning: config is deprecated\nmytool 1.4.2\n", "1.4.2"},
		{"no version at all falls back to first line", "unknown build\n", "unknown build"},
		{"blank", "\n\n", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cliVersion(tc.banner); got != tc.want {
				t.Errorf("cliVersion(%q) = %q, want %q", tc.banner, got, tc.want)
			}
		})
	}
}

// TestCLIVersionNeverExceedsMaxLen: the fallback path still has to respect the
// response's version budget.
func TestCLIVersionNeverExceedsMaxLen(t *testing.T) {
	long := ""
	for i := 0; i < 40; i++ {
		long += "verbose-banner-"
	}
	if got := cliVersion(long); len(got) > maxVersionLen {
		t.Errorf("cliVersion returned %d chars, want <= %d", len(got), maxVersionLen)
	}
}

// writeStub creates an executable printing the given script body.
func writeStub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatalf("write stub: %v", err)
	}
	return path
}

// TestRunVersionReadsStderrAndNonZeroExit: CLIs that print their version to
// stderr, or print it and then fail a later check (`docker --version` with the
// daemon down), previously reported no version at all.
func TestRunVersionReadsStderrAndNonZeroExit(t *testing.T) {
	ctx := context.Background()

	stderrOnly := writeStub(t, "echo 'mytool 2.5.1' >&2\n")
	if got := runVersion(ctx, stderrOnly+" --version"); got != "2.5.1" {
		t.Errorf("stderr-only version = %q, want %q", got, "2.5.1")
	}

	printsThenFails := writeStub(t, "echo 'mytool 3.1.4'\nexit 1\n")
	if got := runVersion(ctx, printsThenFails+" --version"); got != "3.1.4" {
		t.Errorf("non-zero-exit version = %q, want %q", got, "3.1.4")
	}

	silentFailure := writeStub(t, "exit 1\n")
	if got := runVersion(ctx, silentFailure+" --version"); got != "" {
		t.Errorf("silent failure version = %q, want empty", got)
	}

	if got := runVersion(ctx, ""); got != "" {
		t.Errorf("empty command version = %q, want empty", got)
	}
}
