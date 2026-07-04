package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
)

// CompleteAgentNames returns a completion function for agent names
func CompleteAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	agents, listErr := c.Agents.List(cmd.Context())
	if listErr != nil {
		log.Debug("completion: failed to list agents", "error", listErr)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteChannelNames returns a completion function for channel names
func CompleteChannelNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	channels, listErr := c.Channels.List(cmd.Context())
	if listErr != nil {
		log.Debug("completion: failed to list channels", "error", listErr)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(channels))
	for _, ch := range channels {
		names = append(names, ch.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteRoleNames returns a completion function for role names.
// Uses the daemon API to get role names (roles are managed by bcd, not local files).
func CompleteRoleNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return []string{"root", "feature-dev", "base"}, cobra.ShellCompDirectiveNoFileComp
	}

	roles, rolesErr := c.Roles.List(cmd.Context())
	if rolesErr != nil {
		log.Debug("completion: failed to list roles", "error", rolesErr)
		return []string{"root", "feature-dev", "base"}, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for mycel.

To load completions:

Bash:
  $ source <(mycel completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ mycel completion bash > /etc/bash_completion.d/mycel
  # macOS:
  $ mycel completion bash > $(brew --prefix)/etc/bash_completion.d/mycel

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ mycel completion zsh > "${fpath[1]}/_mycel"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ mycel completion fish | source

  # To load completions for each session, execute once:
  $ mycel completion fish > ~/.config/fish/completions/mycel.fish

PowerShell:
  PS> mycel completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> mycel completion powershell > mycel.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
