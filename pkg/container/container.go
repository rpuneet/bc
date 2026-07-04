// Package container implements a Docker-based runtime backend for agent sessions.
//
// Each agent runs in an isolated Docker container with tmux inside for session
// management. This provides process isolation and resource limits while maintaining
// the same interactive experience as the native tmux backend.
//
// Communication uses `docker exec ... tmux send-keys` — no persistent connections
// or FIFOs needed.
package container

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/runtime"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// validEnvVarName matches valid POSIX environment variable names:
// Must start with letter or underscore, followed by letters, digits, or underscores.
// This prevents injection through malicious key names passed to docker run -e.
var validEnvVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateMount checks that a Docker mount spec (src:dst[:opts]) has a safe
// source path. It rejects path traversal (../) and absolute paths outside
// the workspace root. workspaceRoot must be an absolute, cleaned path.
func validateMount(mount, workspaceRoot string) error {
	parts := strings.SplitN(mount, ":", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid mount format %q: expected src:dst", mount)
	}
	src := parts[0]
	if strings.Contains(src, "..") {
		return fmt.Errorf("mount source %q contains path traversal", src)
	}
	cleaned := filepath.Clean(src)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("mount source %q must be an absolute path", src)
	}
	// Resolve symlinks to prevent bypass (e.g., workspace/escape -> /etc)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// Path may not exist yet — fall back to cleaned path check
		resolved = cleaned
	}
	if !strings.HasPrefix(resolved, workspaceRoot+string(filepath.Separator)) && resolved != workspaceRoot {
		return fmt.Errorf("mount source %q resolves outside workspace root %q", src, workspaceRoot)
	}
	return nil
}

// Ensure Backend implements runtime.Backend.
var _ runtime.Backend = (*Backend)(nil)

// Config holds Docker runtime configuration.
type Config struct {
	Image       string
	Network     string
	ExtraMounts []string
	CPUs        float64
	MemoryMB    int64
}

// ConfigFromWorkspace converts workspace DockerRuntimeConfig to container Config.
func ConfigFromWorkspace(dcfg workspace.DockerRuntimeConfig) Config {
	cfg := Config{
		Image:       dcfg.Image,
		Network:     dcfg.Network,
		ExtraMounts: dcfg.ExtraMounts,
		CPUs:        dcfg.CPUs,
		MemoryMB:    dcfg.MemoryMB,
	}
	if cfg.Image == "" {
		cfg.Image = "mycel-agent-claude:latest"
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = 2.0
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = 2048
	}
	if cfg.Network == "" {
		cfg.Network = "bridge"
	}
	return cfg
}

// Backend manages Docker containers as agent sessions.
// Each container runs tmux internally for interactive session management.
type Backend struct {
	logCancels        map[string]context.CancelFunc
	providerRegistry  *provider.Registry
	prefix            string
	workspaceHash     string
	workspacePath     string
	hostWorkspacePath string // host path for Docker-in-Docker mounts (from BC_HOST_WORKSPACE)
	cfg               Config
	mu                sync.RWMutex
}

// NewBackend creates a Docker runtime backend.
// Returns an error if the Docker daemon is not reachable.
func NewBackend(cfg Config, prefix, workspacePath string, registry *provider.Registry) (*Backend, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "info") //nolint:gosec // trusted binary
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker daemon not available: %w", err)
	}

	// Use host workspace path for volume mounts in Docker-in-Docker setups.
	// BC_HOST_WORKSPACE is set by `bc up` when the daemon runs in Docker.
	hostPath := workspacePath
	if hp := os.Getenv("BC_HOST_WORKSPACE"); hp != "" {
		hostPath = hp
	}

	h := sha256.Sum256([]byte(hostPath))
	return &Backend{
		cfg:               cfg,
		prefix:            prefix,
		workspaceHash:     fmt.Sprintf("%x", h[:3]),
		workspacePath:     workspacePath,
		hostWorkspacePath: hostPath,
		providerRegistry:  registry,
		logCancels:        make(map[string]context.CancelFunc),
	}, nil
}

