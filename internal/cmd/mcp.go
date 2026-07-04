package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
	bcworkspace "github.com/rpuneet/mycel/pkg/workspace"
	srvmcp "github.com/rpuneet/mycel/server/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP server configurations",
	Long: `Manage Model Context Protocol (MCP) server configurations.

MCP servers provide tools and resources to AI agents. Configurations are
stored per-workspace and can be referenced by roles.

Examples:
  mycel mcp list                                     # List all MCP servers
  mycel mcp add github --command npx --args "@modelcontextprotocol/server-github"
  mycel mcp add sqlite --command npx --args "@modelcontextprotocol/server-sqlite,/path/to/db"
  mycel mcp add remote --transport sse --url "https://api.example.com/mcp"
  mycel mcp add github --command npx --env "GITHUB_TOKEN=tok_123"
  mycel mcp show github                              # Show server details
  mycel mcp remove github                            # Remove a server
  mycel mcp disable github                           # Disable a server
  mycel mcp enable github                            # Re-enable a server`,
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an MCP server configuration",
	Long: `Add a new MCP server configuration to the repo.

For stdio transport (default), specify --command and optionally --args.
For SSE transport, specify --transport sse and --url.

Environment variables can be passed with --env as KEY=VALUE pairs.

Examples:
  mycel mcp add github --command npx --args "@modelcontextprotocol/server-github"
  mycel mcp add db --command npx --args "@modelcontextprotocol/server-sqlite,/tmp/test.db"
  mycel mcp add remote --transport sse --url "https://api.example.com/mcp"
  mycel mcp add github --command npx --env 'GITHUB_TOKEN=${secret:GITHUB_TOKEN}' --env "OWNER=me"

Use ${secret:NAME} references for sensitive values (see 'mycel secret set').`,
	Args: cobra.ExactArgs(1),
	RunE: runMCPAdd,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List MCP server configurations",
	RunE:  runMCPList,
}

var mcpShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show MCP server configuration details",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPShow,
}

var mcpRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove an MCP server configuration",
	Args:    cobra.ExactArgs(1),
	RunE:    runMCPRemove,
}

var mcpEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable an MCP server configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPEnable,
}

var mcpDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable an MCP server configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPDisable,
}

// Issue #1985: bc-as-MCP-server commands
var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start bc as an MCP server",
	Long: `Start bc as an MCP (Model Context Protocol) server.

AI tools like Claude Code and Cursor can connect to bc via MCP to query
workspace state and control agents natively.

Default transport is stdio (newline-delimited JSON on stdin/stdout).
Use --sse to start an HTTP server instead.

Resources exposed:
  bc://workspace/status   Workspace name, path, and config
  bc://agents             All agents with state, role, and worktree info
  bc://channels           All channels with members and message counts
  bc://costs              Workspace and per-agent cost summaries
  bc://roles              Role definitions with capabilities
  bc://tools              Available AI agent tools

Tools available:
  send_message     Send a message to a channel
  send_file        Upload a file to a channel
  list_channels    List all channels
  read_channel     Read recent messages from a channel
  list_agents      List agents in the workspace
  whoami           Show the calling agent's identity

Examples:
  mycel mcp serve                    # stdio — use in Claude Code settings.json
  mycel mcp serve --sse              # SSE on :8811
  mycel mcp serve --sse --addr :9000 # SSE on custom port`,
	RunE: runMCPServe,
}

var mcpRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register bc as an MCP server in agent settings.json",
	Long: `Automatically add bc to the Claude Code MCP server configuration.

This writes (or updates) the mcp.servers entry in the workspace
settings.json so that agents automatically have access to bc MCP tools.

Examples:
  mycel mcp register               # Register with stdio transport
  mycel mcp register --sse         # Register with SSE transport`,
	RunE: runMCPRegister,
}

// Flags for mcp add.
var (
	mcpAddTransport string
	mcpAddCommand   string
	mcpAddArgs      string
	mcpAddURL       string
	mcpAddEnv       []string
)

// Flags for mcp serve / register.
var (
	mcpServeSSE  bool
	mcpServeAddr string
)

