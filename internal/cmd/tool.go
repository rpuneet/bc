package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/ui"
)

var toolCmd = &cobra.Command{
	Use:     "tool",
	Aliases: []string{"tl"},
	Short:   "Manage AI tool providers",
	Long: `Add, remove, and inspect AI tool providers for agent spawning.

Examples:
  mycel tool list              # Show all tools with status
  mycel tool add myagent       # Add a custom tool
  mycel tool show claude       # Show tool details
  mycel tool setup claude      # Install and configure a tool
  mycel tool status claude     # Check installation status
  mycel tool upgrade claude    # Upgrade an installed tool
  mycel tool delete mytool     # Remove a custom tool
  mycel tool run claude --help # Run a tool directly`,
}

// ── list ──────────────────────────────────────────────────────────────────────

var toolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured tools and their status",
	Long: `Show all registered AI tool providers with installation status, version, and command.

Examples:
  mycel tool list        # Table output
  mycel tool list --json # JSON output for scripting`,
	RunE: runToolList,
}

var toolListJSON bool

// ── show ──────────────────────────────────────────────────────────────────────

var toolShowCmd = &cobra.Command{
	Use:   "show <tool>",
	Short: "Show detailed information about a tool",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolShow,
}

// ── status ────────────────────────────────────────────────────────────────────

var toolStatusCmd = &cobra.Command{
	Use:   "status <tool>",
	Short: "Check installation status of a tool",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolStatus,
}

// ── add ───────────────────────────────────────────────────────────────────────

var toolAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a tool to the workspace",
	Long: `Add a custom tool provider to the workspace.

Examples:
  mycel tool add mytool --command "mytool --yes" --install "pip install mytool"
  mycel tool add mytool --command "mytool" --slash-cmds "/help,/quit"`,
	Args: cobra.ExactArgs(1),
	RunE: runToolAdd,
}

var (
	toolAddCommand   string
	toolAddInstall   string
	toolAddUpgrade   string
	toolAddSlashCmds string
	toolAddJSON      bool
)

// ── setup ─────────────────────────────────────────────────────────────────────

var toolSetupCmd = &cobra.Command{
	Use:   "setup <tool>",
	Short: "Install and configure a tool",
	Long: `Run the tool's install command to set it up.

Steps:
  1. Check if the tool binary is already installed
  2. Run the install command if missing
  3. Verify installation succeeded`,
	Args: cobra.ExactArgs(1),
	RunE: runToolSetup,
}

// ── upgrade ───────────────────────────────────────────────────────────────────

var toolUpgradeCmd = &cobra.Command{
	Use:   "upgrade <tool>",
	Short: "Upgrade an installed tool",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolUpgrade,
}

// ── edit ──────────────────────────────────────────────────────────────────────

var toolEditCmd = &cobra.Command{
	Use:   "edit <tool>",
	Short: "Edit a tool's configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolEdit,
}

var (
	toolEditCommand   string
	toolEditInstall   string
	toolEditUpgrade   string
	toolEditSlashCmds string
	toolEditEnabled   string
)

// ── delete ────────────────────────────────────────────────────────────────────

var toolDeleteCmd = &cobra.Command{
	Use:     "delete <tool>",
	Aliases: []string{"remove", "rm"},
	Short:   "Remove a tool from the workspace",
	Args:    cobra.ExactArgs(1),
	RunE:    runToolDelete,
}

// ── run ───────────────────────────────────────────────────────────────────────

var toolRunCmd = &cobra.Command{
	Use:                "run <tool> [args...]",
	Short:              "Run a tool directly",
	Args:               cobra.MinimumNArgs(1),
	RunE:               runToolRun,
	DisableFlagParsing: true,
}

