package cmd

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// appCmd is the parent command for app/gateway-plugin developer tooling.
var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage app (gateway plugin) integrations",
	Long: `Manage app plugins — the integrations that bridge mycel to external
platforms (Slack, GitHub, webhooks, ...).

Examples:
  mycel app scaffold linear      # Generate a new app plugin skeleton`,
}

// appScaffoldMulti and appScaffoldDir back the scaffold subcommand's flags.
var (
	appScaffoldMulti bool
	appScaffoldDir   string
)

var appScaffoldCmd = &cobra.Command{
	Use:   "scaffold <name>",
	Short: "Generate a new app/gateway plugin skeleton",
	Long: `Generate a new app plugin skeleton under pkg/gateway/<name>/.

Adding an app today = one new plugin package under pkg/gateway/<name>/
plus one import line in pkg/app/builtin/builtin.go. This command
generates the plugin package (a plugin.go implementing app.Plugin and
a <name>.go adapter implementing gateway.NotificationAdapter) so
contributors can fill in the TODOs and wire it up.

Examples:
  mycel app scaffold linear             # pkg/gateway/linear/
  mycel app scaffold telegram --multi   # allow labeled instances`,
	Args: cobra.ExactArgs(1),
	RunE: runAppScaffold,
}

func init() {
	appScaffoldCmd.Flags().BoolVar(&appScaffoldMulti, "multi", false, "allow labeled instances (e.g. telegram:alerts)")
	appScaffoldCmd.Flags().StringVar(&appScaffoldDir, "dir", "pkg/gateway", "parent directory to generate the plugin package under")
	appCmd.AddCommand(appScaffoldCmd)
	rootCmd.AddCommand(appCmd)
}

// appNamePattern enforces lowercase alphanumeric identifiers (optionally
// with underscores), matching the existing pkg/gateway/<name> packages
// and valid Go package names.
var appNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func runAppScaffold(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validateAppName(name); err != nil {
		return err
	}

	pkgDir := filepath.Join(appScaffoldDir, name)
	if _, err := os.Stat(pkgDir); err == nil {
		return fmt.Errorf("refusing to overwrite: %s already exists", pkgDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", pkgDir, err)
	}

	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", pkgDir, err)
	}

	data := appScaffoldData{
		Name:  name,
		Pkg:   name,
		Label: appScaffoldLabel(name),
		Multi: appScaffoldMulti,
	}

	if err := writeGoTemplate(filepath.Join(pkgDir, "plugin.go"), appScaffoldPluginTmpl, data); err != nil {
		return err
	}
	if err := writeGoTemplate(filepath.Join(pkgDir, name+".go"), appScaffoldAdapterTmpl, data); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var msg strings.Builder
	fmt.Fprintf(&msg, "Generated app plugin skeleton at %s\n\n", pkgDir)
	fmt.Fprintf(&msg, "  %s\n", filepath.Join(pkgDir, "plugin.go"))
	fmt.Fprintf(&msg, "  %s\n\n", filepath.Join(pkgDir, name+".go"))
	msg.WriteString("Next steps:\n")
	msg.WriteString("  1. Add this import to pkg/app/builtin/builtin.go:\n")
	fmt.Fprintf(&msg, "       _ \"github.com/rpuneet/mycel/pkg/gateway/%s\"\n", name)
	msg.WriteString("  2. Implement the TODOs in plugin.go and " + name + ".go.\n")
	msg.WriteString("  3. Run: make build-local-mycel\n")

	if _, err := fmt.Fprint(out, msg.String()); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// appScaffoldData is the template context shared by the plugin and
// adapter templates.
type appScaffoldData struct {
	Name  string // descriptor ID / package import path leaf, e.g. "linear"
	Pkg   string // Go package name, same as Name (already validated as an identifier)
	Label string // human-readable label, e.g. "Linear"
	Multi bool
}

// validateAppName rejects names that would not make a valid, non-colliding
// Go package under pkg/gateway.
func validateAppName(name string) error {
	if name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("app name %q must not contain whitespace", name)
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("app name %q must be lowercase", name)
	}
	if !appNamePattern.MatchString(name) {
		return fmt.Errorf("app name %q must start with a letter and contain only lowercase letters, digits, and underscores", name)
	}
	return nil
}

// appScaffoldLabel turns a package-style name into a human-readable
// label, e.g. "google_calendar" -> "Google Calendar".
func appScaffoldLabel(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// writeGoTemplate executes tmpl with data, gofmts the result, and writes
// it to path.
func writeGoTemplate(path string, tmpl *template.Template, data appScaffoldData) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render template for %s: %w", path, err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt %s: %w", path, err)
	}

	if err := os.WriteFile(path, formatted, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