// containerName returns the Docker container name for an agent.
func (b *Backend) containerName(name string) string {
	return b.prefix + b.workspaceHash + "-" + name
}

// validContainerAgentName is the same identifier barrier pkg/agent
// enforces; duplicated here (container cannot import agent) so tainted
// names never reach path construction.
var validContainerAgentName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// containerAgentDir returns the agent's state directory inside the
// container (bcd side). The flat layout at <MycelHome>/agents/<name>/
// is canonical; agents whose state still lives at an older location
// (nested per-workspace dir or the legacy <root>/.bc/ sidecar) keep
// using it until migrated.
func (b *Backend) containerAgentDir(agentName string) string {
	if !validContainerAgentName.MatchString(agentName) {
		return ""
	}
	var flat string
	if home, err := workspace.MycelHome(); err == nil {
		flat = filepath.Join(home, "agents", agentName)
		if _, statErr := os.Stat(flat); statErr == nil {
			return flat
		}
	}
	// Existing agents may still live in older layouts.
	if globalDir, err := workspace.GlobalStateDir(b.workspacePath); err == nil {
		nested := filepath.Join(globalDir, "agents", agentName)
		if _, statErr := os.Stat(nested); statErr == nil {
			return nested
		}
	}
	legacy := filepath.Join(b.workspacePath, ".bc", "agents", agentName)
	if _, statErr := os.Stat(legacy); statErr == nil {
		return legacy
	}
	if flat != "" {
		return flat // canonical location for fresh agents
	}
	return legacy
}

// hostAgentDir returns the host path that maps to the agent's state
// directory, used on the -v mount flag. For bcd running on the host this
// mirrors containerAgentDir; for Docker-in-Docker setups the BC_HOST_BC_HOME
// env var (if set) translates the container's ~/.mycel/ to the host's.
func (b *Backend) hostAgentDir(agentName, hostRoot string) string {
	containerDir := b.containerAgentDir(agentName)
	// Only translate when the path is under BC_HOME (the M11 global dir).
	bcHome, err := workspace.MycelHome()
	if err == nil && strings.HasPrefix(containerDir, bcHome+string(filepath.Separator)) {
		if hostBCHome := os.Getenv("BC_HOST_BC_HOME"); hostBCHome != "" {
			rel := strings.TrimPrefix(containerDir, bcHome)
			return filepath.Join(hostBCHome, rel)
		}
		return containerDir
	}
	// Legacy path (<root>/.bc/agents/<name>): translate via hostRoot.
	rel, relErr := filepath.Rel(b.workspacePath, containerDir)
	if relErr != nil {
		return containerDir
	}
	return filepath.Join(hostRoot, rel)
}

// resolveRepoMount picks the repository mounted at /workspace and the
// container working directory for an agent session.
//
// The agent's repo arrives via env["BC_WORKSPACE"], which the agent
// manager sets from Agent.Repo (mirroring worktreeManagerFor on the
// tmux path). An empty value — or the boot workspace path itself —
// means the boot repo. Any other repo is mounted instead, so agents
// bound to a different repo get THEIR repo at /workspace, not the
// boot repo.
//
// Returns the host-side mount source (for -v) and the container
// workdir (for -w). dir is the agent's worktree as bcd sees it; when
// it lives under the mounted repo, the workdir points at it.
func (b *Backend) resolveRepoMount(dir string, env map[string]string) (hostRepo, workdir string, err error) {
	// Boot repo defaults. BC_HOST_WORKSPACE (Docker-in-Docker) only
	// translates the boot workspace path.
	repoRoot := b.workspacePath
	hostRepo = b.hostWorkspacePath
	if hostRepo == "" {
		hostRepo = b.workspacePath
	}

	if agentRepo := env["BC_WORKSPACE"]; agentRepo != "" && filepath.Clean(agentRepo) != filepath.Clean(b.workspacePath) {
		// The repo value originates from the create-agent API — require
		// an absolute, traversal-free path before it becomes a -v mount
		// source. Like validateMount, ".." is checked on the raw value
		// so traversal cannot be smuggled past filepath.Clean.
		cleaned := filepath.Clean(agentRepo)
		if !filepath.IsAbs(cleaned) || strings.Contains(agentRepo, "..") {
			return "", "", fmt.Errorf("agent repo %q must be an absolute path without traversal", agentRepo)
		}
		repoRoot = cleaned
		// Cross-repo paths are host paths already; no BC_HOST_WORKSPACE
		// translation applies.
		hostRepo = cleaned
	}

	workdir = "/workspace"
	if dir != "" && dir != repoRoot {
		// Agent has an isolated worktree — set working directory to it.
		if rel, relErr := filepath.Rel(repoRoot, dir); relErr == nil && !strings.HasPrefix(rel, "..") {
			workdir = filepath.Join("/workspace", rel)
		}
	}
	return hostRepo, workdir, nil
}