func init() {
	toolListCmd.Flags().BoolVar(&toolListJSON, "json", false, "Output as JSON")

	toolAddCmd.Flags().StringVar(&toolAddCommand, "command", "", "Command to run the tool (required)")
	toolAddCmd.Flags().StringVar(&toolAddInstall, "install", "", "Command to install the tool")
	toolAddCmd.Flags().StringVar(&toolAddUpgrade, "upgrade", "", "Command to upgrade the tool")
	toolAddCmd.Flags().StringVar(&toolAddSlashCmds, "slash-cmds", "", "Comma-separated list of slash commands (e.g. /help,/quit)")
	toolAddCmd.Flags().BoolVar(&toolAddJSON, "json", false, "Output as JSON")

	toolEditCmd.Flags().StringVar(&toolEditCommand, "command", "", "New run command")
	toolEditCmd.Flags().StringVar(&toolEditInstall, "install", "", "New install command")
	toolEditCmd.Flags().StringVar(&toolEditUpgrade, "upgrade", "", "New upgrade command")
	toolEditCmd.Flags().StringVar(&toolEditSlashCmds, "slash-cmds", "", "New slash commands (comma-separated)")
	toolEditCmd.Flags().StringVar(&toolEditEnabled, "enabled", "", "Enable or disable (true/false)")

	toolCmd.AddCommand(toolListCmd)
	toolCmd.AddCommand(toolShowCmd)
	toolCmd.AddCommand(toolStatusCmd)
	toolCmd.AddCommand(toolAddCmd)
	toolCmd.AddCommand(toolSetupCmd)
	toolCmd.AddCommand(toolUpgradeCmd)
	toolCmd.AddCommand(toolEditCmd)
	toolCmd.AddCommand(toolDeleteCmd)
	toolCmd.AddCommand(toolRunCmd)

	toolShowCmd.ValidArgsFunction = completeToolNames
	toolStatusCmd.ValidArgsFunction = completeToolNames
	toolSetupCmd.ValidArgsFunction = completeToolNames
	toolUpgradeCmd.ValidArgsFunction = completeToolNames
	toolEditCmd.ValidArgsFunction = completeToolNames
	toolDeleteCmd.ValidArgsFunction = completeToolNames

	rootCmd.AddCommand(toolCmd)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func completeToolNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	providers := provider.ListProviders()
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

// openToolStore opens the tool store on the single global database
// (works offline, without bcd).
func openToolStore() (*tool.Store, error) {
	ws, err := getRepo()
	if err != nil {
		return nil, errNoRepo(err)
	}
	wsDB, driver, dbErr := db.Global(ws.Config.DBStorageSettings())
	if dbErr != nil {
		return nil, fmt.Errorf("failed to open global database: %w", dbErr)
	}
	s := tool.NewStore(wsDB, driver)
	if err := s.Open(); err != nil {
		return nil, fmt.Errorf("failed to open tool store: %w", err)
	}
	return s, nil
}

// getToolOrProvider looks up a tool by name from the store first, then falls back
// to the provider registry, returning a synthetic Tool for display purposes.
func getToolOrProvider(ctx context.Context, s *tool.Store, name string) (*tool.Tool, error) {
	if s != nil {
		t, err := s.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if t != nil {
			return t, nil
		}
	}
	// Fallback: synthesize from provider registry
	p, err := provider.GetProvider(name)
	if err != nil {
		return nil, fmt.Errorf("unknown tool %q (use 'mycel tool list' to see available tools)", name)
	}
	return &tool.Tool{
		Name:    p.Name(),
		Command: p.Command(),
		Enabled: true,
	}, nil
}

// clientToolToLocal converts a client ToolInfo to a local tool.Tool.
func clientToolToLocal(t *client.ToolInfo) *tool.Tool {
	return &tool.Tool{
		Name:       t.Name,
		Command:    t.Command,
		InstallCmd: t.InstallCmd,
		UpgradeCmd: t.UpgradeCmd,
		MCPServers: t.MCPServers,
		SlashCmds:  t.SlashCmds,
		Enabled:    t.Enabled,
		Builtin:    t.Builtin,
	}
}

type toolInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
	Command string `json:"command"`
	Path    string `json:"path,omitempty"`
}

func getToolInfo(ctx context.Context, p provider.Provider) toolInfo {
	info := toolInfo{
		Name:    p.Name(),
		Command: p.Command(),
	}

	if p.IsInstalled(ctx) {
		info.Status = "installed"
		info.Version = p.Version(ctx)
		if info.Version == "" {
			info.Version = "-"
		}
		path, err := exec.LookPath(p.Name())
		if err == nil {
			info.Path = path
		}
	} else {
		info.Status = "not found"
		info.Version = "-"
	}

	return info
}

