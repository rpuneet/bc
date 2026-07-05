package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pkgdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
	pkgmcp "github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/secret"
	pkgtool "github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// SetupAgentFromRole resolves a role via BFS inheritance and writes all
// Claude Code configuration files to the agent's working directory:
//
//   - CLAUDE.md              ← role prompt body
//   - .mcp.json              ← resolved MCP servers with secret injection
//   - .claude/settings.json  ← role settings (hooks, permissions)
//   - .claude/commands/*.md  ← custom slash commands
//   - .claude/skills/*.md    ← reusable skills
//   - .claude/agents/*.md    ← subagent definitions
//   - .claude/rules/*.md     ← topic-specific rules
//   - REVIEW.md              ← code review checklist
//
// SetupAgentFromRoleWithRuntime sets up agent workspace files for the given role,
// runtime backend, and tool provider. Uses ConfigAdapter for provider-specific
// file layout (prompt file, config dir, MCP setup, plugins).
func SetupAgentFromRoleWithRuntime(ctx context.Context, workspacePath, agentName, roleName, targetDir, runtimeBackend string, toolName ...string) error {
	tool := ""
	if len(toolName) > 0 {
		tool = toolName[0]
	}
	return setupAgentFromRole(ctx, workspacePath, agentName, roleName, targetDir, runtimeBackend, tool)
}

// SetupAgentFromRole sets up agent workspace files for the given role.
// Defaults to tmux runtime (all MCP transports available).
func SetupAgentFromRole(ctx context.Context, workspacePath, agentName, roleName, targetDir string) error {
	return setupAgentFromRole(ctx, workspacePath, agentName, roleName, targetDir, "tmux", "")
}

func setupAgentFromRole(ctx context.Context, workspacePath, agentName, roleName, targetDir, runtimeBackend, toolName string) error {
	rm, err := workspace.NewGlobalRoleManager(workspaceStateDir(workspacePath))
	if err != nil {
		log.Warn("failed to open global role store, skipping setup", "role", roleName, "error", err)
		return nil
	}

	resolved, err := rm.ResolveRole(roleName)
	if err != nil {
		log.Warn("failed to resolve role, skipping setup", "role", roleName, "error", err)
		return nil
	}

	// Resolve the ConfigAdapter for this provider.
	adapter := resolveConfigAdapter(toolName)

	secrets := loadSecrets(workspacePath, resolved.Secrets)
	var errs []string

	// Write prompt file (CLAUDE.md, .cursorrules, GEMINI.md, etc.)
	if resolved.Prompt != "" {
		promptFile := adapter.PromptFile()
		if e := writeTextFile(targetDir, promptFile, resolved.Prompt); e != nil {
			errs = append(errs, e.Error())
		}
	}

	// MCP config via adapter (claude mcp add, .mcp.json, .cursor/mcp.json, etc.)
	if e := writeMCPJSON(ctx, workspacePath, agentName, resolved, secrets, targetDir, runtimeBackend); e != nil {
		errs = append(errs, e.Error())
	}

	// Provider config directory settings (e.g., .claude/settings.json)
	configDir := adapter.ConfigDir()
	if configDir != "" && len(resolved.Settings) > 0 {
		if e := mergeSettingsJSON(filepath.Join(targetDir, configDir), resolved.Settings); e != nil {
			errs = append(errs, e.Error())
		}
	}

	// Plugins via adapter
	agentDir := filepath.Join(workspaceStateDir(workspacePath), "agents", agentName)
	if len(resolved.Plugins) > 0 {
		if e := adapter.SetupPlugins(agentDir, resolved.Plugins); e != nil {
			errs = append(errs, e.Error())
		}
	}

	// Rules, commands, skills, agents — written to provider config dir
	if configDir != "" {
		providerDir := filepath.Join(targetDir, configDir)
		type mdFiles struct {
			files     map[string]string
			dir       string
			supported bool
		}
		for _, pair := range []mdFiles{
			{resolved.Commands, "commands", adapter.SupportsCommands()},
			{resolved.Skills, "skills", adapter.SupportsSkills()},
			{resolved.Agents, "agents", adapter.SupportsCommands()}, // agents follow commands support
			{resolved.Rules, "rules", adapter.SupportsRules()},
		} {
			if !pair.supported {
				continue
			}
			subDir := filepath.Join(providerDir, pair.dir)
			if e := cleanStaleMDFiles(subDir, pair.files); e != nil {
				errs = append(errs, e.Error())
			}
			if e := writeMDFiles(subDir, pair.files); e != nil {
				errs = append(errs, e.Error())
			}
		}
	}

	// REVIEW.md
	if resolved.Review != "" {
		if e := writeTextFile(targetDir, "REVIEW.md", resolved.Review); e != nil {
			errs = append(errs, e.Error())
		}
	}

	if len(errs) > 0 {
		log.Warn("some role files failed to write", "agent", agentName, "errors", len(errs))
		return fmt.Errorf("role setup: %s", strings.Join(errs, "; "))
	}

	log.Debug("agent role setup complete", "agent", agentName, "role", roleName)
	return nil
}