// imageForTool returns the Docker image for a given agent tool name.
func (b *Backend) imageForTool(toolName string) string {
	if toolName == "" {
		return b.cfg.Image
	}
	if b.providerRegistry != nil {
		if p, ok := b.providerRegistry.Get(toolName); ok {
			if cc, ccOk := p.(provider.ContainerCustomizer); ccOk {
				if img := cc.DockerImage(); img != "" {
					return img
				}
			}
		}
	}
	return "mycel-agent-" + toolName + ":latest"
}

// SessionName returns the full session name with prefix.
func (b *Backend) SessionName(name string) string {
	return b.containerName(name)
}

// tmuxTarget returns the tmux session target inside the container.
// Providers like claude --tmux create their own session name (e.g., "workspace_worktree-root"),
// so we discover the first available session rather than assuming a fixed name.
func (b *Backend) tmuxTarget(ctx context.Context, name string) string {
	cn := b.containerName(name)
	//nolint:gosec // trusted
	cmd := exec.CommandContext(ctx, "docker", "exec", cn,
		"tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return name // fallback to agent name
	}
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(sessions) > 0 && sessions[0] != "" {
		return sessions[0]
	}
	return name
}

// HasSession checks if a container exists, is running, AND has a live tmux
// session inside. A container with only zombie processes is treated as dead
// so the caller will respawn it rather than reusing a broken session.
func (b *Backend) HasSession(ctx context.Context, name string) bool {
	cn := b.containerName(name)

	// Check container is running
	//nolint:gosec // trusted
	inspect := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}}", cn)
	out, err := inspect.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return false
	}

	// Also verify at least one tmux session is alive inside the container.
	// Claude --tmux creates sessions with varying names (workspace0, workspace_worktree-*).
	// If all tmux sessions are dead (zombie), treat the container as gone.
	//nolint:gosec // trusted
	tmuxCheck := exec.CommandContext(ctx, "docker", "exec", cn, "tmux", "list-sessions")
	return tmuxCheck.Run() == nil
}

// CreateSession creates a new container session.
func (b *Backend) CreateSession(ctx context.Context, name, dir string) error {
	return b.CreateSessionWithEnv(ctx, name, dir, "bash", nil)
}

// CreateSessionWithCommand creates a container with a command.
func (b *Backend) CreateSessionWithCommand(ctx context.Context, name, dir, command string) error {
	return b.CreateSessionWithEnv(ctx, name, dir, command, nil)
}

