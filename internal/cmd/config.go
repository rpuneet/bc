package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/workspace"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage repo configuration",
	Long: `Commands for managing repo configuration (preferences.json).

Configuration uses a hierarchical key structure with dot notation:
  workspace.name
  providers.claude.command
  providers.default

Examples:
  mycel config show                        # Show all config
  mycel config get providers.default           # Get a specific value
  mycel config set providers.default 6      # Set a value
  mycel config list                        # List all config keys
  mycel config edit                        # Open config in editor
  mycel config validate                    # Validate config file
  mycel config reset                       # Reset to defaults`,
	RunE: runConfigShow,
}

var configShowCmd = &cobra.Command{
	Use:   "show [key]",
	Short: "Show configuration",
	Long: `Display the current repo configuration.

If a key is specified, shows only that section. Otherwise shows entire config.

Examples:
  mycel config show                  # Show all config
  mycel config show tools            # Show tools section
  mycel config show tools.claude     # Show specific tool config
  mycel config show --json           # Output as JSON`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConfigShow,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a specific configuration value using dot notation.

Examples:
  mycel config get workspace.name
  mycel config get providers.default
  mycel config get providers.claude.command
  mycel config get tools.claude.command`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a specific configuration value using dot notation.

The value type is automatically inferred (string, number, boolean).

Examples:
  mycel config set providers.default 6
  mycel config set providers.default claude
  mycel config set runtime.backend docker
  mycel config set tools.claude.command "claude --force"`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration keys",
	Long: `List all available configuration keys in the repo config.

Examples:
  mycel config list
  mycel config list --json           # Output as JSON array`,
	RunE: runConfigList,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file",
	Long: `Open the repo configuration file in your default editor.

Uses $EDITOR environment variable, falls back to 'nano' if not set.

Examples:
  mycel config edit`,
	RunE: runConfigEdit,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long: `Validate the repo configuration file for errors.

Checks for:
  - Valid TOML syntax
  - Required fields present
  - Valid values and types
  - Tool references exist

Examples:
  mycel config validate`,
	RunE: runConfigValidate,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset the repo configuration to default values.

WARNING: This will overwrite your current config. Back up first if needed.

Examples:
  mycel config reset
  mycel config reset --force         # Skip confirmation`,
	RunE: runConfigReset,
}

// #1160: User-level config (.bcrc) commands

var configUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage user-level configuration (~/.bcrc)",
	Long: `Manage user-level configuration stored in ~/.bcrc.

User configuration provides defaults that apply across all mycel repos:
  - Your nickname for channel messages
  - Default role for new agents
  - Preferred AI tools

Workspace config (preferences.json) takes precedence over user config.

Examples:
  mycel config user init   # Create ~/.bcrc with guided prompts
  mycel config user show   # Show user config
  mycel config user path   # Show user config path`,
	RunE: runConfigUserShow,
}

var configUserInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create user configuration file (~/.bcrc)",
	Long: `Create a new ~/.bcrc file with guided prompts.

Examples:
  mycel config user init          # Interactive setup
  mycel config user init --quick  # Use defaults without prompts`,
	RunE: runConfigUserInit,
}

var configUserShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show user configuration",
	Long: `Display the current user configuration from ~/.bcrc.

Examples:
  mycel config user show`,
	RunE: runConfigUserShow,
}

var configUserPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show user configuration file path",
	Long: `Display the path to the user configuration file.

Examples:
  mycel config user path`,
	RunE: runConfigUserPath,
}

// Flags
var (
	configForce bool
)

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configResetCmd)

	// #1160: User config subcommands
	configCmd.AddCommand(configUserCmd)
	configUserCmd.AddCommand(configUserInitCmd)
	configUserCmd.AddCommand(configUserShowCmd)
	configUserCmd.AddCommand(configUserPathCmd)

	configResetCmd.Flags().BoolVar(&configForce, "force", false, "Skip confirmation prompt")
	configUserInitCmd.Flags().Bool("quick", false, "Use defaults without prompts")

	rootCmd.AddCommand(configCmd)
}

func loadWorkspaceConfig() (*workspace.Config, string, error) {
	ws, err := getRepo()
	if err != nil {
		return nil, "", fmt.Errorf("not in a mycel-adopted repo: %w", err)
	}

	if ws.Config == nil {
		return nil, "", fmt.Errorf("repo is using v1 config format. Run 'mycel up' from your repo to bootstrap v2 config")
	}

	return ws.Config, ws.SettingsFile(), nil
}

