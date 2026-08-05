package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/template"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl"},
	Short:   "Manage agent templates",
	Long: `Manage agent templates — reusable configurations for spawning agents.

Templates are managed by the mycel daemon (same store as the web UI).

Examples:
  mycel template list                    # List all templates
  mycel template show blank              # Show template details
  mycel template create my-template      # Scaffold a new template
  mycel template import ./my-tmpl.json   # Import a template from a file
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

var templateImportCmd = &cobra.Command{
	Use:   "import <source>",
	Short: "Import a template from a file, URL, or the marketplace catalog",
	Long: `Import a template into the daemon template store.

<source> may be:
  - a path to a local JSON file describing the template
  - an http(s) URL to a template JSON document
  - the name of a template already known to the marketplace catalog

Importing a name that already exists updates it in place when --force is set.`,
	Args: cobra.ExactArgs(1),
	RunE: runTemplateImport,
}

func init() {
	templateImportCmd.Flags().Bool("force", false, "overwrite an existing template with the same name")
	templateImportCmd.Flags().String("name", "", "override the imported template's name")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateCreateCmd)
	templateCmd.AddCommand(templateImportCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	rootCmd.AddCommand(templateCmd)
}

func runTemplateList(cmd *cobra.Command, _ []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}
	templates, err := c.Templates.List(cmd.Context())
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
	const colGap = 2
	separatorWidth := maxNameLen + colGap + maxDescLen + colGap + len(scopeHeader) + colGap + len(mcpsHeader)
	fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxNameLen, "NAME", maxDescLen, "DESCRIPTION", len(scopeHeader), scopeHeader, mcpsHeader)
	fmt.Println(strings.Repeat("-", separatorWidth))

	for _, t := range templates {
		mcps := strings.Join(t.MCPs, ", ")
		if mcps == "" {
			mcps = "\u2014"
		}
		scope := t.Scope
		if scope == "" {
			scope = "global"
		}
		fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxNameLen, t.Name, maxDescLen, t.Description, len(scopeHeader), scope, mcps)
	}

	fmt.Printf("\n%d template(s)\n", len(templates))
	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}
	t, err := c.Templates.Get(cmd.Context(), args[0])
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
	if t.SystemPrompt != "" {
		fmt.Println("System Prompt:")
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println(t.SystemPrompt)
	} else {
		fmt.Println("System Prompt: (none)")
	}

	return nil
}

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	created, err := c.Templates.Create(cmd.Context(), client.TemplateInfo{
		Name: args[0],
		MCPs: []string{},
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created template %s\n", created.Name)
	return nil
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := c.Templates.Delete(cmd.Context(), args[0]); err != nil {
		return err
	}

	fmt.Printf("Deleted template %s\n", args[0])
	return nil
}

func runTemplateImport(cmd *cobra.Command, args []string) error {
	source := args[0]
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	nameOverride, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	t, prompt, err := resolveImportSource(cmd.Context(), c, source)
	if err != nil {
		return err
	}

	if nameOverride != "" {
		t.Name = nameOverride
	}
	if t.Name == "" {
		return fmt.Errorf("imported template has no name; pass --name to set one")
	}

	info := client.TemplateInfo{
		Name:            t.Name,
		Description:     t.Description,
		Label:           t.Label,
		Provider:        t.Provider,
		MCPs:            t.MCPs,
		Secrets:         t.Secrets,
		Plugins:         t.Plugins,
		Composes:        t.Composes,
		SystemPrompt:    prompt,
		MaxCostUSD:      t.MaxCostUSD,
		StuckTimeoutMin: t.StuckTimeoutMin,
	}

	if _, getErr := c.Templates.Get(cmd.Context(), t.Name); getErr == nil {
		if !force {
			return fmt.Errorf("template %q already exists; re-run with --force to update it", t.Name)
		}
		if _, err := c.Templates.Update(cmd.Context(), t.Name, info); err != nil {
			return fmt.Errorf("update template %q: %w", t.Name, err)
		}
		fmt.Printf("Updated template %q from %s\n", t.Name, source)
		return nil
	}

	if _, err := c.Templates.Create(cmd.Context(), info); err != nil {
		return fmt.Errorf("import template %q: %w", t.Name, err)
	}
	fmt.Printf("Imported template %q from %s\n", t.Name, source)
	return nil
}

// resolveImportSource resolves source into a Template + system prompt via
// URL, local file, or an existing daemon template name.
func resolveImportSource(ctx context.Context, c *client.Client, source string) (template.Template, string, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		t, prompt, err := template.FetchImportDoc(ctx, nil, source)
		if err != nil {
			return template.Template{}, "", fmt.Errorf("fetch template from %s: %w", source, err)
		}
		return t, prompt, nil
	}

	if info, statErr := os.Stat(source); statErr == nil && !info.IsDir() {
		data, err := os.ReadFile(source) //nolint:gosec // source is an explicit user-supplied CLI argument
		if err != nil {
			return template.Template{}, "", fmt.Errorf("read %s: %w", source, err)
		}
		t, prompt, err := template.ParseImportDoc(data)
		if err != nil {
			return template.Template{}, "", fmt.Errorf("%s: %w", source, err)
		}
		return t, prompt, nil
	}

	if existing, err := c.Templates.Get(ctx, source); err == nil {
		return template.Template{
			Name:            existing.Name,
			Description:     existing.Description,
			Label:           existing.Label,
			Provider:        existing.Provider,
			MCPs:            existing.MCPs,
			Secrets:         existing.Secrets,
			Plugins:         existing.Plugins,
			Composes:        existing.Composes,
			MaxCostUSD:      existing.MaxCostUSD,
			StuckTimeoutMin: existing.StuckTimeoutMin,
		}, existing.SystemPrompt, nil
	}
	return template.Template{}, "", fmt.Errorf("%q is not a local file, a URL, or a known template in the store", source)
}