// ── MCP config ──────────────────────────────────────────────────────────────

type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Env     map[string]string `json:"env,omitempty"`
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
	Args    []string          `json:"args,omitempty"`
}

var secretRefPattern = regexp.MustCompile(`\$\{secret:([^}]+)\}`)

func writeMCPJSON(ctx context.Context, workspacePath, agentName string, resolved *workspace.ResolvedRole, secrets map[string]string, targetDir, runtimeBackend string) error {
	isDocker := runtimeBackend == "docker"
	cfg := mcpConfig{MCPServers: make(map[string]mcpServerEntry)}

	// Resolve the single global database (cached when the daemon already
	// opened it with full storage config).
	wsDB, wsDriver, dbErr := pkgdb.Global(nil)

	// Try unified tool store first for MCP server configs, fall back to mcp.Store.
	var toolStore *pkgtool.Store
	var toolStoreOpen bool
	if dbErr == nil {
		toolStore = pkgtool.NewStore(wsDB, wsDriver)
		if openErr := toolStore.Open(); openErr == nil {
			toolStoreOpen = true
			defer toolStore.Close() //nolint:errcheck
		}
	}

	var mcpStore *pkgmcp.Store
	mcpErr := dbErr
	if dbErr == nil {
		mcpStore, mcpErr = pkgmcp.NewStore(wsDB, wsDriver)
	}
	if mcpErr != nil && !toolStoreOpen {
		log.Debug("both tool and MCP stores unavailable", "error", mcpErr)
		return writeJSONFile(targetDir, ".mcp.json", cfg)
	}
	if mcpErr == nil {
		defer mcpStore.Close() //nolint:errcheck
	}

	for _, name := range resolved.MCPServers {
		// The bc server is this daemon itself — its address depends on the
		// live bind address and the agent's runtime, never on the store.
		if name == "bc" {
			cfg.MCPServers["bc"] = mcpServerEntry{URL: bcSelfURL(runtimeBackend, agentName), Type: "sse"}
			continue
		}

		// Try unified tool store first
		var transport, command, url string
		var args []string
		var env map[string]string
		var enabled bool
		var found bool

		if toolStoreOpen {
			t, tErr := toolStore.Get(context.Background(), name)
			if tErr == nil && t != nil && t.Type == pkgtool.ToolTypeMCP {
				transport = t.Transport
				command = t.Command
				url = t.URL
				args = t.Args
				env = t.Env
				enabled = t.Enabled
				found = true
			}
		}

		// Fall back to mcp.Store
		if !found && mcpErr == nil {
			def, getErr := mcpStore.Get(name)
			if getErr == nil && def != nil {
				transport = string(def.Transport)
				command = def.Command
				url = def.URL
				args = def.Args
				env = def.Env
				enabled = def.Enabled
				found = true
			}
		}

		if !found || !enabled {
			continue
		}
		// Docker agents can't use stdio-transport MCP servers (no access to
		// host processes). Skip with warning — use tmux runtime for full MCP.
		if isDocker && transport != "sse" {
			log.Warn("skipping stdio MCP server for Docker agent (unreachable)",
				"agent", agentName, "mcp", name,
				"hint", "use tmux runtime for stdio MCP servers")
			continue
		}
		entry := mcpServerEntry{Command: command, Args: args, URL: url}
		if isDocker && entry.URL != "" {
			entry.URL = rewriteDockerURL(entry.URL)
		}
		if entry.URL != "" && strings.Contains(entry.URL, "/_mcp/sse") {
			entry.URL = strings.Replace(entry.URL, "/_mcp/sse", "/_mcp/"+agentName+"/sse", 1)
		}
		if transport == "sse" {
			entry.Type = "sse"
		}
		if len(env) > 0 {
			entry.Env = make(map[string]string, len(env))
			for k, v := range env {
				resolved := resolveSecretValue(v, secrets)
				if resolved != "" {
					entry.Env[k] = resolved
				} else {
					entry.Env[k] = "${" + k + "}"
				}
			}
		}
		cfg.MCPServers[name] = entry
	}

	// Always ensure the bc MCP server is included with agent-scoped URL.
	// This is required for send_message, report_status, and other bc tools.
	if _, hasBc := cfg.MCPServers["bc"]; !hasBc {
		cfg.MCPServers["bc"] = mcpServerEntry{URL: bcSelfURL(runtimeBackend, agentName), Type: "sse"}
	}

	// Prefer claude CLI for MCP setup; fall back to .mcp.json file write.
	if len(cfg.MCPServers) > 0 && setupMCPViaCLI(ctx, targetDir, agentName, cfg.MCPServers) {
		log.Debug("MCP servers configured via claude CLI", "agent", agentName, "count", len(cfg.MCPServers))
		return nil
	}

	// Fallback: write .mcp.json directly (for non-Claude providers or if CLI unavailable)
	return writeJSONFile(targetDir, ".mcp.json", cfg)
}