func checkBinaryInstalled(ctx context.Context, name string) (installed bool, version string, path string) {
	p, err := provider.GetProvider(name)
	if err == nil {
		if p.IsInstalled(ctx) {
			return true, p.Version(ctx), ""
		}
		return false, "", ""
	}
	// No provider — check binary by name directly
	lp, lerr := exec.LookPath(name)
	if lerr != nil {
		return false, "", ""
	}
	cmd := exec.CommandContext(ctx, name, "--version") //nolint:gosec // user-provided tool name
	out, _ := cmd.Output()
	ver := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return true, ver, lp
}

// ── list ──────────────────────────────────────────────────────────────────────

func runToolList(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Try bcd API first
	c := getClient()
	tools, apiErr := c.Tools.List(ctx)
	if apiErr == nil {
		if toolListJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(tools)
		}

		tbl := ui.NewTable("TOOL", "STATUS", "ENABLED", "COMMAND")
		for _, t := range tools {
			installed, _, _ := checkBinaryInstalled(ctx, toolBinaryName(clientToolToLocal(t)))
			status := ui.DimText("not found")
			if installed {
				status = ui.GreenText("installed")
			}
			enabled := ui.GreenText("yes")
			if !t.Enabled {
				enabled = ui.DimText("no")
			}
			tbl.AddRow(t.Name, status, enabled, truncateToolCmd(t.Command))
		}
		tbl.Print()
		return nil
	}

	// Fallback: try workspace store, then provider registry
	s, storeErr := openToolStore()
	if storeErr == nil {
		defer s.Close() //nolint:errcheck // best-effort close

		storeTools, err := s.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tools: %w", err)
		}

		if toolListJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(storeTools)
		}

		tbl := ui.NewTable("TOOL", "STATUS", "ENABLED", "COMMAND")
		for _, t := range storeTools {
			installed, _, _ := checkBinaryInstalled(ctx, toolBinaryName(t))
			status := ui.DimText("not found")
			if installed {
				status = ui.GreenText("installed")
			}
			enabled := ui.GreenText("yes")
			if !t.Enabled {
				enabled = ui.DimText("no")
			}
			tbl.AddRow(t.Name, status, enabled, truncateToolCmd(t.Command))
		}
		tbl.Print()
		return nil
	}

	// No workspace — show provider registry
	providers := provider.ListProviders()
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name() < providers[j].Name()
	})

	infos := make([]toolInfo, len(providers))
	for i, p := range providers {
		infos[i] = getToolInfo(ctx, p)
	}

	if toolListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(infos)
	}

	tbl := ui.NewTable("TOOL", "STATUS", "VERSION", "COMMAND")
	for _, info := range infos {
		status := ui.DimText("not found")
		if info.Status == "installed" {
			status = ui.GreenText("installed")
		}
		tbl.AddRow(info.Name, status, info.Version, info.Command)
	}
	tbl.Print()
	return nil
}

// toolBinaryName returns the binary name for a tool (first word of command).
func toolBinaryName(t *tool.Tool) string {
	parts := strings.Fields(t.Command)
	if len(parts) == 0 {
		return t.Name
	}
	return filepath.Base(parts[0])
}

func truncateToolCmd(s string) string {
	const max = 40
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ── show ──────────────────────────────────────────────────────────────────────

func runToolShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Try bcd API first
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		t := clientToolToLocal(apiTool)
		return showToolDetail(ctx, t)
	}

	// Fallback: direct store access
	s, err := openToolStore()
	if err != nil {
		return err
	}
	defer s.Close() //nolint:errcheck // best-effort close

	t, err := getToolOrProvider(ctx, s, name)
	if err != nil {
		return err
	}

	return showToolDetail(ctx, t)
}

func showToolDetail(ctx context.Context, t *tool.Tool) error {
	installed, version, path := checkBinaryInstalled(ctx, toolBinaryName(t))
	statusStr := ui.RedText("not found")
	if installed {
		statusStr = ui.GreenText("installed")
	}
	if version == "" {
		version = "-"
	}
	if path == "" {
		path = "-"
	}

	enabledStr := ui.GreenText("yes")
	if !t.Enabled {
		enabledStr = ui.DimText("no")
	}

	slashCmds := "-"
	if len(t.SlashCmds) > 0 {
		slashCmds = strings.Join(t.SlashCmds, "  ")
	}

	mcpServers := "-"
	if len(t.MCPServers) > 0 {
		mcpServers = strings.Join(t.MCPServers, "  ")
	}

	installCmd := t.InstallCmd
	if installCmd == "" {
		installCmd = "-"
	}
	upgradeCmd := t.UpgradeCmd
	if upgradeCmd == "" {
		upgradeCmd = "-"
	}

	ui.SimpleTable(
		"Tool", t.Name,
		"Status", statusStr,
		"Enabled", enabledStr,
		"Version", version,
		"Command", t.Command,
		"Install", installCmd,
		"Upgrade", upgradeCmd,
		"Slash cmds", slashCmds,
		"MCP servers", mcpServers,
		"Path", path,
	)

	return nil
}