// CreateSessionWithEnv creates a fully isolated Docker container for an agent.
//
// Mounts:
//   - agent's repo → /workspace (project code; env BC_WORKSPACE, boot workspace when unset)
//   - .bc/volumes/<agent>/.claude → /home/agent/.claude (persistent Claude state)
//   - bc-shared-tmp → /tmp/bc-shared (shared volume for cross-container file exchange)
//
// Env vars:
//   - From the env map (BC_AGENT_ID, BC_AGENT_ROLE, role secrets via bc env)
//
// Everything else (auth, plugins, MCP, settings) is managed by Claude inside
// the container and persists in the .claude volume across restarts.
func (b *Backend) CreateSessionWithEnv(ctx context.Context, name, dir, command string, env map[string]string) error {
	if !validContainerAgentName.MatchString(name) {
		return fmt.Errorf("invalid agent name %q", name)
	}
	// Validate workspace path — containers without a workspace mount will fail
	// with "--worktree requires a git repository" inside the container.
	if dir == "" {
		return fmt.Errorf("workspace path is required for container %q: empty dir would leave container with no git state", name)
	}

	// Verify dir contains a git repository (regular .git dir or worktree .git file)
	gitPath := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		return fmt.Errorf("workspace %q is not a git repository (no .git found): %w", dir, err)
	}

	// Resolve image once per session — avoids double docker probe
	// (validation below + docker run selection at the end).
	image := b.cfg.Image
	if toolName, ok := env["BC_AGENT_TOOL"]; ok && toolName != "" {
		image = b.imageForTool(toolName)
	}

	// Validate tool/image consistency — catch mismatches like running "gemini"
	// command inside a "bc-agent-claude" image (Exit 127).
	if toolName, ok := env["BC_AGENT_TOOL"]; ok && toolName != "" {
		cmdBin := strings.Fields(command)
		if len(cmdBin) > 0 {
			bin := cmdBin[0]
			// If image is tool-specific (mycel-agent-<X> or legacy bc-agent-<X>)
			// but command binary doesn't match, the binary likely doesn't exist
			// in the image.
			var imageTool string
			switch {
			case strings.HasPrefix(image, "mycel-agent-"):
				imageTool = strings.TrimSuffix(strings.TrimPrefix(image, "mycel-agent-"), ":latest")
			case strings.HasPrefix(image, "bc-agent-"):
				imageTool = strings.TrimSuffix(strings.TrimPrefix(image, "bc-agent-"), ":latest")
			}
			if imageTool != "" {
				if bin != imageTool && bin != "bash" && bin != "sh" {
					// Only warn if the binary name looks like a different tool
					for _, knownTool := range []string{"claude", "gemini", "cursor", "codex"} {
						if bin == knownTool && bin != imageTool {
							return fmt.Errorf("tool/image mismatch: command %q will not be found in image %q (expected %q binary)", bin, image, imageTool)
						}
					}
				}
			}
		}
	}

	cn := b.containerName(name)

	// Remove any stale container with the same name.
	//nolint:gosec // trusted
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", cn).Run() //nolint:errcheck // best-effort

	args := []string{
		"run", "-d", "-t",
		"--name", cn,
		"--label", "bc.managed=true",
		"--label", "bc.workspace=" + b.workspaceHash,
		"--label", "bc.agent=" + name,
	}

	// Resource limits
	if b.cfg.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", b.cfg.CPUs))
	}
	if b.cfg.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", b.cfg.MemoryMB))
	}

	// Network
	if b.cfg.Network != "" {
		args = append(args, "--network", b.cfg.Network)
	}

	// Ensure host.docker.internal resolves on Linux (macOS/Windows get this automatically)
	args = append(args, "--add-host=host.docker.internal:host-gateway")

	// Mount 1: The agent's repo at /workspace.
	// The full repo is mounted so git worktrees resolve naturally
	// (worktree .git files can traverse back to .git/ for objects/refs).
	// Agents bound to a different repo (env BC_WORKSPACE) get that repo
	// mounted — never the boot repo. If the agent has an isolated
	// worktree under the repo, we set -w to that subdirectory.
	hostRepo, containerWorkdir, mountErr := b.resolveRepoMount(dir, env)
	if mountErr != nil {
		return mountErr
	}
	args = append(args, "-v", hostRepo+":/workspace")
	args = append(args, "-w", containerWorkdir)

	// Agent state (Mounts 2–3) always lives in the BOOT workspace's data
	// dir regardless of which repo the agent is bound to, so state-dir
	// host translation keys off the boot host root, not the repo mount.
	hostRoot := b.hostWorkspacePath
	if hostRoot == "" {
		hostRoot = b.workspacePath
	}

	// Mount 2: Persistent Claude state (~/.claude/ dir)
	// Use local (container) path for mkdir, host path for -v mount.
	//
	// Agent state lives at <MycelHome>/agents/<name>/ (flat layout);
	// containerAgentDir keeps pre-migration agents on their older
	// per-workspace dirs. The host path may differ when bcd runs in
	// Docker-in-Docker; honor BC_HOST_BC_HOME (if set) for the
	// host-side mycel home.
	localAgentDir := b.containerAgentDir(name)
	hostAgentDir := b.hostAgentDir(name, hostRoot)
	if err := os.MkdirAll(filepath.Join(localAgentDir, "claude"), 0750); err != nil {
		log.Warn("failed to create agent volume dir", "agent", name, "error", err)
	} else {
		args = append(args, "-v", filepath.Join(hostAgentDir, "claude")+":/home/agent/.claude")
	}

	// Mount 3: ~/.claude.json (app config with oauthAccount — needed for auth persistence)
	localClaudeJSON := filepath.Join(localAgentDir, "claude.json")
	if _, statErr := os.Stat(localClaudeJSON); os.IsNotExist(statErr) {
		_ = os.WriteFile(localClaudeJSON, []byte("{}"), 0600) //nolint:errcheck // best-effort
	}
	// Pre-trust the working directory (container-side path) so Claude
	// Code never hangs at the interactive "trust this folder" prompt.
	if err := SeedClaudeTrust(localClaudeJSON, containerWorkdir); err != nil {
		log.Warn("failed to seed claude trust", "agent", name, "error", err)
	}
	args = append(args, "-v", filepath.Join(hostAgentDir, "claude.json")+":/home/agent/.claude.json")

	// Mount 4: Shared tmp volume for cross-container file exchange (e.g., Playwright screenshots).
	// Uses a named Docker volume so all agent containers and the Playwright container share the same data.
	args = append(args, "-v", "bc-shared-tmp:/tmp/bc-shared")

	// Extra mounts from workspace config (e.g., shared caches, tool binaries).
	// Validate each mount source to prevent arbitrary host filesystem access.
	for _, mount := range b.cfg.ExtraMounts {
		if err := validateMount(mount, b.workspacePath); err != nil {
			return fmt.Errorf("extra mount rejected: %w", err)
		}
		args = append(args, "-v", mount)
	}

	// Pre-seed Claude settings to skip interactive theme selection prompt.
	// Claude Code shows an interactive theme picker on first run when no
	// settings exist, which blocks headless Docker agents indefinitely.
	if err := SeedClaudeSettings(filepath.Join(localAgentDir, "claude")); err != nil {
		log.Warn("failed to seed claude settings", "agent", name, "error", err)
	}

	// Environment variables — only from the env map.
	// The env map contains BC_* identity vars and role secrets resolved
	// from bc env by the agent manager's injectEnv().
	for k, v := range env {
		if !validEnvVarName.MatchString(k) {
			return fmt.Errorf("invalid environment variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", k)
		}
		args = append(args, "-e", k+"="+v)
	}

	// Run the agent command. claude --tmux handles its own tmux session.
	// image was resolved once at the top of this function.
	args = append(args, "--entrypoint", "bash", image, "-c", command)

	log.Debug("creating docker container", "name", cn, "image", image)
	//nolint:gosec // args are constructed from trusted internal values
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create container %s: %w (%s)", cn, err, strings.TrimSpace(string(output)))
	}

	return nil
}