// rewriteDockerURL rewrites localhost URLs to host.docker.internal so Docker
// containers can reach services on the host. Works on macOS and Windows;
// on Linux, host.docker.internal requires --add-host in docker run.
func rewriteDockerURL(u string) string {
	u = strings.Replace(u, "localhost", "host.docker.internal", 1)
	u = strings.Replace(u, "127.0.0.1", "host.docker.internal", 1)
	return u
}

// bcSelfURL returns this daemon's agent-scoped MCP SSE endpoint for the
// given runtime. A URL stored in mcp_servers can't be right for both
// runtimes at once (tmux needs 127.0.0.1, docker needs
// host.docker.internal) nor track the actual bind port, so the self
// endpoint is always derived from the live daemon address.
func bcSelfURL(runtimeBackend, agentName string) string {
	return daemonAddrForRuntime(runtimeBackend) + "/_mcp/" + agentName + "/sse"
}

// ── Secrets ─────────────────────────────────────────────────────────────────

func loadSecrets(workspacePath string, names []string) map[string]string {
	m := make(map[string]string)
	if len(names) == 0 {
		return m
	}
	ss, err := secret.NewStore(workspacePath, "")
	if err != nil {
		return m
	}
	defer ss.Close() //nolint:errcheck
	for _, n := range names {
		if v, e := ss.GetValue(n); e == nil {
			m[n] = v
		}
	}
	return m
}

func resolveSecretValue(value string, secrets map[string]string) string {
	if !strings.Contains(value, "${secret:") {
		return value
	}
	return secretRefPattern.ReplaceAllStringFunc(value, func(match string) string {
		sub := secretRefPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if v, ok := secrets[sub[1]]; ok {
			return v
		}
		return ""
	})
}

// ── File writers ────────────────────────────────────────────────────────────

// cleanSetupDir normalizes dir and rejects traversal segments so role
// files are always written inside the intended agent directory.
func cleanSetupDir(dir string) (string, error) {
	dir = filepath.Clean(dir)
	if strings.Contains(dir, "..") {
		return "", fmt.Errorf("unsafe directory %q", dir)
	}
	return dir, nil
}

func writeTextFile(dir, name, content string) error {
	dir, err := cleanSetupDir(dir)
	if err != nil {
		return err
	}
	if !filepath.IsLocal(name) {
		return fmt.Errorf("unsafe file name %q", name)
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0600)
}

func writeJSONFile(dir, name string, data any) error {
	dir, err := cleanSetupDir(dir)
	if err != nil {
		return err
	}
	if !filepath.IsLocal(name) {
		return fmt.Errorf("unsafe file name %q", name)
	}
	if err = os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	return os.WriteFile(filepath.Join(dir, name), b, 0600)
}

