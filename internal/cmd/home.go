package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
)

// runHome is called from runRoot when a workspace is found.
// The bc home command has been removed; the TUI opens automatically.

func runHome(cmd *cobra.Command, args []string) error {
	log.Debug("home command started")

	// Find workspace
	ws, err := getRepo()
	if err != nil {
		return errNoRepo(err)
	}

	tuiEntry, tuiRoot, err := resolveTUIEntry(ws.RootDir)
	if err != nil {
		return err
	}

	// Find bun or node
	runtime, err := findJSRuntime()
	if err != nil {
		return err
	}
	log.Debug("using JS runtime", "runtime", runtime)

	// Run the TUI with signal handling
	log.Debug("starting TUI", "entry", tuiEntry)

	// Create context with signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// #nosec G204 - runtime is from exec.LookPath, safe to use
	tuiCmd := exec.CommandContext(ctx, runtime, "run", tuiEntry)
	tuiCmd.Dir = tuiRoot
	tuiCmd.Stdin = os.Stdin
	tuiCmd.Stdout = os.Stdout
	tuiCmd.Stderr = os.Stderr

	// Set environment for mycel CLI path
	// Get the current executable path so TUI can call bc
	bcBin, _ := os.Executable()
	tuiCmd.Env = append(os.Environ(),
		fmt.Sprintf("BC_ROOT=%s", ws.RootDir),
		fmt.Sprintf("BC_BIN=%s", bcBin),
	)

	return tuiCmd.Run()
}

// findJSRuntime finds bun or node executable.
func findJSRuntime() (string, error) {
	// Prefer bun
	if path, err := exec.LookPath("bun"); err == nil {
		return path, nil
	}

	// Fall back to node
	if path, err := exec.LookPath("node"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("bun or node not found. Install bun: https://bun.sh")
}

// findTUIRoot finds the directory containing the tui/ folder.
// In worktrees, ws.RootDir resolves through symlinked .bc to the parent repo,
// but we want to use the worktree's local tui/ if it exists.
// Priority: cwd > ws.RootDir
func findTUIRoot(wsRoot string) (string, error) {
	// First, check current working directory
	cwd, err := os.Getwd()
	if err == nil {
		tuiDir := filepath.Join(cwd, "tui")
		if _, statErr := os.Stat(tuiDir); statErr == nil {
			log.Debug("using TUI from cwd", "path", tuiDir)
			return cwd, nil
		}
	}

	// Fall back to workspace root
	tuiDir := filepath.Join(wsRoot, "tui")
	if _, statErr := os.Stat(tuiDir); statErr == nil {
		log.Debug("using TUI from workspace root", "path", tuiDir)
		return wsRoot, nil
	}

	return "", fmt.Errorf("TUI directory not found in cwd (%s) or repo root (%s)", cwd, wsRoot)
}

// resolveTUIEntry returns the path to the TUI entry point script and the
// directory to run it from. It prefers the embedded bundle (released binary),
// falling back to the dev checkout's tui/dist/index.js (or builds it if
// source is present).
//
// Returns (entryPath, runDir, error).
func resolveTUIEntry(wsRoot string) (string, string, error) {
	// Released binary: use the embedded bundle.
	if hasEmbeddedTUI() {
		return extractEmbeddedTUI()
	}

	// Dev checkout: find tui/ directory and use tsc-built dist/index.js.
	tuiRoot, err := findTUIRoot(wsRoot)
	if err != nil {
		return "", "", err
	}
	tuiDir := filepath.Join(tuiRoot, "tui")
	tuiEntry := filepath.Join(tuiDir, "dist", "index.js")

	if _, statErr := os.Stat(tuiEntry); os.IsNotExist(statErr) {
		log.Debug("TUI not built, checking for source")
		tuiSrc := filepath.Join(tuiDir, "src", "index.tsx")
		if _, srcErr := os.Stat(tuiSrc); os.IsNotExist(srcErr) {
			return "", "", fmt.Errorf("TUI not found. Run from the mycel repository root or install a released mycel binary")
		}

		fmt.Println("TUI not built. Building now...")
		buildCtx := context.Background()
		buildCmd := exec.CommandContext(buildCtx, "make", "build-local-tui")
		buildCmd.Dir = tuiRoot
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if buildErr := buildCmd.Run(); buildErr != nil {
			return "", "", fmt.Errorf("failed to build TUI: %w\nRun 'make build-local-tui' manually", buildErr)
		}
	}

	return tuiEntry, tuiRoot, nil
}

// extractEmbeddedTUI writes the embedded tuiBundleJS to a stable cache dir
// under $XDG_CACHE_HOME/bc/tui/ (or ~/.cache/bc/tui/) and returns its path.
// The cache dir is reused across invocations to avoid disk churn; a hash of
// the embedded content gates re-extraction so binary upgrades get fresh files.
func extractEmbeddedTUI() (string, string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	bundleDir := filepath.Join(cacheRoot, "bc", "tui", fmt.Sprintf("%x", tuiBundleHash()))
	entry := filepath.Join(bundleDir, "index.js")

	// Skip re-extraction if the file already exists and is the right size.
	if info, statErr := os.Stat(entry); statErr == nil && info.Size() == int64(len(tuiBundleJS)) {
		log.Debug("using cached embedded TUI bundle", "path", entry)
		return entry, bundleDir, nil
	}

	if mkErr := os.MkdirAll(bundleDir, 0o750); mkErr != nil {
		return "", "", fmt.Errorf("create TUI cache dir: %w", mkErr)
	}
	if writeErr := os.WriteFile(entry, tuiBundleJS, 0o600); writeErr != nil {
		return "", "", fmt.Errorf("write TUI bundle: %w", writeErr)
	}
	log.Debug("extracted embedded TUI bundle", "path", entry, "bytes", len(tuiBundleJS))
	return entry, bundleDir, nil
}

// tuiBundleHash returns a short content hash of the embedded bundle.
// Used to pick a cache directory that changes when the binary is upgraded.
func tuiBundleHash() [8]byte {
	var h [8]byte
	// FNV-1a 64-bit, kept inline to avoid another import.
	hash := uint64(14695981039346656037)
	for _, b := range tuiBundleJS {
		hash ^= uint64(b)
		hash *= 1099511628211
	}
	for i := 0; i < 8; i++ {
		h[i] = byte(hash >> (i * 8))
	}
	return h
}