// ── status ────────────────────────────────────────────────────────────────────

func runToolStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Try bcd API first for tool info
	var t *tool.Tool
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		t = clientToolToLocal(apiTool)
	} else {
		// Fallback: direct store access
		s, err := openToolStore()
		if err != nil {
			return err
		}
		defer s.Close() //nolint:errcheck // best-effort close

		t, err = getToolOrProvider(ctx, s, name)
		if err != nil {
			return err
		}
	}

	binName := toolBinaryName(t)
	installed, version, path := checkBinaryInstalled(ctx, binName)

	if installed {
		fmt.Printf("%s %s is installed", ui.GreenText("✓"), name)
		if version != "" {
			fmt.Printf(" (%s)", version)
		}
		if path != "" {
			fmt.Printf(" at %s", path)
		}
		fmt.Println()
	} else {
		fmt.Printf("%s %s is not installed\n", ui.RedText("✗"), name)
		if t.InstallCmd != "" {
			fmt.Printf("  Install: %s\n", ui.DimText(t.InstallCmd))
			fmt.Printf("  Run:     bc tool setup %s\n", name)
		}
	}

	return nil
}

// ── add ───────────────────────────────────────────────────────────────────────

func runToolAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	if toolAddCommand == "" {
		return fmt.Errorf("--command is required (e.g. --command %q)", name+" --yes")
	}

	var slashCmds []string
	if toolAddSlashCmds != "" {
		for _, sc := range strings.Split(toolAddSlashCmds, ",") {
			sc = strings.TrimSpace(sc)
			if sc != "" {
				slashCmds = append(slashCmds, sc)
			}
		}
	}

	// Try bcd API first
	c := getClient()
	apiTool := &client.ToolInfo{
		Name:       name,
		Command:    toolAddCommand,
		InstallCmd: toolAddInstall,
		UpgradeCmd: toolAddUpgrade,
		SlashCmds:  slashCmds,
		Enabled:    true,
	}

	added, apiErr := c.Tools.Update(ctx, apiTool)
	if apiErr == nil {
		if toolAddJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(added)
		}
		fmt.Printf("Added tool %s\n", ui.GreenText(name))
		if apiTool.InstallCmd != "" {
			fmt.Printf("Run 'mycel tool setup %s' to install it.\n", name)
		}
		return nil
	}

	// Fallback: direct store access
	s, err := openToolStore()
	if err != nil {
		return err
	}
	defer s.Close() //nolint:errcheck // best-effort close

	t := &tool.Tool{
		Name:       name,
		Command:    toolAddCommand,
		InstallCmd: toolAddInstall,
		UpgradeCmd: toolAddUpgrade,
		SlashCmds:  slashCmds,
		Enabled:    true,
	}

	if addErr := s.Add(ctx, t); addErr != nil {
		return fmt.Errorf("failed to add tool: %w", addErr)
	}

	addedTool, err := s.Get(ctx, name)
	if err != nil {
		return err
	}

	if toolAddJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(addedTool)
	}

	fmt.Printf("Added tool %s\n", ui.GreenText(name))
	if t.InstallCmd != "" {
		fmt.Printf("Run 'mycel tool setup %s' to install it.\n", name)
	}
	return nil
}

// ── setup ─────────────────────────────────────────────────────────────────────