func init() {
	mcpAddCmd.Flags().StringVar(&mcpAddTransport, "transport", "stdio", "Transport type (stdio or sse)")
	mcpAddCmd.Flags().StringVar(&mcpAddCommand, "command", "", "Command to run (for stdio transport)")
	mcpAddCmd.Flags().StringVar(&mcpAddArgs, "args", "", "Comma-separated arguments")
	mcpAddCmd.Flags().StringVar(&mcpAddURL, "url", "", "Server URL (for sse transport)")
	mcpAddCmd.Flags().StringArrayVar(&mcpAddEnv, "env", nil, "Environment variables (KEY=VALUE, repeatable)")

	mcpServeCmd.Flags().BoolVar(&mcpServeSSE, "sse", false, "Use SSE transport instead of stdio")
	mcpServeCmd.Flags().StringVar(&mcpServeAddr, "addr", ":8811", "Address to listen on (SSE mode only)")

	mcpRegisterCmd.Flags().BoolVar(&mcpServeSSE, "sse", false, "Register SSE transport endpoint")
	mcpRegisterCmd.Flags().StringVar(&mcpServeAddr, "addr", ":8811", "SSE server address to register")

	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpShowCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpEnableCmd)
	mcpCmd.AddCommand(mcpDisableCmd)
	mcpCmd.AddCommand(mcpServeCmd)
	mcpCmd.AddCommand(mcpRegisterCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("server name %q contains invalid characters (use letters, numbers, dash, underscore)", name)
	}

	// Parse env vars
	env := make(map[string]string)
	for _, e := range mcpAddEnv {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			return fmt.Errorf("invalid env format %q (expected KEY=VALUE)", e)
		}
		env[k] = v
	}

	// Parse args
	var serverArgs []string
	if mcpAddArgs != "" {
		serverArgs = strings.Split(mcpAddArgs, ",")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	cfg := &client.MCPServerConfig{
		Name:      name,
		Transport: mcpAddTransport,
		Command:   mcpAddCommand,
		Args:      serverArgs,
		URL:       mcpAddURL,
		Env:       env,
		Enabled:   true,
	}

	if _, addErr := c.MCP.Add(cmd.Context(), cfg); addErr != nil {
		return fmt.Errorf("add mcp server: %w", addErr)
	}

	fmt.Printf("Added MCP server %q (%s)\n", name, mcpAddTransport)
	return nil
}