// KillSession stops and removes a container.
func (b *Backend) KillSession(ctx context.Context, name string) error {
	cn := b.containerName(name)
	log.Debug("killing docker container", "name", cn)

	// Cancel any log stream
	b.mu.Lock()
	if cancel, ok := b.logCancels[cn]; ok {
		cancel()
		delete(b.logCancels, cn)
	}
	b.mu.Unlock()

	// Stop container (10s timeout) — do NOT remove it.
	// The container's volume preserves auth, plugins, MCP config, and sessions.
	// bc agent start will restart the stopped container.
	// bc agent delete handles removal.
	//nolint:gosec // trusted
	stopCmd := exec.CommandContext(ctx, "docker", "stop", "-t", "10", cn)
	output, err := stopCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop container %s: %w (%s)", cn, err, strings.TrimSpace(string(output)))
	}

	return nil
}

// RemoveSession stops and removes a container permanently.
// Called by agent delete, not agent stop.
func (b *Backend) RemoveSession(ctx context.Context, name string) error {
	cn := b.containerName(name)

	// Cancel any log stream
	b.mu.Lock()
	if cancel, ok := b.logCancels[cn]; ok {
		cancel()
		delete(b.logCancels, cn)
	}
	b.mu.Unlock()

	//nolint:gosec // trusted
	rmCmd := exec.CommandContext(ctx, "docker", "rm", "-f", cn)
	output, err := rmCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %w (%s)", cn, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// RenameSession renames a container.
func (b *Backend) RenameSession(ctx context.Context, oldName, newName string) error {
	oldCN := b.containerName(oldName)
	newCN := b.containerName(newName)

	//nolint:gosec // trusted
	cmd := exec.CommandContext(ctx, "docker", "rename", oldCN, newCN)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rename container %s to %s: %w (%s)", oldCN, newCN, err, strings.TrimSpace(string(output)))
	}

	// Update log cancel mapping
	b.mu.Lock()
	if cancel, ok := b.logCancels[oldCN]; ok {
		b.logCancels[newCN] = cancel
		delete(b.logCancels, oldCN)
	}
	b.mu.Unlock()

	return nil
}