// loadConfigViaAPI tries to load workspace config through the daemon API,
// falling back to direct file read when the daemon is not running.
func loadConfigViaAPI(ctx context.Context) (*workspace.Config, error) {
	c, err := newDaemonClient(ctx)
	if err != nil && client.IsDaemonNotRunning(err) {
		// Offline fallback
		cfg, _, loadErr := loadWorkspaceConfig()
		return cfg, loadErr
	}
	if err != nil {
		return nil, err
	}

	raw, err := c.Settings.Get(ctx)
	if err != nil {
		return nil, err
	}

	var cfg workspace.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	return &cfg, nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigViaAPI(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	// If a key is specified, show only that section
	if len(args) > 0 {
		key := args[0]
		value, valErr := getConfigValue(cfg, key)
		if valErr != nil {
			return valErr
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(value)
		}

		return printValue(key, value, 0)
	}

	// Show entire config
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	printConfig(cfg)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigViaAPI(cmd.Context())
	if err != nil {
		return err
	}

	key := args[0]
	value, err := getConfigValue(cfg, key)
	if err != nil {
		return err
	}

	// Print just the value
	fmt.Println(formatValue(value))
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	valueStr := args[1]

	// Try API first for live config updates
	c, clientErr := newDaemonClient(cmd.Context())
	if clientErr == nil {
		// Load current config via API, apply change, push update
		raw, getErr := c.Settings.Get(cmd.Context())
		if getErr != nil {
			return getErr
		}
		var cfg workspace.Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("decode settings: %w", err)
		}
		if err := setConfigValue(&cfg, key, valueStr); err != nil {
			return err
		}
		// Build patch from the modified config
		data, err := json.Marshal(&cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		var patch map[string]any
		if err := json.Unmarshal(data, &patch); err != nil {
			return fmt.Errorf("build patch: %w", err)
		}
		if _, err := c.Settings.Update(cmd.Context(), patch); err != nil {
			return err
		}
		fmt.Printf("Set %s = %s\n", key, valueStr)
		return nil
	}

	if !client.IsDaemonNotRunning(clientErr) {
		return clientErr
	}

	// Offline fallback: direct file modification
	cfg, configPath, err := loadWorkspaceConfig()
	if err != nil {
		return err
	}

	if err := setConfigValue(cfg, key, valueStr); err != nil {
		return err
	}

	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, valueStr)
	return nil
}

func runConfigList(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfigViaAPI(cmd.Context())
	if err != nil {
		return err
	}

	keys := listConfigKeys(cfg, "")

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(keys)
	}

	for _, key := range keys {
		fmt.Println(key)
	}
	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	_, configPath, err := loadWorkspaceConfig()
	if err != nil {
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	// #nosec G204 - editor command is from user's EDITOR env var
	editorCmd := exec.CommandContext(context.Background(), editor, configPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	return editorCmd.Run()
}

func runConfigValidate(cmd *cobra.Command, _ []string) error {
	cfg, configPath, err := loadWorkspaceConfig()
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		fmt.Printf("Config validation failed: %v\n", err)
		fmt.Printf("   File: %s\n", configPath)
		return err
	}

	fmt.Printf("Config is valid\n")
	fmt.Printf("  File: %s\n", configPath)
	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	ws, err := getRepo()
	if err != nil {
		return errNoRepo(err)
	}

	configPath := ws.SettingsFile()

	if !configForce {
		fmt.Printf("⚠️  This will overwrite your config at: %s\n", configPath)
		fmt.Print("Continue? [y/N]: ")

		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			// Handle scan error or empty input
			fmt.Println("Canceled")
			return nil
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Canceled")
			return nil
		}
	}

	// Create default config
	defaultCfg := workspace.DefaultConfig()

	// Save it
	if err := defaultCfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Config reset to defaults\n")
	fmt.Printf("  File: %s\n", configPath)
	return nil
}

// Helper functions

func getConfigValue(cfg *workspace.Config, key string) (any, error) {
	parts := strings.Split(key, ".")
	v := reflect.ValueOf(*cfg)

	for i, part := range parts {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			return nil, fmt.Errorf("invalid key path at '%s'", strings.Join(parts[:i+1], "."))
		}

		// Find field (case-insensitive)
		field := findField(v, part)
		if !field.IsValid() {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}

		v = field
	}

	return v.Interface(), nil
}

