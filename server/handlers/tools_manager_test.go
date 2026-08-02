package handlers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInferToolManager(t *testing.T) {
	tests := []struct {
		name       string
		realPath   string
		installCmd string
		want       string
	}{
		// Path evidence: what actually installed the binary on PATH.
		{"brew cellar target", "/opt/homebrew/Cellar/ripgrep/14.1.1/bin/rg", "", "brew"},
		{"brew apple silicon prefix", "/opt/homebrew/bin/jq", "", "brew"},
		{"brew on linux", "/home/linuxbrew/.linuxbrew/bin/gh", "", "brew"},
		{"npm global", "/usr/local/lib/node_modules/npm/bin/npx", "", "npm"},
		{"cargo install", "/Users/x/.cargo/bin/rg", "", "cargo"},
		{"pipx venv", "/Users/x/.local/pipx/venvs/black/bin/black", "", "pipx"},
		{"os provided", "/usr/bin/git", "", "system"},
		{"os provided sbin", "/sbin/ping", "", "system"},

		// /usr/local/bin proves nothing on its own: Intel Homebrew installs
		// there, and so does `make install`. Must not be called "system".
		{"ambiguous usr local", "/usr/local/bin/mytool", "", ""},
		{"ambiguous usr local falls back to install cmd", "/usr/local/bin/rg", "brew install ripgrep", "brew"},

		// Install-command evidence, for a tool that is not installed yet.
		{"npm install cmd", "", "npm install -g @openai/codex", "npm"},
		{"brew install cmd", "", "brew install ripgrep", "brew"},
		{"sudo wrapper skipped", "", "sudo apt-get install -y jq", "apt"},
		{"env assignment skipped", "", "DEBIAN_FRONTEND=noninteractive apt-get install -y jq", "apt"},
		{"manager by absolute path", "", "/opt/homebrew/bin/brew install fd", "brew"},
		{"pip3 normalises to pip", "", "pip3 install httpie", "pip"},

		// No evidence: a bespoke installer must read as unknown, not be
		// guessed at from a package name that happens to match a manager.
		{"curl installer", "", "curl -fsSL https://example.com/i.sh | sh", ""},
		{"package named like a manager", "", "curl -o /tmp/npm https://example.com/npm", ""},
		{"nothing at all", "", "", ""},

		// Path wins over config: config can be stale, the binary cannot.
		{"path beats install cmd", "/opt/homebrew/bin/rg", "npm install -g ripgrep", "brew"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferToolManager(tc.realPath, tc.installCmd); got != tc.want {
				t.Errorf("InferToolManager(%q, %q) = %q, want %q", tc.realPath, tc.installCmd, got, tc.want)
			}
		})
	}
}

// The markers are matched case-insensitively so they still hold on a
// case-insensitive macOS volume, where the Cellar can appear as "cellar".
func TestManagerFromPathIgnoresCase(t *testing.T) {
	for _, p := range []string{
		"/opt/homebrew/cellar/ripgrep/14.1.1/bin/rg",
		"/opt/Homebrew/Cellar/ripgrep/14.1.1/bin/rg",
	} {
		if got := managerFromPath(p); got != "brew" {
			t.Errorf("managerFromPath(%q) = %q, want brew", p, got)
		}
	}
}

// describeBinary must report an absolute path and follow a symlink to identify
// the owner, which is the whole reason a brew tool is attributable at all.
func TestDescribeBinaryResolvesSymlinkToFindManager(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink layout is POSIX-specific")
	}
	// Lay out a fake Homebrew: a bin/ directory on PATH holding a symlink
	// into a Cellar, exactly as brew links its formulae.
	root := t.TempDir()
	cellar := filepath.Join(root, "Cellar", "faketool", "1.2.3", "bin")
	if err := os.MkdirAll(cellar, 0o750); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cellar, "faketool")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "faketool")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)

	path, manager := describeBinary("faketool", "")
	if path != link {
		t.Errorf("path = %q, want the symlink on PATH %q", path, link)
	}
	if manager != "brew" {
		t.Errorf("manager = %q, want brew (via the Cellar symlink target)", manager)
	}
}

// A tool that is not installed has no path, and its manager can only come from
// the configured install command.
func TestDescribeBinaryMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	path, manager := describeBinary("definitely-not-a-real-binary", "brew install whatever")
	if path != "" {
		t.Errorf("path = %q, want empty for a binary not on PATH", path)
	}
	if manager != "brew" {
		t.Errorf("manager = %q, want brew from the install command", manager)
	}
}
