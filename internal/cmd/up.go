package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// normalizeAddr ensures the host part of a host:port address is not empty.
// If the host is missing (e.g. ":8080"), it defaults to "127.0.0.1".
func normalizeAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr // not a host:port pair, return as-is
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start mycel server",
	Long: `Start the mycel server (API, web UI, MCP, agent management).

Bootstraps everything on first run — no separate init step:
  - inside a git repo: the repo root is adopted as the anchor repo
    (state lives under ~/.mycel, the repo stays pristine) and new
    agents default their repo to it
  - outside a git repo: the server boots against MycelHome only

By default the server runs in the foreground (for Docker/Railway).
Use -d to run as a background daemon.

Examples:
  mycel up                              # Foreground (Docker/Railway)
  mycel up -d                           # Background daemon
  mycel up --addr 0.0.0.0:9374         # Custom listen address
  mycel up --workspace /path/to/ws     # Explicit workspace`,
	RunE: runUp,
}

var (
	upAddr      string
	upWorkspace string
	upDaemon    bool
	upCORS      string
	upAPIKey    string
)

func init() {
	upCmd.Flags().StringVar(&upAddr, "addr", "127.0.0.1:9374", "Listen address (host:port)")
	upCmd.Flags().StringVar(&upWorkspace, "workspace", "", "Workspace directory (defaults to current workspace)")
	upCmd.Flags().BoolVarP(&upDaemon, "daemon", "d", false, "Run as background daemon")
	upCmd.Flags().StringVar(&upCORS, "cors-origin", "*", "CORS allowed origin")
	upCmd.Flags().StringVar(&upAPIKey, "api-key", os.Getenv("MYCEL_API_KEY"), "API key for Bearer token auth (or set MYCEL_API_KEY)")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, _ []string) error {
	wsRoot := upWorkspace
	if wsRoot == "" {
		wsRoot = resolveUpWorkspace()
	} else {
		// Validate the workspace path: it must exist and be a git repo.
		// It does NOT need to be initialized — the server bootstraps
		// uninitialized repos via workspace.Init (idempotent).
		abs, absErr := filepath.Abs(wsRoot)
		if absErr != nil {
			return fmt.Errorf("cannot resolve repo path %s: %w", wsRoot, absErr)
		}
		if _, statErr := os.Stat(filepath.Join(abs, ".git")); statErr != nil {
			return fmt.Errorf("%s is not a git repository (mycel needs git for agent worktrees)", abs)
		}
		wsRoot = abs
	}

	// Read server config from preferences.json for defaults
	if wsRoot != "" {
		if ws, loadErr := workspace.Load(wsRoot); loadErr == nil && ws.Config != nil {
			// Use preferences.json addr if --addr wasn't explicitly set
			if !cmd.Flags().Changed("addr") {
				host := ws.Config.Server.Host
				if host == "" {
					host = "127.0.0.1"
				}
				port := 9374
				if ws.Config.Server.Port > 0 {
					port = ws.Config.Server.Port
				}
				upAddr = fmt.Sprintf("%s:%d", host, port)
			}
		}
	}

	// Normalize addr: ":8080" → "127.0.0.1:8080"
	upAddr = normalizeAddr(upAddr)

	// Daemon mode: re-exec bc up in background
	if upDaemon {
		return runUpDaemon(wsRoot)
	}

	// Foreground mode: run server directly
	if wsRoot != "" {
		fmt.Printf("Starting mycel server in %s\n", wsRoot)
	} else {
		fmt.Println("Starting mycel server (no repo yet — add one from the web UI, or run 'mycel up' inside a git repo)")
	}
	fmt.Printf("  addr: %s\n\n", upAddr)

	// Lazy-start the bc-db container when the workspace is configured
	// for TimescaleDB storage. SQLite (the default) needs nothing.
	maybeBootstrapTimescale(wsRoot)

	// Set MYCEL_DAEMON_ADDR so agents inherit the correct server address for hooks.
	// Without this, agents default to :9374 even when bcd runs on a different port.
	bcdAddr := "http://" + upAddr
	if err := os.Setenv("MYCEL_DAEMON_ADDR", bcdAddr); err == nil {
		fmt.Printf("  MYCEL_DAEMON_ADDR: %s\n", bcdAddr)
	}

	// Publish the listen address at ~/.mycel/daemon.addr so the mycel CLI and
	// agents can find the daemon without MYCEL_DAEMON_ADDR when it runs on
	// a non-default port. Best-effort — failure to write is not fatal,
	// but each failure mode must warn so users aren't silently routed
	// back to the hardcoded :9374 default (the exact bug #43 fixed).
	if _, ensureErr := workspace.EnsureGlobalDir(); ensureErr != nil {
		log.Warn("daemon addr: ensure ~/.mycel failed — CLI will fall back to default port", "error", ensureErr)
	} else if addrPath, pathErr := workspace.DaemonAddrPath(); pathErr != nil {
		log.Warn("daemon addr: resolve path failed — CLI will fall back to default port", "error", pathErr)
	} else if writeErr := os.WriteFile(addrPath, []byte(bcdAddr+"\n"), 0o600); writeErr != nil {
		log.Warn("daemon addr: write failed — CLI will fall back to default port", "path", addrPath, "error", writeErr)
	}

	return RunServer(upAddr, wsRoot, upCORS, upAPIKey)
}

// resolveUpWorkspace picks the anchor repo for `mycel up`. The daemon is
// single-tenant and only needs MycelHome to boot; the repo is the default
// that new agents bind to:
//
//  1. MYCEL_WORKSPACE or a known workspace enclosing cwd
//  2. the enclosing git repo root — adopted as the anchor repo
//     (the server runs workspace.Init, which is idempotent)
//  3. "" — boot against MycelHome only; new agents must name a repo
func resolveUpWorkspace() string {
	if ws, err := getRepo(); err == nil && ws != nil {
		return ws.RootDir
	}
	if root := findGitRoot(); root != "" {
		return root
	}
	return ""
}

