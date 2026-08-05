// Package tmux provides tmux session management for agent orchestration.
//
// Each mycel agent runs in an isolated tmux session, allowing for:
//   - Concurrent agent execution
//   - Session persistence across restarts
//   - Direct terminal access for debugging
//
// # Basic Usage
//
// Create a session manager:
//
//	mgr := tmux.NewManagerWithRepo("mycel-", "/path/to/repo")
//
// Create a session:
//
//	err := mgr.CreateSession("eng-01", "/path/to/worktree")
//
// Send commands to a session:
//
//	err := mgr.SendKeys("eng-01", "echo hello")
//
// Capture output:
//
//	output, err := mgr.CapturePane("eng-01", 100) // last 100 lines
//
// # Session Naming
//
// Sessions are prefixed and optionally include a repo hash for isolation:
//
//	// With repo hash: mycel-a1b2c3-eng-01
//	mgr := tmux.NewManagerWithRepo("mycel-", "/path/to/repo")
//
//	// Without hash: mycel-eng-01
//	mgr := tmux.NewManager("mycel-")
//
// # Caching
//
// The manager caches session existence checks to reduce subprocess calls.
// Cache is automatically invalidated when sessions are created or destroyed.
package tmux

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// validEnvVarName matches valid POSIX environment variable names:
// Must start with letter or underscore, followed by letters, digits, or underscores.
// This prevents shell injection through malicious key names.
var validEnvVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Session represents a tmux session.
type Session struct {
	Name      string
	Created   string
	Directory string
	Windows   int
	Attached  bool
}

// DefaultCacheTTL is the default time-to-live for cached session data.
const DefaultCacheTTL = 2 * time.Second

// DefaultPrefix is the canonical tmux session-name prefix.
const DefaultPrefix = "mycel-"

// Manager handles tmux session operations.
type Manager struct {
	// Session cache for reducing tmux subprocess calls (#980).
	// Cache is invalidated on CreateSession/KillSession/RenameSession/KillServer.
	sessionsCacheAt time.Time // When sessions cache was populated
	hasCacheAt      time.Time // When hasSession cache was populated
	execCommand     func(name string, arg ...string) *exec.Cmd
	sessionLocks    map[string]*sync.Mutex
	hasSessionCache map[string]bool // Cached session existence checks
	sessionsCache   []Session       // Cached list of sessions
	SessionPrefix   string          // Prepended to all session names (e.g., "mycel-")
	// DefaultShell is the absolute path of the shell used for new sessions
	// (prefs runtime.tmux.default_shell). Empty means "/bin/bash".
	DefaultShell string
	cacheTTL     time.Duration // Cache TTL (default: 2 seconds)
	// HistoryLimit is applied as tmux history-limit on create when > 0
	// (prefs runtime.tmux.history_limit).
	HistoryLimit int
	cacheMu      sync.RWMutex // Protects cache fields
	sessionMu    sync.Mutex   // Protects per-session SendKeys serialization
}

// NormalizeSessionPrefix ensures a trailing "-" so agent names do not glue
// to the prefix. Prefs historically default to "mycel" while code used
// "mycel-"; both become "mycel-".
func NormalizeSessionPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return DefaultPrefix
	}
	if !strings.HasSuffix(prefix, "-") {
		return prefix + "-"
	}
	return prefix
}

// safeShellPath allows absolute shell paths only (no spaces / metacharacters).
var safeShellPath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

func (m *Manager) sessionShell() string {
	if m.DefaultShell != "" && safeShellPath.MatchString(m.DefaultShell) {
		return m.DefaultShell
	}
	return "/bin/bash"
}

