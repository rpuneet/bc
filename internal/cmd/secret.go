package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/ui"
)

var secretCmd = &cobra.Command{
	Use:     "secret",
	Aliases: []string{"sec"},
	Short:   "Manage encrypted secrets",
	Long: `Manage encrypted secrets for the repo.

Secrets store API keys and tokens used by tools, MCP servers, and agents.
Values are encrypted at rest with AES-256-GCM. The API never exposes
secret values in list/show operations.

Other configs reference secrets with ${secret:NAME} syntax:
  [tools.claude-code]
  env = { ANTHROPIC_API_KEY = "${secret:ANTHROPIC_API_KEY}" }

Examples:
  mycel secret set ANTHROPIC_API_KEY                    # Prompt for value
  mycel secret set ANTHROPIC_API_KEY --value "sk-..."   # Set directly
  mycel secret set GITHUB_TOKEN --from-env GITHUB_TOKEN # Import from env var
  mycel secret list                                     # List names (no values)
  mycel secret show ANTHROPIC_API_KEY                   # Show metadata
  mycel secret show ANTHROPIC_API_KEY --reveal          # Show actual value
  mycel secret delete ANTHROPIC_API_KEY                 # Delete a secret`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Create or update a secret",
	Long: `Create or update an encrypted secret.

The value can be provided via --value, --from-env, or --from-file.
If none are specified, reads from stdin.

Note: --value appears in shell history. For sensitive values, prefer:
  mycel secret set API_KEY --from-env API_KEY
  echo "sk-abc123" | mycel secret set API_KEY

Examples:
  mycel secret set API_KEY --value "sk-abc123"
  mycel secret set API_KEY --from-env API_KEY
  mycel secret set API_KEY --from-file /path/to/key
  echo "sk-abc123" | mycel secret set API_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretSet,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a secret value (prints to stdout)",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets (names and metadata only)",
	RunE:  runSecretList,
}

var secretShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show secret metadata",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretShow,
}

var secretDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm"},
	Short:   "Delete a secret",
	Args:    cobra.ExactArgs(1),
	RunE:    runSecretDelete,
}

// Flags
var (
	secretSetValue     string
	secretSetFromEnv   string
	secretSetFromFile  string
	secretSetDesc      string
	secretSetWorkspace bool
	secretShowReveal   bool
	secretDeleteWS     bool
)

func init() {
	secretSetCmd.Flags().StringVar(&secretSetValue, "value", "", "Secret value (visible in shell history — prefer --from-env or stdin)")
	secretSetCmd.Flags().StringVar(&secretSetFromEnv, "from-env", "", "Import value from environment variable")
	secretSetCmd.Flags().StringVar(&secretSetFromFile, "from-file", "", "Import value from file")
	secretSetCmd.Flags().StringVar(&secretSetDesc, "desc", "", "Secret description")
	secretSetCmd.Flags().BoolVar(&secretSetWorkspace, "workspace", false, "Write as a workspace-scoped override (default: user-global vault)")
	secretShowCmd.Flags().BoolVar(&secretShowReveal, "reveal", false, "Show the actual secret value")
	secretDeleteCmd.Flags().BoolVar(&secretDeleteWS, "workspace", false, "Delete only the workspace override")

	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretShowCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	rootCmd.AddCommand(secretCmd)
}

// openSecretStore returns the user-global vault by default (daemon-less
// --reveal and `secret get` paths). Callers that want the repo-scoped
// override call openRepoSecretStore instead.
func openSecretStore() (*secret.Store, error) {
	passphrase, err := secret.Passphrase()
	if err != nil {
		return nil, fmt.Errorf("resolve secret passphrase: %w", err)
	}
	vaultPath, err := home.GlobalSecretsVault()
	if err != nil {
		return nil, fmt.Errorf("resolve global vault path: %w", err)
	}
	if _, ensureErr := home.EnsureGlobalDir(); ensureErr != nil {
		return nil, fmt.Errorf("ensure global mycel dir: %w", ensureErr)
	}
	return secret.OpenVaultFile(vaultPath, passphrase)
}

// openRepoSecretStore opens the repo-scoped secrets store at
// <repo>/.mycel/secrets.db, used for repo-scoped overrides (the CLI's
// --workspace flag).
func openRepoSecretStore() (*secret.Store, error) {
	h, err := getRepo()
	if err != nil {
		return nil, errNoRepo(err)
	}
	passphrase, err := secret.Passphrase()
	if err != nil {
		return nil, fmt.Errorf("resolve secret passphrase: %w", err)
	}
	return secret.NewStore(h.RootDir, passphrase)
}

// resolveSecretValue extracts the secret value from flags or stdin.
func resolveSecretValue() (string, error) {
	switch {
	case secretSetValue != "":
		return secretSetValue, nil
	case secretSetFromEnv != "":
		value := os.Getenv(secretSetFromEnv)
		if value == "" {
			return "", fmt.Errorf("environment variable %q is empty or not set", secretSetFromEnv)
		}
		return value, nil
	case secretSetFromFile != "":
		data, err := os.ReadFile(secretSetFromFile) //nolint:gosec // user-provided path
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		return strings.TrimRight(string(data), "\n\r"), nil
	default:
		// Read from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return "", fmt.Errorf("no value provided (use --value, --from-env, --from-file, or pipe to stdin)")
		}
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024)) // 1MB max
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n\r"), nil
	}
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("secret name %q contains invalid characters (use letters, numbers, dash, underscore)", name)
	}

	value, err := resolveSecretValue()
	if err != nil {
		return err
	}

	// Repo-scoped writes bypass the daemon and target the
	// repo-scoped store directly — the daemon API is global-only.
	if secretSetWorkspace {
		store, storeErr := openRepoSecretStore()
		if storeErr != nil {
			return storeErr
		}
		defer func() { _ = store.Close() }()
		if setErr := store.Set(name, value, secretSetDesc); setErr != nil {
			return setErr
		}
		fmt.Printf("Secret %q saved (repo scope)\n", name)
		return nil
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if _, err := c.Secrets.Create(cmd.Context(), name, value, secretSetDesc); err != nil {
		return err
	}

	fmt.Printf("Secret %q saved (global scope)\n", name)
	return nil
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("secret name %q contains invalid characters", name)
	}

	// secret get requires direct store access — the API never returns values
	store, err := openSecretStore()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	value, err := store.GetValue(name)
	if err != nil {
		return err
	}

	fmt.Fprint(cmd.OutOrStdout(), value) //nolint:errcheck // writing to stdout
	return nil
}

func runSecretList(cmd *cobra.Command, _ []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	secrets, err := c.Secrets.List(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		response := struct {
			Secrets []client.SecretInfo `json:"secrets"`
		}{Secrets: secrets}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(secrets) == 0 {
		ui.Warning("No secrets configured")
		ui.BlankLine()
		ui.Info("Run 'mycel secret set <name> --value <value>' to add one")
		return nil
	}

	table := ui.NewTable("NAME", "DESCRIPTION", "UPDATED")
	for _, s := range secrets {
		desc := s.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		updated := s.UpdatedAt.Format("2006-01-02 15:04")
		table.AddRow(s.Name, desc, updated)
	}
	table.Print()
	return nil
}

func runSecretShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("secret name %q contains invalid characters", name)
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	meta, err := c.Secrets.Get(cmd.Context(), name)
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(meta)
	}

	ui.SimpleTable(
		"Name", meta.Name,
		"Description", meta.Description,
		"Created", meta.CreatedAt.Format("2006-01-02 15:04:05"),
		"Updated", meta.UpdatedAt.Format("2006-01-02 15:04:05"),
	)

	if secretShowReveal {
		// --reveal requires direct store access — API never returns values
		store, storeErr := openSecretStore()
		if storeErr != nil {
			return fmt.Errorf("reveal secret: %w", storeErr)
		}
		defer func() { _ = store.Close() }()

		value, valErr := store.GetValue(name)
		if valErr != nil {
			return fmt.Errorf("reveal secret: %w", valErr)
		}
		fmt.Printf("\nValue: %s\n", value)
	}

	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !validIdentifier(name) {
		return fmt.Errorf("secret name %q contains invalid characters", name)
	}

	if secretDeleteWS {
		store, storeErr := openRepoSecretStore()
		if storeErr != nil {
			return storeErr
		}
		defer func() { _ = store.Close() }()
		if delErr := store.Delete(name); delErr != nil {
			return delErr
		}
		fmt.Printf("Deleted secret %q (repo scope)\n", name)
		return nil
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Secrets.Delete(cmd.Context(), name); err != nil {
		return err
	}

	fmt.Printf("Deleted secret %q\n", name)
	return nil
}