// findGitRoot walks up from cwd looking for a .git entry (dir for normal
// repos, file for worktrees/submodules). Returns "" when cwd is not
// inside a git repository.
func findGitRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// maybeBootstrapTimescale starts the bc-db (TimescaleDB) container when
// the workspace's storage.default is "timescale". Non-fatal: warns if
// Docker is unavailable. SQLite workspaces (the default) skip this.
func maybeBootstrapTimescale(wsRoot string) {
	if wsRoot == "" {
		return
	}
	ws, err := workspace.Load(wsRoot)
	if err != nil || ws.Config == nil || ws.Config.Storage.Default != "timescale" {
		return
	}

	// Honor the configured connection settings; only bootstrap the local
	// container for a localhost target — a remote Timescale is the user's.
	ts := ws.Config.Storage.Timescale
	host := ts.Host
	if host == "" {
		host = "localhost"
	}
	if host != "localhost" && host != "127.0.0.1" {
		return
	}
	port := ts.Port
	if port == 0 {
		port = 5432
	}
	password := ts.Password
	if password == "" {
		password = "bc"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := dockerRun(ctx, "bc-db", []string{
		"-p", fmt.Sprintf("%d:5432", port),
		"-e", "POSTGRES_PASSWORD=" + password,
		"-v", "bc-db-data:/var/lib/postgresql/data",
		"--restart", "always",
		"bc-bcdb:latest",
	}); err != nil {
		fmt.Printf("  %s bc-db: %v\n", ui.YellowText("warning"), err)
		return
	}
	fmt.Printf("  %s database ready\n\n", ui.GreenText("ok"))
}

// runUpDaemon starts bc up in the background by re-executing the mycel binary.
// Logs go to ~/.mycel/daemon.log, PID to ~/.mycel/daemon.pid.
func runUpDaemon(wsRoot string) error {
	if _, err := workspace.EnsureGlobalDir(); err != nil {
		return fmt.Errorf("ensure bc home: %w", err)
	}

	pidPath, err := workspace.DaemonPidPath()
	if err != nil {
		return fmt.Errorf("resolve daemon pid path: %w", err)
	}

	// Check if already running
	if pidData, readErr := os.ReadFile(pidPath); readErr == nil { //nolint:gosec // controlled home path
		pid := strings.TrimSpace(string(pidData))
		checkCmd := exec.CommandContext(context.Background(), "kill", "-0", pid) //nolint:gosec // trusted
		if checkCmd.Run() == nil {
			fmt.Printf("  mycel server already running (PID %s)\n", pid)
			fmt.Printf("  http://%s\n", upAddr)
			return nil
		}
	}

	// Find our own binary to re-exec
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find mycel binary: %w", err)
	}

	logPath, err := workspace.DaemonLogPath()
	if err != nil {
		return fmt.Errorf("resolve daemon log path: %w", err)
	}

	// Build args for foreground mode (without -d)
	args := []string{
		"up",
		"--addr", upAddr,
		"--cors-origin", upCORS,
	}
	if wsRoot != "" {
		args = append(args, "--workspace", wsRoot)
	}
	if upAPIKey != "" {
		args = append(args, "--api-key", upAPIKey)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) //nolint:gosec // controlled path
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	// Detach stdin so the child survives terminal close.
	nullFile, nullErr := os.Open(os.DevNull)
	if nullErr != nil {
		_ = logFile.Close()
		return fmt.Errorf("open %s: %w", os.DevNull, nullErr)
	}

	cmd := exec.CommandContext(context.Background(), selfPath, args...) //nolint:gosec // trusted binary
	cmd.Stdin = nullFile
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = wsRoot
	cmd.Env = os.Environ()
	// Start in a new session so SIGHUP from terminal close doesn't propagate (no-op on Windows).
	detachSession(cmd)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = nullFile.Close()
		return fmt.Errorf("start mycel server: %w", err)
	}
	_ = logFile.Close()
	_ = nullFile.Close()

	// Write PID file
	if writeErr := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0600); writeErr != nil {
		log.Warn("failed to write PID file", "path", pidPath, "error", writeErr)
	}

	// Detach — don't wait for the process
	_ = cmd.Process.Release()

	fmt.Printf("  %s mycel server started (PID %d)\n", ui.GreenText("ok"), cmd.Process.Pid)
	fmt.Printf("  http://%s\n", upAddr)
	fmt.Printf("  logs: %s\n", logPath)
	fmt.Printf("  pid:  %s\n", pidPath)
	fmt.Println()

	return nil
}

// wsID returns a short workspace hash for container naming.
func wsID(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h[:3])
}

// dockerRun starts a container if not already running.
func dockerRun(ctx context.Context, name string, args []string) error {
	// Check if already running
	//nolint:gosec // trusted
	out, _ := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	if strings.TrimSpace(string(out)) == "true" {
		fmt.Printf("  %s %s (already running)\n", ui.GreenText("ok"), name)
		return nil
	}

	// Remove stale container
	//nolint:gosec // trusted
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()

	// Start
	fmt.Printf("  Starting %s... ", name)
	cmdArgs := append([]string{"run", "-d", "--name", name}, args...)
	//nolint:gosec // trusted
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Println(ui.YellowText(fmt.Sprintf("failed (%v)", err)))
		log.Debug("docker run failed", "name", name, "output", string(output))
		return fmt.Errorf("container %s: %w", name, err)
	}
	fmt.Println(ui.GreenText("started"))
	return nil
}