// command returns an exec.Cmd using the configured executor.
func (m *Manager) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if m.execCommand != nil {
		return m.execCommand(name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}

// userFriendlyTmuxError converts raw tmux error output to a user-friendly message.
// It detects common error patterns and returns clearer, actionable messages.
func userFriendlyTmuxError(output string) string {
	output = strings.TrimSpace(output)
	outputLower := strings.ToLower(output)

	switch {
	case strings.Contains(outputLower, "can't find pane"):
		return "session not found (may have terminated)"
	case strings.Contains(outputLower, "can't find session"):
		return "session not found (may have terminated)"
	case strings.Contains(outputLower, "no server running"):
		return "tmux server not running"
	case strings.Contains(outputLower, "session not found"):
		return "session not found (may have terminated)"
	case strings.Contains(outputLower, "can't find window"):
		return "session window not found"
	default:
		// For other errors, return a shortened version without internal session IDs
		if len(output) > 50 {
			return output[:50] + "..."
		}
		return output
	}
}

// NewManager creates a new tmux manager with the given prefix.
func NewManager(prefix string) *Manager {
	return &Manager{
		SessionPrefix:   prefix,
		execCommand:     exec.Command,
		hasSessionCache: make(map[string]bool),
		cacheTTL:        DefaultCacheTTL,
	}
}

// NewDefaultManager creates a new tmux manager with the canonical prefix.
func NewDefaultManager() *Manager {
	return &Manager{
		SessionPrefix:   DefaultPrefix,
		execCommand:     exec.Command,
		hasSessionCache: make(map[string]bool),
		cacheTTL:        DefaultCacheTTL,
	}
}

// WithExecCommand returns a copy of the Manager with a custom command executor.
// This is primarily used for testing to mock subprocess calls.
func (m *Manager) WithExecCommand(fn func(string, ...string) *exec.Cmd) *Manager {
	m.execCommand = fn
	return m
}

// SessionName returns the full session name with prefix.
//
// Deliberately not namespaced by anything else. Session names used to carry a
// hash of the repo the daemon booted in, from when state lived in a directory
// inside each project and two projects could genuinely collide. State is one
// ~/.mycel now, agent names are unique across it, and the hash only meant that
// two daemons started in different directories could not see each other's
// sessions — so opening the desktop app left every running agent listed and
// unreachable (#3569).
func (m *Manager) SessionName(name string) string {
	return m.SessionPrefix + name
}

// HasSession checks if a session exists.
// Results are cached with a short TTL to reduce subprocess calls.
func (m *Manager) HasSession(ctx context.Context, name string) bool {
	fullName := m.SessionName(name)

	// Check cache first
	m.cacheMu.RLock()
	ttl := m.cacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	if time.Since(m.hasCacheAt) < ttl {
		if exists, ok := m.hasSessionCache[fullName]; ok {
			m.cacheMu.RUnlock()
			return exists
		}
	}
	m.cacheMu.RUnlock()

	// Cache miss - query tmux
	cmd := m.command(ctx, "tmux", "has-session", "-t", fullName)
	exists := cmd.Run() == nil

	// Update cache
	m.cacheMu.Lock()
	if m.hasSessionCache == nil {
		m.hasSessionCache = make(map[string]bool)
	}
	m.hasSessionCache[fullName] = exists
	m.hasCacheAt = time.Now()
	m.cacheMu.Unlock()

	return exists
}

// invalidateCache clears all cached session data.
// Call this after creating or killing sessions.
func (m *Manager) invalidateCache() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.sessionsCache = nil
	m.sessionsCacheAt = time.Time{}
	m.hasSessionCache = make(map[string]bool)
	m.hasCacheAt = time.Time{}
}

// CreateSession creates a new tmux session.
func (m *Manager) CreateSession(ctx context.Context, name, dir string) error {
	fullName := m.SessionName(name)
	log.Debug("creating tmux session", "name", fullName, "dir", dir)

	args := []string{"new-session", "-d", "-s", fullName}
	if dir != "" {
		args = append(args, "-c", dir)
	}

	cmd := m.command(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session %s: %w (%s)", fullName, err, string(output))
	}

	// Invalidate cache after creating session
	m.invalidateCache()
	return nil
}

// CreateSessionWithCommand creates a session and runs a command.
func (m *Manager) CreateSessionWithCommand(ctx context.Context, name, dir, command string) error {
	return m.CreateSessionWithEnv(ctx, name, dir, command, nil)
}