func runMCPList(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	configs, listErr := c.MCP.List(cmd.Context())
	if listErr != nil {
		return fmt.Errorf("list mcp servers: %w", listErr)
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		response := struct {
			Servers []*client.MCPServerConfig `json:"servers"`
		}{Servers: configs}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(configs) == 0 {
		ui.Warning("No MCP servers configured")
		ui.BlankLine()
		ui.Info("Run 'mycel mcp add <name> --command <cmd>' to add one")
		return nil
	}

	table := ui.NewTable("NAME", "TRANSPORT", "COMMAND/URL", "ENABLED")
	for _, cfg := range configs {
		target := cfg.Command
		if cfg.Transport == "sse" {
			target = cfg.URL
		}
		enabled := "yes"
		if !cfg.Enabled {
			enabled = "no"
		}
		table.AddRow(cfg.Name, cfg.Transport, target, enabled)
	}
	table.Print()
	return nil
}

func runMCPShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("server name %q contains invalid characters", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	cfg, getErr := c.MCP.Get(cmd.Context(), name)
	if getErr != nil {
		return fmt.Errorf("get mcp server: %w", getErr)
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		// Mask env values in JSON output to avoid leaking secrets
		masked := *cfg
		if len(masked.Env) > 0 {
			masked.Env = make(map[string]string, len(cfg.Env))
			for k := range cfg.Env {
				masked.Env[k] = "***"
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(&masked)
	}

	enabled := "yes"
	if !cfg.Enabled {
		enabled = "no"
	}

	ui.SimpleTable(
		"Name", cfg.Name,
		"Transport", cfg.Transport,
		"Enabled", enabled,
	)

	if cfg.Command != "" {
		ui.SimpleTable("Command", cfg.Command)
	}
	if len(cfg.Args) > 0 {
		ui.SimpleTable("Args", strings.Join(cfg.Args, ", "))
	}
	if cfg.URL != "" {
		ui.SimpleTable("URL", cfg.URL)
	}
	if len(cfg.Env) > 0 {
		pairs := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			pairs = append(pairs, k+"=***")
		}
		ui.SimpleTable("Env", strings.Join(pairs, ", "))
	}

	return nil
}

func runMCPRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("server name %q contains invalid characters", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if removeErr := c.MCP.Remove(cmd.Context(), name); removeErr != nil {
		return fmt.Errorf("remove mcp server: %w", removeErr)
	}

	// Clean stale references from role files
	ws, wsErr := getRepo()
	if wsErr == nil && ws != nil {
		rm := ws.RoleManager
		roles, loadErr := rm.LoadAllRoles()
		if loadErr == nil {
			for roleName, role := range roles {
				for _, srv := range role.Metadata.MCPServers {
					if srv == name {
						if cleanErr := rm.RemoveMCPServer(roleName, name); cleanErr != nil {
							log.Warn("failed to clean MCP ref from role", "role", roleName, "server", name, "error", cleanErr)
						} else {
							fmt.Printf("  Removed %q reference from role %q\n", name, roleName)
						}
						break
					}
				}
			}
		}
	}

	fmt.Printf("Removed MCP server %q\n", name)
	return nil
}

func runMCPEnable(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("server name %q contains invalid characters", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if enableErr := c.MCP.Enable(cmd.Context(), name); enableErr != nil {
		return fmt.Errorf("enable mcp server: %w", enableErr)
	}

	fmt.Printf("Enabled MCP server %q\n", name)
	return nil
}

func runMCPDisable(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("server name %q contains invalid characters", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if disableErr := c.MCP.Disable(cmd.Context(), name); disableErr != nil {
		return fmt.Errorf("disable mcp server: %w", disableErr)
	}

	fmt.Printf("Disabled MCP server %q\n", name)
	return nil
}

// ─── bc mcp serve (#1985) ─────────────────────────────────────────────────────

func runMCPServe(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	ws, err := requireRepo()
	if err != nil {
		return err
	}

	srv, err := srvmcp.New(srvmcp.Config{
		Workspace: ws,
		Version:   version,
	})
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer srv.Close() //nolint:errcheck

	if mcpServeSSE {
		fmt.Fprintf(os.Stderr, "mycel MCP server listening on %s (SSE transport)\n", mcpServeAddr)
		fmt.Fprintf(os.Stderr, "  Connect via: http://%s/sse\n", mcpServeAddr)
		return srv.ServeSSE(ctx, mcpServeAddr)
	}

	// stdio transport — don't write to stdout (it's the protocol stream)
	fmt.Fprintf(os.Stderr, "mycel MCP server ready (stdio transport)\n")
	return srv.ServeStdio(ctx)
}

// ─── bc mcp register (#1985) ──────────────────────────────────────────────────

func runMCPRegister(cmd *cobra.Command, _ []string) error {
	ws, err := requireRepo()
	if err != nil {
		return err
	}

	// The workspace config file is <StateDir>/preferences.json.
	settingsPath := ws.SettingsFile()

	// Load or create settings
	settings := map[string]any{}
	if data, readErr := os.ReadFile(settingsPath); readErr == nil { //nolint:gosec // known path
		if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
			return fmt.Errorf("failed to parse %s: %w", filepath.Base(settingsPath), jsonErr)
		}
	}

	// Build MCP server entry
	var mcpEntry map[string]any
	if mcpServeSSE {
		sseURL := mcpServeAddr
		if !strings.HasPrefix(sseURL, "http://") && !strings.HasPrefix(sseURL, "https://") {
			sseURL = "http://" + sseURL
		}
		if !strings.HasSuffix(sseURL, "/sse") {
			sseURL += "/sse"
		}
		mcpEntry = map[string]any{
			"name":      "bc",
			"transport": "sse",
			"url":       sseURL,
		}
	} else {
		bcPath, lookErr := exec.LookPath("bc")
		if lookErr != nil {
			bcPath = "bc"
		}
		mcpEntry = map[string]any{
			"name":    "bc",
			"command": bcPath,
			"args":    []string{"mcp", "serve"},
		}
	}

	// Update mcp.servers in settings
	mcpSection, _ := settings["mcp"].(map[string]any)
	if mcpSection == nil {
		mcpSection = map[string]any{}
	}

	servers, _ := mcpSection["servers"].([]any)
	updated := false
	for i, s := range servers {
		if m, ok := s.(map[string]any); ok && m["name"] == "bc" {
			servers[i] = mcpEntry
			updated = true
			break
		}
	}
	if !updated {
		servers = append(servers, mcpEntry)
	}

	mcpSection["servers"] = servers
	settings["mcp"] = mcpSection

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Write back to the canonical preferences.json.
	prefsPath := filepath.Join(ws.StateDir(), bcworkspace.PreferencesFileName)
	if writeErr := os.WriteFile(prefsPath, data, 0600); writeErr != nil { //nolint:gosec // 0600 is correct
		return fmt.Errorf("failed to write %s: %w", bcworkspace.PreferencesFileName, writeErr)
	}
	settingsPath = prefsPath

	transport := "stdio"
	if mcpServeSSE {
		transport = "sse (" + mcpServeAddr + ")"
	}
	fmt.Printf("✓ Registered bc MCP server in %s\n", settingsPath)
	fmt.Printf("  Transport: %s\n", transport)
	fmt.Println("\nAgents will automatically have access to bc MCP tools.")

	return nil
}