// cleanStaleMDFiles removes .md files from dir that are not in the new file set.
// Non-.md files and the directory itself are left untouched.
func cleanStaleMDFiles(dir string, newFiles map[string]string) error {
	dir, dirErr := cleanSetupDir(dir)
	if dirErr != nil {
		return dirErr
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // directory doesn't exist yet, nothing to clean
		}
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	// Build a set of expected .md filenames from the new file map.
	expected := make(map[string]struct{}, len(newFiles))
	for name := range newFiles {
		fname := name
		if !strings.HasSuffix(fname, ".md") {
			fname += ".md"
		}
		expected[fname] = struct{}{}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if _, ok := expected[name]; ok {
			continue
		}
		// Stale .md file — remove it.
		if rmErr := os.Remove(filepath.Join(dir, name)); rmErr != nil {
			return fmt.Errorf("remove stale file %s: %w", name, rmErr)
		}
	}
	return nil
}

// mergeSettingsJSON reads existing settings.json from dir, merges role settings
// into it (role settings override non-hook keys, but existing hooks are preserved),
// and writes the merged result back.
func mergeSettingsJSON(dir string, roleSettings map[string]any) error {
	dir, dirErr := cleanSetupDir(dir)
	if dirErr != nil {
		return dirErr
	}
	settingsPath := filepath.Join(dir, "settings.json")

	// Read existing settings (may contain hooks from WriteWorkspaceHookSettings).
	existing := make(map[string]any)
	data, err := os.ReadFile(settingsPath) //nolint:gosec // controlled agent workspace path
	if err == nil {
		_ = json.Unmarshal(data, &existing) //nolint:errcheck // if malformed, start fresh
	}

	// Preserve existing hooks — save them before merging.
	savedHooks, hasHooks := existing["hooks"]

	// Merge role settings into existing (role overrides existing keys).
	for k, v := range roleSettings {
		existing[k] = v
	}

	// Restore hooks if they existed and role settings didn't explicitly set them,
	// or merge them back so hooks from WriteWorkspaceHookSettings are not lost.
	if hasHooks {
		if _, roleHasHooks := roleSettings["hooks"]; !roleHasHooks {
			existing["hooks"] = savedHooks
		}
	}

	return writeJSONFile(dir, "settings.json", existing)
}

func writeMDFiles(dir string, files map[string]string) error {
	if len(files) == 0 {
		return nil
	}
	dir, dirErr := cleanSetupDir(dir)
	if dirErr != nil {
		return dirErr
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	for name, content := range files {
		fname := name
		if !strings.HasSuffix(fname, ".md") {
			fname += ".md"
		}
		if !filepath.IsLocal(fname) {
			return fmt.Errorf("unsafe file name %q", fname)
		}
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(content), 0600); err != nil {
			return fmt.Errorf("write %s: %w", fname, err)
		}
	}
	return nil
}

// validateAgentTools checks that CLI tools listed in the role config are available.
// Returns a list of issues found (empty = all good).
func validateAgentTools(workspacePath, roleName string) []string {
	if roleName == "" {
		return nil
	}

	// Roles live in the single global database, not in the target repo's
	// local state dir — resolve them from the global store so agents created
	// against any repo validate correctly.
	rm, err := workspace.NewGlobalRoleManager(workspaceStateDir(workspacePath))
	if err != nil {
		return []string{fmt.Sprintf("role store unavailable: %v", err)}
	}

	resolved, err := rm.ResolveRole(roleName)
	if err != nil {
		return []string{fmt.Sprintf("cannot resolve role %q: %v", roleName, err)}
	}

	var issues []string

	// Check CLI tools from role config
	for _, toolName := range resolved.CLITools {
		if _, err := execLookPath(toolName); err != nil {
			issues = append(issues, fmt.Sprintf("CLI tool %q not found in PATH", toolName))
		}
	}

	// Check MCP servers from role config — verify definition exists and health check
	var mcpStore *pkgmcp.Store
	wsDB, wsDriver, mcpErr := pkgdb.Global(nil)
	if mcpErr == nil {
		mcpStore, mcpErr = pkgmcp.NewStore(wsDB, wsDriver)
	}
	if mcpErr != nil {
		issues = append(issues, fmt.Sprintf("MCP store unavailable: %v", mcpErr))
		return issues
	}
	defer mcpStore.Close() //nolint:errcheck

	for _, name := range resolved.MCPServers {
		// The bc server is this daemon — health-check the address agents
		// will actually be given (derived per runtime), not the store row.
		// From the daemon's own process the tmux-shaped address applies.
		if name == "bc" {
			if err := checkSSEEndpoint(daemonAddrForRuntime("tmux") + "/_mcp/sse"); err != nil {
				issues = append(issues, fmt.Sprintf("bc MCP endpoint unreachable: %v", err))
			}
			continue
		}

		def, getErr := mcpStore.Get(name)
		if getErr != nil || def == nil {
			issues = append(issues, fmt.Sprintf("MCP server %q not defined in store", name))
			continue
		}

		// Health check based on transport type
		switch string(def.Transport) {
		case "sse":
			if def.URL != "" {
				if err := checkSSEEndpoint(def.URL); err != nil {
					issues = append(issues, fmt.Sprintf("MCP server %q SSE endpoint unreachable: %v", name, err))
				}
			}
		case "stdio":
			if def.Command != "" {
				cmd := strings.Fields(def.Command)[0]
				if _, err := execLookPath(cmd); err != nil {
					issues = append(issues, fmt.Sprintf("MCP server %q command %q not found in PATH", name, cmd))
				}
			}
		}
	}

	return issues
}