// SendKeys sends text to the agent's tmux session with Enter.
func (b *Backend) SendKeys(ctx context.Context, name, keys string) error {
	return b.SendKeysWithSubmit(ctx, name, keys, "Enter")
}

// SendKeysWithSubmit sends text to the agent's tmux session inside the container
// via `docker exec ... tmux send-keys`. Stateless — no persistent connections.
// Text is sent literally (-l flag), then the submit key is sent separately.
func (b *Backend) SendKeysWithSubmit(ctx context.Context, name, keys, submitKey string) error {
	cn := b.containerName(name)
	target := b.tmuxTarget(ctx, name)
	keys = strings.TrimRight(keys, "\n")

	// Send text literally (not as key names)
	//nolint:gosec // all args are trusted internal values
	sendCmd := exec.CommandContext(ctx, "docker", "exec", cn,
		"tmux", "send-keys", "-t", target, "-l", "--", keys)
	if output, err := sendCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to send keys to %s: %w (%s)", cn, err, strings.TrimSpace(string(output)))
	}

	// Send submit key separately (as a tmux key name, not literal)
	if submitKey != "" {
		//nolint:gosec // trusted
		keyCmd := exec.CommandContext(ctx, "docker", "exec", cn,
			"tmux", "send-keys", "-t", target, submitKey)
		if output, err := keyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to send submit key to %s: %w (%s)", cn, err, strings.TrimSpace(string(output)))
		}
	}

	return nil
}