func runToolSetup(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Get tool info (try API, fallback to store)
	var t *tool.Tool
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		t = clientToolToLocal(apiTool)
	} else {
		s, err := openToolStore()
		if err != nil {
			return err
		}
		defer s.Close() //nolint:errcheck // best-effort close

		t, err = getToolOrProvider(ctx, s, name)
		if err != nil {
			return err
		}
	}

	// Step 1: check if already installed
	binName := toolBinaryName(t)
	if installed, version, _ := checkBinaryInstalled(ctx, binName); installed {
		fmt.Printf("%s %s is already installed (%s)\n", ui.GreenText("✓"), name, version)
		if !t.Enabled {
			// Try to enable via API
			if enErr := c.Tools.Enable(ctx, name); enErr != nil {
				// Fallback to direct store
				s, sErr := openToolStore()
				if sErr == nil {
					defer s.Close() //nolint:errcheck // best-effort close
					if enErr := s.SetEnabled(ctx, name, true); enErr == nil {
						fmt.Printf("Enabled %s in workspace.\n", name)
					}
				}
			} else {
				fmt.Printf("Enabled %s in workspace.\n", name)
			}
		}
		return nil
	}

	// Step 2: run install command
	if t.InstallCmd == "" {
		return fmt.Errorf("no install command configured for %q; add one with: bc tool edit %s --install <cmd>", name, name)
	}

	fmt.Printf("Installing %s...\n", name)
	fmt.Printf("  $ %s\n\n", t.InstallCmd)

	installCmd := exec.CommandContext(ctx, "sh", "-c", t.InstallCmd) //nolint:gosec // user-configured install command
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	// Step 3: verify
	if installed, version, _ := checkBinaryInstalled(ctx, binName); installed {
		fmt.Printf("\n%s %s installed successfully (%s)\n", ui.GreenText("✓"), name, version)
		// Enable via API or fallback
		if enErr := c.Tools.Enable(ctx, name); enErr != nil {
			s, sErr := openToolStore()
			if sErr == nil {
				defer s.Close()                   //nolint:errcheck // best-effort close
				_ = s.SetEnabled(ctx, name, true) //nolint:errcheck // best-effort enable
			}
		}
	} else {
		fmt.Printf("\n%s %s binary not found after install — check the install command\n", ui.RedText("✗"), name)
	}

	return nil
}

// ── upgrade ───────────────────────────────────────────────────────────────────

func runToolUpgrade(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Get tool info (try API, fallback to store)
	var t *tool.Tool
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		t = clientToolToLocal(apiTool)
	} else {
		s, err := openToolStore()
		if err != nil {
			return err
		}
		defer s.Close() //nolint:errcheck // best-effort close

		t, err = getToolOrProvider(ctx, s, name)
		if err != nil {
			return err
		}
	}

	if t.UpgradeCmd == "" {
		if t.InstallCmd != "" {
			fmt.Printf("No upgrade command configured; re-running install command.\n")
			t.UpgradeCmd = t.InstallCmd
		} else {
			return fmt.Errorf("no upgrade command configured for %q", name)
		}
	}

	fmt.Printf("Upgrading %s...\n", name)
	fmt.Printf("  $ %s\n\n", t.UpgradeCmd)

	upgradeCmd := exec.CommandContext(ctx, "sh", "-c", t.UpgradeCmd) //nolint:gosec // user-configured upgrade command
	upgradeCmd.Stdout = os.Stdout
	upgradeCmd.Stderr = os.Stderr
	if err := upgradeCmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	if installed, version, _ := checkBinaryInstalled(ctx, toolBinaryName(t)); installed {
		fmt.Printf("\n%s %s upgraded successfully (%s)\n", ui.GreenText("✓"), name, version)
	}

	return nil
}

// ── edit ──────────────────────────────────────────────────────────────────────