// CreateSessionWithEnv creates a session with env vars baked into the shell command.
// Environment variable keys are validated to prevent shell injection attacks.
// Keys must match POSIX standards: start with letter/underscore, contain only alphanumerics/underscores.
func (m *Manager) CreateSessionWithEnv(ctx context.Context, name, dir, command string, env map[string]string) error {
	fullName := m.SessionName(name)

	// Build shell command with env vars prefixed
	parts := make([]string, 0, len(env)+2)
	// Unset CLAUDECODE so spawned Claude doesn't detect a nested session
	parts = append(parts, "unset CLAUDECODE;")
	for k, v := range env {
		// Validate env var key to prevent shell injection
		if !validEnvVarName.MatchString(k) {
			return fmt.Errorf("invalid environment variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", k)
		}
		parts = append(parts, fmt.Sprintf("export %s=%q;", k, v))
	}
	parts = append(parts, command)
	shellCmd := strings.Join(parts, " ")

	args := []string{"new-session", "-d", "-s", fullName}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	args = append(args, m.sessionShell(), "-c", shellCmd)

	cmd := m.command(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session %s: %w (%s)", fullName, err, string(output))
	}

	if m.HistoryLimit > 0 {
		histCmd := m.command(ctx, "tmux", "set-option", "-t", fullName, "history-limit", strconv.Itoa(m.HistoryLimit))
		if histOut, histErr := histCmd.CombinedOutput(); histErr != nil {
			log.Warn("tmux history-limit not applied", "session", fullName, "error", histErr, "output", string(histOut))
		}
	}

	// Invalidate cache after creating session
	m.invalidateCache()
	return nil
}

// KillSession kills a tmux session.
func (m *Manager) KillSession(ctx context.Context, name string) error {
	fullName := m.SessionName(name)
	log.Debug("killing tmux session", "name", fullName)
	cmd := m.command(ctx, "tmux", "kill-session", "-t", fullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill session %s: %w (%s)", fullName, err, string(output))
	}

	// Invalidate cache after killing session
	m.invalidateCache()
	return nil
}

// RenameSession renames a tmux session.
func (m *Manager) RenameSession(ctx context.Context, oldName, newName string) error {
	oldFullName := m.SessionName(oldName)
	newFullName := m.SessionName(newName)
	log.Debug("renaming tmux session", "old", oldFullName, "new", newFullName)
	cmd := m.command(ctx, "tmux", "rename-session", "-t", oldFullName, newFullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rename session %s to %s: %w (%s)", oldFullName, newFullName, err, string(output))
	}

	// Invalidate cache after renaming session
	m.invalidateCache()
	return nil
}

