package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/bc/pkg/template"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl"},
	Short:   "Manage agent templates",
	Long: `Manage agent templates — reusable configurations for spawning agents.

Templates replace Roles as the agent creation primitive. Each template
stores metadata (MCPs, secrets, tool policies) and an optional system prompt.

Examples:
  bc template list                    # List all templates
  bc template show feature-dev        # Show template details
  bc template create my-template      # Scaffold a new template
  bc template delete my-template      # Delete a template`,
	RunE: runTemplateList,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all templates",
	RunE:  runTemplateList,
}

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show template details and system prompt",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

var templateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateCreate,
}

var templateDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateDelete,
}

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateCreateCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}

func templatesDir() (string, error) {
	ws, err := requireWorkspace()
	if err != nil {
		return "", err
	}
	return filepath.Join(ws.StateDir(), "templates"), nil
}

func runTemplateList(_ *cobra.Command, _ []string) error {
	dir, err := templatesDir()
	if err != nil {
		return err
	}

	store := template.NewStore(dir)
	templates, err := store.List()
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates defined. Use 'bc template create <name>' to create one.")
		return nil
	}

	maxNameLen := 4
	maxDescLen := 11
	for _, t := range templates {
		if len(t.Name) > maxNameLen {
			maxNameLen = len(t.Name)
		}
		if len(t.Description) > maxDescLen {
			maxDescLen = len(t.Description)
		}
	}

	const mcpsHeader = "MCPS"
	const colGap = 2 // spaces between columns
	separatorWidth := maxNameLen + colGap + maxDescLen + colGap + len(mcpsHeader)
	fmt.Printf("%-*s  %-*s  %s\n", maxNameLen, "NAME", maxDescLen, "DESCRIPTION", mcpsHeader)
	fmt.Println(strings.Repeat("-", separatorWidth))

	for _, t := range templates {
		mcps := strings.Join(t.MCPs, ", ")
		if mcps == "" {
			mcps = "\u2014"
		}
		fmt.Printf("%-*s  %-*s  %s\n", maxNameLen, t.Name, maxDescLen, t.Description, mcps)
	}

	fmt.Printf("\n%d template(s)\n", len(templates))
	return nil
}

func runTemplateShow(_ *cobra.Command, args []string) error {
	dir, err := templatesDir()
	if err != nil {
		return err
	}

	store := template.NewStore(dir)
	t, prompt, err := store.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Name:        %s\n", t.Name)
	if t.Description != "" {
		fmt.Printf("Description: %s\n", t.Description)
	}
	if len(t.MCPs) > 0 {
		fmt.Printf("MCPs:        %s\n", strings.Join(t.MCPs, ", "))
	}
	if len(t.Secrets) > 0 {
		fmt.Printf("Secrets:     %s\n", strings.Join(t.Secrets, ", "))
	}
	if len(t.Plugins) > 0 {
		fmt.Printf("Plugins:     %s\n", strings.Join(t.Plugins, ", "))
	}
	if t.MaxCostUSD > 0 {
		fmt.Printf("Max Cost:    $%.2f\n", t.MaxCostUSD)
	}
	if t.StuckTimeoutMin > 0 {
		fmt.Printf("Stuck Timeout: %d min\n", t.StuckTimeoutMin)
	}

	fmt.Println()
	if prompt != "" {
		fmt.Println("System Prompt:")
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println(prompt)
	} else {
		fmt.Println("System Prompt: (none)")
	}

	return nil
}

func runTemplateCreate(_ *cobra.Command, args []string) error {
	dir, err := templatesDir()
	if err != nil {
		return err
	}

	name := args[0]
	store := template.NewStore(dir)
	t := template.Template{
		Name:        name,
		Description: "",
		MCPs:        []string{"bc"},
	}

	if err := store.Create(t, ""); err != nil {
		return err
	}

	// Print the actual path returned by the store rather than a hard-coded relative path.
	fmt.Printf("Created template at %s\n", filepath.Join(dir, name+".json"))
	return nil
}

func runTemplateDelete(_ *cobra.Command, args []string) error {
	dir, err := templatesDir()
	if err != nil {
		return err
	}

	name := args[0]
	store := template.NewStore(dir)
	if err := store.Delete(name); err != nil {
		return err
	}

	fmt.Printf("Deleted template %s\n", name)
	return nil
}
