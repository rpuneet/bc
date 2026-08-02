package handlers

import (
	"context"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/tool"
)

// maxVersionLen is the maximum length for version strings in tool responses.
const maxVersionLen = 80

// UnifiedTool represents a tool (MCP or CLI) with its status.
type UnifiedTool struct { //nolint:govet // field order matches JSON/API contract
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Transport string `json:"transport,omitempty"`
	Command   string `json:"command,omitempty"`
	// Path is where the binary actually resolved on PATH, absolute. Kept
	// distinct from Command, which is what was configured: for most tools
	// Command is a bare name ("git") and calling that a path — as the UI
	// once did — tells the user nothing they didn't type.
	Path string `json:"path,omitempty"`
	// Manager is the package manager that owns this tool, inferred rather
	// than configured. See InferToolManager.
	Manager    string `json:"manager,omitempty"`
	URL        string `json:"url,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
	InstallCmd string `json:"install_cmd,omitempty"`
	UpgradeCmd string `json:"upgrade_cmd,omitempty"`
	Required   bool   `json:"required"`
}

// UnifiedToolsHandler handles the merged /api/tools endpoint.
type UnifiedToolsHandler struct {
	mcpStore  *mcp.Store
	toolStore *tool.Store
	agents    *agent.AgentService
	h         *home.Home
}

// NewUnifiedToolsHandler creates a UnifiedToolsHandler.
func NewUnifiedToolsHandler(mcpStore *mcp.Store, toolStore *tool.Store, agents *agent.AgentService, h *home.Home) *UnifiedToolsHandler {
	return &UnifiedToolsHandler{mcpStore: mcpStore, toolStore: toolStore, agents: agents, h: h}
}

// Register mounts unified tools routes.
func (h *UnifiedToolsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/tools/unified", h.list)
	mux.HandleFunc("/api/tools/unified/check", h.checkAll)
}

// truncVersion returns a version string truncated to maxVersionLen.
func truncVersion(ver string) string {
	ver = strings.TrimSpace(ver)
	if len(ver) > maxVersionLen {
		return ver[:maxVersionLen]
	}
	return ver
}

// looseVersionRe matches two-part versions with an optional qualifier, for
// CLIs that never print a third component ("tmux 3.5a", "GNU Make 3.81").
var looseVersionRe = regexp.MustCompile(`\d+\.\d+[A-Za-z0-9.+-]*`)

// cliVersion reduces a `--version` banner to the version itself. Banners are
// chatty and often multi-line ("aws-cli/2.36.14 Python/3.14.6 Darwin/...", GNU
// Make's version followed by its license), so truncating the raw text left the
// Tools table showing a sentence sliced mid-word.
//
// Lines are considered in order and the earliest one carrying any version wins,
// because the version is what a CLI leads with. Preferring a full semver token
// across the whole banner instead would mine later lines for junk: GNU Make
// prints "GNU Make 3.81" and, six lines down, "built for i386-apple-darwin11.3.0"
// — reporting make as 11.3.0.
func cliVersion(out string) string {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if m := semverTokenRe.FindString(line); m != "" {
			return m
		}
		if m := looseVersionRe.FindString(line); m != "" {
			return m
		}
	}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			return truncVersion(line)
		}
	}
	return ""
}

// resolveBinary extracts the binary name from a command string, falling back to name.
func resolveBinary(command, name string) string {
	bin := command
	if i := strings.IndexByte(bin, ' '); i > 0 {
		bin = bin[:i]
	}
	if bin == "" {
		bin = name
	}
	return bin
}

// runVersion runs a version command and returns just the version it printed.
// Output is read combined and parsed even on a non-zero exit, because some
// CLIs report their version on stderr or fail a later check after printing it
// (`docker --version` with the daemon down).
func runVersion(ctx context.Context, versionCmd string) string {
	parts := strings.Fields(versionCmd)
	if len(parts) == 0 {
		return ""
	}
	out, err := exec.CommandContext(ctx, parts[0], parts[1:]...).CombinedOutput() //nolint:gosec // tool names from config
	if err != nil && len(out) == 0 {
		return ""
	}
	return cliVersion(string(out))
}

// describeBinary resolves bin on PATH and reports both where it is and which
// package manager owns it, so neither has to be configured by hand.
//
// Symlinks are resolved for the manager inference only: the path reported is
// the one PATH actually yields (what the user would run), while its target is
// what identifies the owner — a Homebrew binary on PATH is typically a symlink
// and only the Cellar target names brew.
//
// When bin is not on PATH there is no path to report, and the manager can only
// come from the configured install command.
func describeBinary(bin, installCmd string) (path, manager string) {
	p, err := exec.LookPath(bin)
	if err != nil {
		return "", managerFromInstallCmd(installCmd)
	}
	if abs, absErr := filepath.Abs(p); absErr == nil {
		p = abs
	}
	real := p
	if target, symErr := filepath.EvalSymlinks(p); symErr == nil {
		real = target
	}
	return p, InferToolManager(real, installCmd)
}

// resolveToolStatus determines a tool's status based on enabled state, type, and binary availability.
func resolveToolStatus(enabled bool, toolType, command, name string) string {
	if !enabled {
		return "disabled"
	}
	if toolType == "mcp" {
		return "configured"
	}
	bin := resolveBinary(command, name)
	if _, err := exec.LookPath(bin); err != nil {
		return "not_installed"
	}
	return "installed"
}

// list returns all tools (MCP + CLI) with their current status.
func (h *UnifiedToolsHandler) list(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	var tools []UnifiedTool
	seen := make(map[string]bool)

	// MCP servers from store (highest priority)
	if h.mcpStore != nil {
		servers, err := h.mcpStore.List()
		if err == nil {
			for _, s := range servers {
				seen[s.Name] = true
				status := "configured"
				if !s.Enabled {
					status = "disabled"
				}
				tools = append(tools, UnifiedTool{
					Name:      s.Name,
					Type:      "mcp",
					Transport: string(s.Transport),
					Command:   s.Command,
					URL:       s.URL,
					Status:    status,
					Required:  true,
				})
			}
		}
	}

	// CLI tools from role configs
	if h.h != nil && h.h.RoleManager != nil {
		roles, _ := h.h.RoleManager.LoadAllRoles()
		for _, role := range roles {
			for _, t := range role.Metadata.CLITools {
				if seen[t] {
					continue
				}
				seen[t] = true
				ut := UnifiedTool{
					Name:     t,
					Type:     "cli",
					Command:  t,
					Required: true,
				}
				ut.Path, ut.Manager = describeBinary(t, "")
				if ut.Path != "" {
					ut.Status = "installed"
					ut.Version = runVersion(r.Context(), t+" --version")
				} else {
					ut.Status = "not_installed"
				}
				tools = append(tools, ut)
			}
		}
	}

	// Built-in tools from tool store
	if h.toolStore != nil {
		builtins, err := h.toolStore.List(r.Context())
		if err == nil {
			for _, t := range builtins {
				if seen[t.Name] {
					continue
				}
				seen[t.Name] = true
				toolType := "cli"
				if t.Type != "" {
					toolType = t.Type
				}
				status := resolveToolStatus(t.Enabled, toolType, t.Command, t.Name)
				ut := UnifiedTool{
					Name:       t.Name,
					Type:       toolType,
					Command:    t.Command,
					Transport:  t.Transport,
					URL:        t.URL,
					Status:     status,
					InstallCmd: t.InstallCmd,
					UpgradeCmd: t.UpgradeCmd,
				}
				if toolType == "cli" {
					ut.Path, ut.Manager = describeBinary(resolveBinary(t.Command, t.Name), t.InstallCmd)
				}
				if toolType == "cli" && status == "installed" && t.VersionCmd != "" {
					ut.Version = runVersion(r.Context(), t.VersionCmd)
				}
				tools = append(tools, ut)
			}
		}
	}

	if tools == nil {
		tools = []UnifiedTool{}
	}
	writeJSON(w, http.StatusOK, tools)
}

// checkAll runs health checks on all tools and returns results.
func (h *UnifiedToolsHandler) checkAll(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var results []UnifiedTool

	// Check MCP servers
	if h.mcpStore != nil {
		servers, err := h.mcpStore.List()
		if err == nil {
			for _, s := range servers {
				ut := UnifiedTool{
					Name:      s.Name,
					Type:      "mcp",
					Transport: string(s.Transport),
					Status:    "connected",
					Required:  true,
				}
				if s.Transport == "stdio" && s.Command != "" {
					cmd := strings.Fields(s.Command)[0]
					if _, err := exec.LookPath(cmd); err != nil {
						ut.Status = "error"
						ut.Error = "command not found: " + cmd
					}
				}
				results = append(results, ut)
			}
		}
	}

	// Check CLI tools from roles
	seen := make(map[string]bool)
	if h.h != nil && h.h.RoleManager != nil {
		roles, _ := h.h.RoleManager.LoadAllRoles()
		for _, role := range roles {
			for _, t := range role.Metadata.CLITools {
				if seen[t] {
					continue
				}
				seen[t] = true
				ut := UnifiedTool{
					Name:     t,
					Type:     "cli",
					Command:  t,
					Required: true,
				}
				ut.Path, ut.Manager = describeBinary(t, "")
				if ut.Path != "" {
					ut.Status = "installed"
					ut.Version = runVersion(r.Context(), t+" --version")
				} else {
					ut.Status = "not_installed"
					ut.Error = t + " not found in PATH"
				}
				results = append(results, ut)
			}
		}
	}

	// Check CLI tools from tool store (user-added tools)
	if h.toolStore != nil {
		builtins, err := h.toolStore.List(r.Context())
		if err == nil {
			for _, t := range builtins {
				if seen[t.Name] {
					continue
				}
				seen[t.Name] = true
				toolType := "cli"
				if t.Type != "" {
					toolType = t.Type
				}
				if toolType != "cli" {
					continue
				}
				ut := UnifiedTool{
					Name:       t.Name,
					Type:       toolType,
					Command:    t.Command,
					InstallCmd: t.InstallCmd,
					UpgradeCmd: t.UpgradeCmd,
				}
				bin := resolveBinary(t.Command, t.Name)
				ut.Path, ut.Manager = describeBinary(bin, t.InstallCmd)
				switch {
				case !t.Enabled:
					ut.Status = "disabled"
				case ut.Path == "":
					ut.Status = "not_installed"
					ut.Error = bin + " not found in PATH"
				default:
					ut.Status = "installed"
					versionCmd := t.VersionCmd
					if versionCmd == "" {
						versionCmd = bin + " --version"
					}
					ut.Version = runVersion(r.Context(), versionCmd)
				}
				results = append(results, ut)
			}
		}
	}

	if results == nil {
		results = []UnifiedTool{}
	}
	writeJSON(w, http.StatusOK, results)
}