// getSessionLock returns a mutex for the given session name, creating one if needed.
// This serializes concurrent SendKeys calls to the same session.
func (m *Manager) getSessionLock(sessionName string) *sync.Mutex {
	m.sessionMu.Lock()
	defer m.sessionMu.Unlock()
	if m.sessionLocks == nil {
		m.sessionLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := m.sessionLocks[sessionName]
	if !ok {
		mu = &sync.Mutex{}
		m.sessionLocks[sessionName] = mu
	}
	return mu
}

// SendKeys sends keys to a session with Enter as submit key.
// This is a convenience wrapper around SendKeysWithSubmit.
func (m *Manager) SendKeys(ctx context.Context, name, keys string) error {
	return m.SendKeysWithSubmit(ctx, name, keys, "Enter")
}

// SendKeysWithSubmit sends keys to a session with a specified submit key.
// For messages longer than 500 chars, uses tmux load-buffer/paste-buffer to avoid truncation.
// Trailing newlines are trimmed. submitKey specifies what to send after the message:
// - "Enter" sends the Enter key as a tmux key-name event
// - "" sends nothing (message is left in input buffer)
// - Other values are sent as tmux key names (e.g., "C-m" for Ctrl+M)
func (m *Manager) SendKeysWithSubmit(ctx context.Context, name, keys, submitKey string) error {
	keys = strings.TrimRight(keys, "\n")
	fullName := m.SessionName(name)

	// Serialize sends to the same session to prevent interleaving
	mu := m.getSessionLock(fullName)
	mu.Lock()
	defer mu.Unlock()

	if len(keys) <= 500 {
		// Send text literally (no key-name lookup)
		cmd := m.command(ctx, "tmux", "send-keys", "-t", fullName, "-l", keys)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to send message: %s", userFriendlyTmuxError(string(output)))
		}
	} else {
		// Long message: use temp file + load-buffer + paste-buffer with named buffer
		// Use a unique buffer name with session identifier to prevent race conditions
		// even under concurrent pressure to same session
		sessionID := fullName
		if len(fullName) > 15 {
			sessionID = fullName[:15]
		}
		bufferName := "s" + strings.ReplaceAll(sessionID, "-", "") + "-" + generateBufferName()

		tmpDir := filepath.Join(os.TempDir(), "mycel-tmux")
		if err := os.MkdirAll(tmpDir, 0700); err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		tmpFile, err := os.CreateTemp(tmpDir, "send-*.txt")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer func() { _ = os.Remove(tmpPath) }() //nolint:errcheck // deferred cleanup

		if _, err := tmpFile.WriteString(keys); err != nil {
			_ = tmpFile.Close() //nolint:errcheck // closing on error path
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		_ = tmpFile.Close() //nolint:errcheck // closing before load

		// Load into named buffer
		loadCmd := m.command(ctx, "tmux", "load-buffer", "-b", bufferName, tmpPath)
		if output, err := loadCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to load buffer: %w (%s)", err, string(output))
		}

		// Paste from named buffer and delete it afterward
		pasteCmd := m.command(ctx, "tmux", "paste-buffer", "-b", bufferName, "-d", "-t", fullName)
		if output, err := pasteCmd.CombinedOutput(); err != nil {
			// Clean up buffer on error
			_ = m.command(ctx, "tmux", "delete-buffer", "-b", bufferName).Run() //nolint:errcheck // best-effort cleanup
			return fmt.Errorf("failed to send message: %s", userFriendlyTmuxError(string(output)))
		}
	}

	if submitKey == "" {
		return nil
	}

	// Send the submit key as a separate operation.
	// Use -H 0D (raw hex carriage return byte) for Enter instead of the key name.
	// In tmux 3.5+, "send-keys Enter" (key name resolution) is unreliable after
	// paste-buffer operations — the key is silently dropped after bracketed paste.
	// The -H flag sends the raw byte directly, bypassing key resolution, and works
	// reliably in all scenarios including after paste-buffer.
	delay := 100 * time.Millisecond
	if len(keys) > 500 {
		delay = 500 * time.Millisecond
	}
	time.Sleep(delay)

	var submitCmd *exec.Cmd
	if submitKey == "Enter" {
		// Send raw CR byte (0x0D) via -H flag — reliable after paste-buffer.
		submitCmd = m.command(ctx, "tmux", "send-keys", "-t", fullName, "-H", "0D")
	} else {
		submitCmd = m.command(ctx, "tmux", "send-keys", "-t", fullName, submitKey)
	}
	if output, err := submitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to send submit key to %s: %w (%s)", fullName, err, string(output))
	}

	return nil
}

// Capture captures the current pane content.
func (m *Manager) Capture(ctx context.Context, name string, lines int) (string, error) {
	fullName := m.SessionName(name)

	args := []string{"capture-pane", "-t", fullName, "-p"}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	cmd := m.command(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %s: %w", fullName, err)
	}
	return string(output), nil
}

