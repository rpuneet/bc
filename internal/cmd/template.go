package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/workspace"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl"},
	Short:   "Manage agent templates",
	Long: `Manage agent templates — reusable configurations for spawning agents.

Templates are stored in ~/.mycel/templates/ (user-global).

Examples:
  mycel template list                    # List all templates
  mycel template show feature-dev        # Show template details
  mycel template create my-template      # Scaffold a new template
  mycel template delete my-template      # Delete a template`,
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

// openTemplateStore returns the single user-global template store at
// ~/.mycel/templates/.
func openTemplateStore() (*template.Store, error) {
	globalDir, err := workspace.GlobalTemplatesDir()
	if err != nil {
		return nil, fmt.Errorf("resolve global templates dir: %w", err)
	}
	return template.NewStore(globalDir), nil
}

func runTemplateList(_ *cobra.Command, _ []string) error {
	store, err := openTemplateStore()
	if err != nil {
		return err
	}
	templates, err := store.List()
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates defined. Use 'mycel template create <name>' to create one.")
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
	const scopeHeader = "SCOPE"
	const colGap = 2 // spaces between columns
	separatorWidth := maxNameLen + colGap + maxDescLen + colGap + len(scopeHeader) + colGap + len(mcpsHeader)
	fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxNameLen, "NAME", maxDescLen, "DESCRIPTION", len(scopeHeader), scopeHeader, mcpsHeader)
	fmt.Println(strings.Repeat("-", separatorWidth))

	for _, t := range templates {
		mcps := strings.Join(t.MCPs, ", ")
		if mcps == "" {
			mcps = "\u2014"
		}
		scope := string(t.Scope)
		if scope == "" {
			scope = "global"
		}
		fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxNameLen, t.Name, maxDescLen, t.Description, len(scopeHeader), scope, mcps)
	}

	fmt.Printf("\n%d template(s)\n", len(templates))
	return nil
}

func runTemplateShow(_ *cobra.Command, args []string) error {
	store, err := openTemplateStore()
	if err != nil {
		return err
	}
	t, prompt, err := store.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Name:        %s\n", t.Name)
	if t.Scope != "" {
		fmt.Printf("Scope:       %s\n", t.Scope)
	}
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
	store, err := openTemplateStore()
	if err != nil {
		return err
	}

	name := args[0]
	if _, err := workspace.EnsureGlobalDir(); err != nil {
		return err
	}

	t := template.Template{
		Name:        name,
		Description: "",
		MCPs:        []string{},
	}
	if err := store.Create(t, "", template.ScopeGlobal); err != nil {
		return err
	}

	// Print the actual path so the user knows where the file landed.
	fmt.Printf("Created template at %s\n", filepath.Join(store.GlobalDir(), name+".json"))
	return nil
}

func runTemplateDelete(_ *cobra.Command, args []string) error {
	store, err := openTemplateStore()
	if err != nil {
		return err
	}

	name := args[0]
	if err := store.Delete(name, template.ScopeGlobal); err != nil {
		return err
	}

	fmt.Printf("Deleted template %s\n", name)
	return nil
}
