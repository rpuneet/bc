// Package envpath makes GUI-launched mycel processes find Homebrew (and
// other well-known) binaries the same way an interactive shell would.
//
// macOS .app bundles and some launchd/desktop contexts inherit a minimal
// PATH that omits /opt/homebrew/bin. Without enrichment, LookPath("tmux")
// fails even when tmux is installed, and the UI reports "no runtime
// backend" — a false dependency failure.
package envpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var enrichOnce sync.Once

// ExtraBinDirs returns platform directories that commonly hold user-installed
// CLIs (Homebrew, /usr/local, ~/.local). Only existing directories are
// returned so PATH stays free of dead entries.
func ExtraBinDirs() []string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/opt/homebrew/bin", // Apple Silicon Homebrew
			"/opt/homebrew/sbin",
			"/usr/local/bin", // Intel Homebrew / manual installs
			"/usr/local/sbin",
		}
	case "linux":
		candidates = []string{
			"/home/linuxbrew/.linuxbrew/bin",
			"/usr/local/bin",
			"/usr/local/sbin",
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, "go", "bin"),
		)
	}

	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// Merge prepends missing ExtraBinDirs entries onto path (PATH-shaped).
// Existing entries keep their relative order; extras that are already
// present are not duplicated. Pure — does not touch the process env.
func Merge(path string) string {
	extras := ExtraBinDirs()
	if len(extras) == 0 {
		return path
	}
	parts := filepath.SplitList(path)
	have := make(map[string]struct{}, len(parts)+len(extras))
	for _, p := range parts {
		if p == "" {
			continue
		}
		have[p] = struct{}{}
	}
	prefix := make([]string, 0, len(extras))
	for _, d := range extras {
		if _, ok := have[d]; ok {
			continue
		}
		prefix = append(prefix, d)
		have[d] = struct{}{}
	}
	if len(prefix) == 0 {
		return path
	}
	if path == "" {
		return strings.Join(prefix, string(os.PathListSeparator))
	}
	return strings.Join(prefix, string(os.PathListSeparator)) + string(os.PathListSeparator) + path
}

// Enrich once prepends well-known bin directories onto the process PATH.
// Safe to call from multiple entrypoints (CLI, desktop); subsequent
// calls are no-ops.
func Enrich() {
	enrichOnce.Do(func() {
		merged := Merge(os.Getenv("PATH"))
		if merged != os.Getenv("PATH") {
			_ = os.Setenv("PATH", merged)
		}
	})
}