func runToolEdit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Try bcd API first
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		changed := false

		if toolEditCommand != "" {
			apiTool.Command = toolEditCommand
			changed = true
		}
		if toolEditInstall != "" {
			apiTool.InstallCmd = toolEditInstall
			changed = true
		}
		if toolEditUpgrade != "" {
			apiTool.UpgradeCmd = toolEditUpgrade
			changed = true
		}
		if toolEditSlashCmds != "" {
			var cmds []string
			for _, sc := range strings.Split(toolEditSlashCmds, ",") {
				sc = strings.TrimSpace(sc)
				if sc != "" {
					cmds = append(cmds, sc)
				}
			}
			apiTool.SlashCmds = cmds
			changed = true
		}
		if toolEditEnabled != "" {
			switch strings.ToLower(toolEditEnabled) {
			case "true", "yes", "1":
				apiTool.Enabled = true
			case "false", "no", "0":
				apiTool.Enabled = false
			default:
				return fmt.Errorf("--enabled must be true or false")
			}
			changed = true
		}

		if !changed {
			return fmt.Errorf("no changes specified — use flags like --command, --install, --enabled")
		}

		if _, updateErr := c.Tools.Update(ctx, apiTool); updateErr != nil {
			return fmt.Errorf("failed to update tool: %w", updateErr)
		}

		fmt.Printf("Updated tool %s\n", ui.GreenText(name))
		return nil
	}

	// Fallback: direct store access
	s, err := openToolStore()
	if err != nil {
		return err
	}
	defer s.Close() //nolint:errcheck // best-effort close

	t, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("tool %q not found — add it with: bc tool add %s", name, name)
	}

	changed := false

	if toolEditCommand != "" {
		t.Command = toolEditCommand
		changed = true
	}
	if toolEditInstall != "" {
		t.InstallCmd = toolEditInstall
		changed = true
	}
	if toolEditUpgrade != "" {
		t.UpgradeCmd = toolEditUpgrade
		changed = true
	}
	if toolEditSlashCmds != "" {
		var cmds []string
		for _, sc := range strings.Split(toolEditSlashCmds, ",") {
			sc = strings.TrimSpace(sc)
			if sc != "" {
				cmds = append(cmds, sc)
			}
		}
		t.SlashCmds = cmds
		changed = true
	}
	if toolEditEnabled != "" {
		switch strings.ToLower(toolEditEnabled) {
		case "true", "yes", "1":
			t.Enabled = true
		case "false", "no", "0":
			t.Enabled = false
		default:
			return fmt.Errorf("--enabled must be true or false")
		}
		changed = true
	}

	if !changed {
		return fmt.Errorf("no changes specified — use flags like --command, --install, --enabled")
	}

	if err := s.Update(ctx, t); err != nil {
		return fmt.Errorf("failed to update tool: %w", err)
	}

	fmt.Printf("Updated tool %s\n", ui.GreenText(name))
	return nil
}

// ── delete ────────────────────────────────────────────────────────────────────

func runToolDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// Try bcd API first
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		if apiTool.Builtin {
			return fmt.Errorf("cannot delete built-in tool %q — disable it instead: bc tool edit %s --enabled false", name, name)
		}
		if delErr := c.Tools.Delete(ctx, name); delErr != nil {
			return fmt.Errorf("failed to delete tool: %w", delErr)
		}
		fmt.Printf("Deleted tool %s\n", name)
		return nil
	}

	// Fallback: direct store access
	s, err := openToolStore()
	if err != nil {
		return err
	}
	defer s.Close() //nolint:errcheck // best-effort close

	t, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("tool %q not found", name)
	}
	if t.Builtin {
		return fmt.Errorf("cannot delete built-in tool %q — disable it instead: bc tool edit %s --enabled false", name, name)
	}

	if err := s.Delete(ctx, name); err != nil {
		return fmt.Errorf("failed to delete tool: %w", err)
	}

	fmt.Printf("Deleted tool %s\n", name)
	return nil
}

// ── run ───────────────────────────────────────────────────────────────────────

func runToolRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if len(args) == 0 {
		return fmt.Errorf("tool name required")
	}
	name := args[0]
	extraArgs := args[1:]

	// Try bcd API first for tool info
	var cmdStr string
	c := getClient()
	apiTool, apiErr := c.Tools.Get(ctx, name)
	if apiErr == nil && apiTool != nil {
		cmdStr = apiTool.Command
	} else {
		// Fallback: direct store or provider
		s, storeErr := openToolStore()
		var t *tool.Tool
		if storeErr == nil {
			defer s.Close() //nolint:errcheck // best-effort close
			var getErr error
			t, getErr = s.Get(ctx, name)
			if getErr != nil {
				return getErr
			}
		}

		if t != nil {
			cmdStr = t.Command
		} else {
			p, err := provider.GetProvider(name)
			if err != nil {
				return fmt.Errorf("unknown tool %q", name)
			}
			cmdStr = p.Command()
		}
	}

	// Build final command: expand stored command + any extra args
	parts := strings.Fields(cmdStr)
	parts = append(parts, extraArgs...)

	runCmd := exec.CommandContext(ctx, parts[0], parts[1:]...) //nolint:gosec // user-selected tool
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	return runCmd.Run()
}