// Capture returns recent output from the agent's tmux pane.
func (b *Backend) Capture(ctx context.Context, name string, lines int) (string, error) {
	cn := b.containerName(name)

	startLine := "-100"
	if lines > 0 {
		startLine = fmt.Sprintf("-%d", lines)
	}

	target := b.tmuxTarget(ctx, name)

	//nolint:gosec // all args are trusted
	cmd := exec.CommandContext(ctx, "docker", "exec", cn,
		"tmux", "capture-pane", "-t", target, "-p", "-S", startLine)
	output, err := cmd.Output()
	if err != nil {
		// Fall back to docker logs if tmux capture fails
		//nolint:gosec // trusted
		fallback := exec.CommandContext(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", lines), cn)
		fbOut, fbErr := fallback.Output()
		if fbErr != nil {
			return "", fmt.Errorf("failed to capture output from %s: %w", cn, err)
		}
		return string(fbOut), nil
	}
	return string(output), nil
}

// ListSessions lists RUNNING BC-managed containers for this workspace.
func (b *Backend) ListSessions(ctx context.Context) ([]runtime.Session, error) {
	//nolint:gosec // all args are trusted internal values
	cmd := exec.CommandContext(ctx, "docker", "ps",
		"--filter", "label=bc.managed=true",
		"--filter", "label=bc.workspace="+b.workspaceHash,
		"--filter", "status=running",
		"--format", "{{.Names}}|{{.CreatedAt}}|{{.Status}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var sessions []runtime.Session
	fullPrefix := b.prefix + b.workspaceHash + "-"
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		n := parts[0]
		if !strings.HasPrefix(n, fullPrefix) {
			continue
		}
		matched := fullPrefix
		sessions = append(sessions, runtime.Session{
			Name:    strings.TrimPrefix(n, matched),
			Created: parts[1],
		})
	}

	return sessions, nil
}

// AttachCmd returns an exec.Cmd to attach to the agent's tmux session inside the container.
func (b *Backend) AttachCmd(ctx context.Context, name string) *exec.Cmd {
	cn := b.containerName(name)
	target := b.tmuxTarget(ctx, name)
	//nolint:gosec // trusted
	return exec.CommandContext(ctx, "docker", "exec", "-it", cn, "tmux", "attach", "-t", target)
}

// IsRunning checks if the Docker daemon is running.
func (b *Backend) IsRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info") //nolint:gosec // trusted binary
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// KillServer stops and removes all BC containers for this workspace.
func (b *Backend) KillServer(ctx context.Context) error {
	//nolint:gosec // all args are trusted internal values
	cmd := exec.CommandContext(ctx, "docker", "ps", "-aq",
		"--filter", "label=bc.managed=true",
		"--filter", "label=bc.workspace="+b.workspaceHash)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	ids := strings.Fields(strings.TrimSpace(string(output)))
	if len(ids) == 0 {
		return nil
	}

	// Cancel all log streams
	b.mu.Lock()
	for n, cancel := range b.logCancels {
		cancel()
		delete(b.logCancels, n)
	}
	b.mu.Unlock()

	// Remove all containers
	args := append([]string{"rm", "-f"}, ids...)
	//nolint:gosec // trusted
	rmCmd := exec.CommandContext(ctx, "docker", args...)
	rmOutput, rmErr := rmCmd.CombinedOutput()
	if rmErr != nil {
		return fmt.Errorf("failed to remove containers: %w (%s)", rmErr, strings.TrimSpace(string(rmOutput)))
	}

	return nil
}

// SetEnvironment is a no-op for Docker containers.
func (b *Backend) SetEnvironment(_ context.Context, name, key, value string) error {
	log.Debug("SetEnvironment is a no-op for docker containers", "name", name, "key", key, "value", value)
	return nil
}

// PipePane streams container logs to a file.
func (b *Backend) PipePane(ctx context.Context, name, logPath string) error {
	// logPath is opened for append below — clean it and reject
	// traversal sequences before it reaches the filesystem. Empty means
	// "no piping" and must stay empty (Clean would turn it into ".").
	if logPath != "" {
		logPath = filepath.Clean(logPath)
		if strings.Contains(logPath, "..") {
			return fmt.Errorf("unsafe log path: %s", logPath)
		}
	}
	cn := b.containerName(name)

	b.mu.Lock()
	if cancel, ok := b.logCancels[cn]; ok {
		cancel()
		delete(b.logCancels, cn)
	}
	b.mu.Unlock()

	if logPath == "" {
		return nil
	}

	logCtx, cancel := context.WithCancel(ctx)

	b.mu.Lock()
	b.logCancels[cn] = cancel
	b.mu.Unlock()

	go func() {
		defer cancel()

		// Stream directly to file — no in-memory buffering.
		// The old approach used bytes.Buffer which grew unboundedly.
		//nolint:gosec // logPath is from trusted internal sources
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			log.Warn("failed to open log file", "path", logPath, "error", err)
			return
		}
		defer func() { _ = f.Close() }() //nolint:errcheck // best-effort

		//nolint:gosec // trusted
		cmd := exec.CommandContext(logCtx, "docker", "logs", "-f", cn)
		cmd.Stdout = f
		cmd.Stderr = f

		if err := cmd.Start(); err != nil {
			log.Warn("failed to start log streaming", "container", cn, "error", err)
			return
		}

		_ = cmd.Wait() //nolint:errcheck // expected when context canceled
	}()

	return nil
}