// execLookPath is a testable wrapper around exec.LookPath.
var execLookPath = defaultLookPath

func defaultLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// checkSSEEndpoint verifies an MCP SSE endpoint is reachable by sending a
// HEAD request with a short timeout. Returns nil if the endpoint responds.
func checkSSEEndpoint(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	_ = resp.Body.Close()
	return nil
}

// setupMCPViaCLI configures MCP servers using `claude mcp add` commands
// instead of writing .mcp.json directly. This is the preferred approach
// as it uses Claude Code's native configuration system.
//
// Returns true if claude CLI was used successfully, false if caller should
// fall back to file-based .mcp.json.
func setupMCPViaCLI(ctx context.Context, targetDir, agentName string, servers map[string]mcpServerEntry) bool {
	// Check if claude CLI is available
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		log.Debug("claude CLI not available, falling back to .mcp.json", "error", err)
		return false
	}

	// First remove any existing MCP servers to avoid duplicates
	existingCmd := exec.CommandContext(ctx, claudePath, "mcp", "list") //nolint:gosec // trusted claude CLI path
	existingCmd.Dir = targetDir
	if out, err := existingCmd.Output(); err == nil && len(out) > 0 {
		// Parse existing servers and remove them
		for name := range servers {
			rmCmd := exec.CommandContext(ctx, claudePath, "mcp", "remove", name, "--scope", "project") //nolint:gosec // trusted claude CLI path
			rmCmd.Dir = targetDir
			_ = rmCmd.Run() //nolint:errcheck // ignore if not found
		}
	}

	allOK := true
	for name, entry := range servers {
		args := []string{"mcp", "add", "--scope", "project"}

		if entry.Type == "sse" || entry.URL != "" {
			// SSE/HTTP transport
			args = append(args, "--transport", "sse")

			// Add env vars
			for k, v := range entry.Env {
				args = append(args, "-e", k+"="+v)
			}

			args = append(args, name, entry.URL)
		} else if entry.Command != "" {
			// Stdio transport
			for k, v := range entry.Env {
				args = append(args, "-e", k+"="+v)
			}

			// Split command and args
			args = append(args, name, "--")
			cmdParts := strings.Fields(entry.Command)
			args = append(args, cmdParts...)
			args = append(args, entry.Args...)
		} else {
			log.Warn("skipping MCP server with no URL or command", "name", name)
			continue
		}

		cmd := exec.CommandContext(ctx, claudePath, args...) //nolint:gosec // args are from trusted config
		cmd.Dir = targetDir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Warn("claude mcp add failed", "name", name, "error", err, "output", string(out))
			allOK = false
		} else {
			log.Debug("claude mcp add succeeded", "name", name)
		}
	}

	return allOK
}

// resolveConfigAdapter returns the ConfigAdapter for the given tool name.
// Falls back to Claude adapter (default) if tool is empty or unknown.
func resolveConfigAdapter(toolName string) provider.ConfigAdapter {
	if toolName == "" {
		toolName = "claude" // default provider
	}
	p, ok := provider.DefaultRegistry.Get(toolName)
	if !ok {
		return provider.NewGenericAdapter(toolName)
	}
	if adapter := provider.GetConfigAdapter(p); adapter != nil {
		return adapter
	}
	return provider.NewGenericAdapter(toolName)
}