func setConfigValue(cfg *workspace.Config, key, valueStr string) error {
	parts := strings.Split(key, ".")
	v := reflect.ValueOf(cfg).Elem()

	// Navigate to the parent of the field we want to set
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			return fmt.Errorf("invalid key path at '%s'", strings.Join(parts[:i+1], "."))
		}

		field := findField(v, part)
		if !field.IsValid() {
			return fmt.Errorf("unknown config key: %s", strings.Join(parts[:i+1], "."))
		}

		v = field
	}

	// Set the final field
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	lastPart := parts[len(parts)-1]
	field := findField(v, lastPart)

	if !field.IsValid() {
		return fmt.Errorf("unknown config key: %s", key)
	}

	if !field.CanSet() {
		return fmt.Errorf("cannot set config key: %s", key)
	}

	// Convert value string to appropriate type
	switch field.Kind() {
	case reflect.String:
		field.SetString(valueStr)
	case reflect.Int, reflect.Int64:
		val, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer value: %s", valueStr)
		}
		field.SetInt(val)
	case reflect.Bool:
		val, err := strconv.ParseBool(valueStr)
		if err != nil {
			return fmt.Errorf("invalid boolean value: %s", valueStr)
		}
		field.SetBool(val)
	case reflect.Float64:
		val, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: %s", valueStr)
		}
		field.SetFloat(val)
	default:
		return fmt.Errorf("unsupported type for key %s: %s", key, field.Kind())
	}

	return nil
}

func findField(v reflect.Value, name string) reflect.Value {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		// Check TOML tag first
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			tagName := strings.Split(jsonTag, ",")[0]
			if strings.EqualFold(tagName, name) {
				return v.Field(i)
			}
		}
		// Fall back to field name
		if strings.EqualFold(field.Name, name) {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

func listConfigKeys(cfg *workspace.Config, prefix string) []string {
	var keys []string
	v := reflect.ValueOf(*cfg)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Get TOML name
		tomlName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			tomlName = strings.Split(jsonTag, ",")[0]
		}
		tomlName = strings.ToLower(tomlName)

		fullKey := tomlName
		if prefix != "" {
			fullKey = prefix + "." + tomlName
		}

		// If it's a struct, recurse
		if fieldValue.Kind() == reflect.Struct {
			subKeys := listStructKeys(fieldValue, fullKey)
			keys = append(keys, subKeys...)
		} else {
			keys = append(keys, fullKey)
		}
	}

	return keys
}

func listStructKeys(v reflect.Value, prefix string) []string {
	var keys []string
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Get TOML name
		tomlName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			tomlName = strings.Split(jsonTag, ",")[0]
		}
		tomlName = strings.ToLower(tomlName)

		fullKey := prefix + "." + tomlName

		// If it's a struct, recurse
		if fieldValue.Kind() == reflect.Struct {
			subKeys := listStructKeys(fieldValue, fullKey)
			keys = append(keys, subKeys...)
		} else if fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() {
			elem := fieldValue.Elem()
			if elem.Kind() == reflect.Struct {
				subKeys := listStructKeys(elem, fullKey)
				keys = append(keys, subKeys...)
			} else {
				keys = append(keys, fullKey)
			}
		} else {
			keys = append(keys, fullKey)
		}
	}

	return keys
}

func printConfig(cfg *workspace.Config) {
	fmt.Println("Repo Configuration")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	fmt.Printf("  version: %d\n", cfg.Version)
	fmt.Println()

	fmt.Println("[user]")
	fmt.Printf("  name: %s\n", cfg.User.Name)
	fmt.Println()

	fmt.Println("[server]")
	fmt.Printf("  host: %s\n", cfg.Server.Host)
	fmt.Printf("  port: %d\n", cfg.Server.Port)
	fmt.Printf("  cors_origin: %s\n", cfg.Server.CORSOrigin)
	fmt.Println()

	fmt.Println("[runtime]")
	fmt.Printf("  default: %s\n", cfg.Runtime.Default)
	fmt.Println()

	fmt.Println("[providers]")
	fmt.Printf("  default: %s\n", cfg.Providers.Default)
	for name, p := range cfg.Providers.Providers {
		fmt.Printf("  %s.command: %s\n", name, p.Command)
	}
	fmt.Println()

	fmt.Println("[cron]")
	fmt.Printf("  poll_interval_seconds: %d\n", cfg.Cron.PollIntervalSeconds)
	fmt.Printf("  job_timeout_seconds: %d\n", cfg.Cron.JobTimeoutSeconds)
	fmt.Println()

	fmt.Println("[ui]")
	fmt.Printf("  theme: %s\n", cfg.UI.Theme)
	fmt.Printf("  mode: %s\n", cfg.UI.Mode)
	fmt.Printf("  default_view: %s\n", cfg.UI.DefaultView)
}