// ListSessions lists all sessions with our prefix.
// Results are cached with a short TTL to reduce subprocess calls.
func (m *Manager) ListSessions(ctx context.Context) ([]Session, error) {
	// Check cache first
	m.cacheMu.RLock()
	ttl := m.cacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	if time.Since(m.sessionsCacheAt) < ttl && m.sessionsCache != nil {
		// Return a copy to prevent mutation
		result := make([]Session, len(m.sessionsCache))
		copy(result, m.sessionsCache)
		m.cacheMu.RUnlock()
		return result, nil
	}
	m.cacheMu.RUnlock()

	// Cache miss - query tmux
	cmd := m.command(ctx, "tmux", "list-sessions", "-F",
		"#{session_name}|#{session_created_string}|#{session_attached}|#{session_windows}|#{session_path}")

	output, err := cmd.Output()
	if err != nil {
		// tmux list-sessions fails with various messages when no server/sessions
		// exist: "no server running", "error connecting to ...", etc.
		// If it's an exit error, there are simply no sessions available.
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, err
	}

	fullPrefix := m.SessionPrefix

	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		name := parts[0]
		if !strings.HasPrefix(name, fullPrefix) {
			continue
		}
		matchedPrefix := fullPrefix

		windows, _ := strconv.Atoi(parts[3])
		sessions = append(sessions, Session{
			Name:      strings.TrimPrefix(name, matchedPrefix),
			Created:   parts[1],
			Directory: parts[4],
			Windows:   windows,
			Attached:  parts[2] == "1",
		})
	}

	// Update cache
	m.cacheMu.Lock()
	m.sessionsCache = make([]Session, len(sessions))
	copy(m.sessionsCache, sessions)
	m.sessionsCacheAt = time.Now()
	m.cacheMu.Unlock()

	return sessions, nil
}

// AttachCmd returns an exec.Cmd to attach to a session.
// The caller should set Stdin/Stdout/Stderr and run it.
func (m *Manager) AttachCmd(ctx context.Context, name string) *exec.Cmd {
	fullName := m.SessionName(name)
	return m.command(ctx, "tmux", "attach-session", "-t", fullName)
}

// IsRunning checks if tmux server is running.
func (m *Manager) IsRunning(ctx context.Context) bool {
	cmd := m.command(ctx, "tmux", "list-sessions")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// "no server running" means tmux is available but no sessions
		if strings.Contains(stderr.String(), "no server running") {
			return false
		}
	}
	return err == nil
}

// KillServer kills the tmux server (all sessions).
func (m *Manager) KillServer(ctx context.Context) error {
	cmd := m.command(ctx, "tmux", "kill-server")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill tmux server: %w (%s)", err, string(output))
	}

	// Invalidate cache after killing server
	m.invalidateCache()
	return nil
}

// SetEnvironment sets an environment variable in a session.
// Environment variable key is validated to prevent shell injection attacks.
// Key must match POSIX standards: start with letter/underscore, contain only alphanumerics/underscores.
func (m *Manager) SetEnvironment(ctx context.Context, name, key, value string) error {
	// Validate env var key to prevent shell injection
	if !validEnvVarName.MatchString(key) {
		return fmt.Errorf("invalid environment variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", key)
	}
	fullName := m.SessionName(name)
	cmd := m.command(ctx, "tmux", "set-environment", "-t", fullName, key, value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set environment %s=%s in session %s: %w (%s)", key, value, fullName, err, string(output))
	}
	return nil
}

// PipePane starts or stops streaming a session's output to a log file.
// If logPath is non-empty, starts piping output via "cat >> logPath".
// If logPath is empty, stops any existing pipe.
func (m *Manager) PipePane(ctx context.Context, name, logPath string) error {
	fullName := m.SessionName(name)

	if logPath == "" {
		// Stop pipe
		cmd := m.command(ctx, "tmux", "pipe-pane", "-t", fullName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to stop pipe-pane for %s: %w (%s)", fullName, err, string(output))
		}
		return nil
	}

	// Start pipe: stream output to log file with append.
	// Shell-quote logPath to prevent injection via metacharacters.
	quoted := "'" + strings.ReplaceAll(logPath, "'", "'\\''") + "'"
	pipeCmd := fmt.Sprintf("cat >> %s", quoted)
	cmd := m.command(ctx, "tmux", "pipe-pane", "-t", fullName, pipeCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start pipe-pane for %s: %w (%s)", fullName, err, string(output))
	}
	return nil
}

// generateBufferName creates a unique buffer name for tmux operations.
// This prevents race conditions when multiple goroutines send keys concurrently.
func generateBufferName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read always returns len(b), nil
	return "mycel-" + hex.EncodeToString(b)
}
