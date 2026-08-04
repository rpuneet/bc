package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/template"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl"},
	Short:   "Manage agent templates",
	Long: `Manage agent templates — reusable configurations for spawning agents.

Templates are stored in ~/.mycel/templates/ (user-global).

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
	Long: `Import a template into the global template store (~/.mycel/templates/).

<source> may be:
  - a path to a local JSON file describing the template
  - an http(s) URL to a template JSON document
  - the name of a template already known to the marketplace catalog
    (mycel marketplace list --type template)

The JSON document has the same shape as 'mycel template show', plus an
optional "system_prompt" string field carrying the system prompt text:

  {
    "name": "my-template",
    "description": "...",
    "mcps": ["mycel"],
    "system_prompt": "You are..."
  }

Importing a name that already exists in the store updates it in place;
pass --force to allow the overwrite.`,
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

// openTemplateStore returns the single user-global template store at
// ~/.mycel/templates/.
func openTemplateStore() (*template.Store, error) {
	globalDir, err := home.GlobalTemplatesDir()
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
	if _, err := home.EnsureGlobalDir(); err != nil {
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

// runTemplateImport implements `mycel template import <source>`. source can
// be a local JSON file path, an http(s) URL to a template JSON document, or
// the name of a template already listed in the marketplace catalog.
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

	store, err := openTemplateStore()
	if err != nil {
		return err
	}
	if _, dirErr := home.EnsureGlobalDir(); dirErr != nil {
		return dirErr
	}

	t, prompt, err := resolveImportSource(cmd.Context(), store, source)
	if err != nil {
		return err
	}

	if nameOverride != "" {
		t.Name = nameOverride
	}
	if t.Name == "" {
		return fmt.Errorf("imported template has no name; pass --name to set one")
	}
	t.Scope = ""

	if _, _, getErr := store.Get(t.Name); getErr == nil {
		if !force {
			return fmt.Errorf("template %q already exists; re-run with --force to update it", t.Name)
		}
		if err := store.Update(t.Name, t, prompt); err != nil {
			return fmt.Errorf("update template %q: %w", t.Name, err)
		}
		fmt.Printf("Updated template %q from %s\n", t.Name, source)
		return nil
	}

	if err := store.Create(t, prompt, template.ScopeGlobal); err != nil {
		return fmt.Errorf("import template %q: %w", t.Name, err)
	}
	fmt.Printf("Imported template %q from %s\n", t.Name, source)
	return nil
}

// resolveImportSource resolves source into a Template + system prompt. It
// tries, in order: an http(s) URL, a local file path, and finally a lookup
// by name against this store directly. The last case covers "known
// marketplace item name": the marketplace catalog's "mycel" source (see
// pkg/marketplace.Aggregator.fetchMycel) lists exactly this store's own
// templates, so resolving a bare name against the store is equivalent to
// resolving it via the aggregator today, without paying for a live fetch
// across every other registry the aggregator knows about.
func resolveImportSource(ctx context.Context, store *template.Store, source string) (template.Template, string, error) {
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

	if t, prompt, err := store.Get(source); err == nil {
		return *t, prompt, nil
	}
	return template.Template{}, "", fmt.Errorf("%q is not a local file, a URL, or a known template in the store", source)
}