func printValue(key string, value any, indent int) error {
	prefix := strings.Repeat("  ", indent)

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Struct {
		fmt.Printf("%s[%s]\n", prefix, key)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldValue := v.Field(i)

			tomlName := field.Name
			if jsonTag := field.Tag.Get("json"); jsonTag != "" {
				tomlName = strings.Split(jsonTag, ",")[0]
			}

			if err := printValue(tomlName, fieldValue.Interface(), indent+1); err != nil {
				return err
			}
		}
		return nil
	}

	fmt.Printf("%s%s: %s\n", prefix, key, formatValue(value))
	return nil
}

func formatValue(value any) string {
	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Slice:
		var items []string
		for i := 0; i < v.Len(); i++ {
			items = append(items, formatValue(v.Index(i).Interface()))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case reflect.Ptr:
		if v.IsNil() {
			return "<nil>"
		}
		return formatValue(v.Elem().Interface())
	default:
		return fmt.Sprintf("%v", value)
	}
}

// #1160: User config command implementations

func runConfigUserInit(cmd *cobra.Command, _ []string) error {
	quick, _ := cmd.Flags().GetBool("quick")

	path := workspace.UserRCConfigPath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	// Check if already exists
	if workspace.UserRCExists() {
		fmt.Printf("⚠️  User config already exists at %s\n", path)
		fmt.Print("Overwrite? [y/N]: ")

		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			// Handle scan error or empty input
			fmt.Println("Canceled")
			return nil
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Canceled")
			return nil
		}
	}

	var cfg workspace.UserRCConfig

	if quick {
		// Quick mode: use defaults
		cfg = workspace.DefaultUserRCConfig()
	} else {
		// Interactive mode
		var err error
		cfg, err = runConfigUserInitWizard()
		if err != nil {
			return err
		}
	}

	// Save the config
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Printf("✓ Created %s\n", path)
	fmt.Println()
	fmt.Println("Your user config:")
	printUserRCConfig(&cfg)

	return nil
}

func runConfigUserInitWizard() (workspace.UserRCConfig, error) {
	cfg := workspace.DefaultUserRCConfig()

	fmt.Println()
	fmt.Println("mycel - User Configuration Setup")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println()

	// Nickname
	fmt.Printf("Your nickname [%s]: ", "@bc")
	var input string
	if _, err := fmt.Scanln(&input); err == nil && input != "" {
		nickname, err := workspace.NormalizeNickname(input)
		if err != nil {
			fmt.Printf("⚠️  %s, using default\n", err)
		} else {
			cfg.User.Nickname = nickname
		}
	}

	// Default role
	fmt.Printf("Default role for new agents [engineer]: ")
	if _, err := fmt.Scanln(&input); err == nil && input != "" {
		cfg.Defaults.DefaultRole = input
	}

	// Auto-start root
	fmt.Printf("Auto-start root agent on 'mycel up'? [Y/n]: ")
	if _, err := fmt.Scanln(&input); err == nil {
		input = strings.ToLower(strings.TrimSpace(input))
		cfg.Defaults.AutoStartRoot = input != "n" && input != "no"
	}

	// Preferred tools
	fmt.Printf("Preferred tools (comma-separated) [claude-code, gemini]: ")
	if _, err := fmt.Scanln(&input); err == nil && input != "" {
		tools := strings.Split(input, ",")
		for i := range tools {
			tools[i] = strings.TrimSpace(tools[i])
		}
		cfg.Tools.Preferred = tools
	}

	return cfg, nil
}

func runConfigUserShow(_ *cobra.Command, _ []string) error {
	cfg, err := workspace.LoadUserRCConfig()
	if err != nil {
		return err
	}

	if cfg == nil {
		path := workspace.UserRCConfigPath()
		fmt.Printf("No user config found at %s\n", path)
		fmt.Println("Run 'mycel config user init' to create one.")
		return nil
	}

	fmt.Println("# User Config (~/.bcrc)")
	fmt.Println()
	printUserRCConfig(cfg)

	return nil
}

func runConfigUserPath(_ *cobra.Command, _ []string) error {
	path := workspace.UserRCConfigPath()
	exists := workspace.UserRCExists()

	fmt.Printf("User config: %s", path)
	if exists {
		fmt.Println(" (exists)")
	} else {
		fmt.Println(" (not found)")
	}

	return nil
}

func printUserRCConfig(cfg *workspace.UserRCConfig) {
	fmt.Println("[user]")
	fmt.Printf("  nickname = %q\n", cfg.User.Nickname)
	fmt.Println()
	fmt.Println("[defaults]")
	fmt.Printf("  default_role = %q\n", cfg.Defaults.DefaultRole)
	fmt.Printf("  auto_start_root = %v\n", cfg.Defaults.AutoStartRoot)
	fmt.Println()
	fmt.Println("[tools]")
	fmt.Printf("  preferred = %v\n", cfg.Tools.Preferred)
}
