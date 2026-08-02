package cmd

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// runScaffold invokes runAppScaffold directly (bypassing the daemon-bound
// rootCmd.Execute path used elsewhere in this package) with the given
// flag values, returning stdout and any error.
func runScaffold(t *testing.T, name, dir string, multi bool) (string, error) {
	t.Helper()

	origDir, origMulti := appScaffoldDir, appScaffoldMulti
	appScaffoldDir, appScaffoldMulti = dir, multi
	t.Cleanup(func() {
		appScaffoldDir, appScaffoldMulti = origDir, origMulti
	})

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runAppScaffold(cmd, []string{name})
	return buf.String(), err
}

func TestAppScaffoldGeneratesValidGo(t *testing.T) {
	dir := t.TempDir()

	out, err := runScaffold(t, "scaffoldfoo", dir, false)
	if err != nil {
		t.Fatalf("runAppScaffold: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}

	pkgDir := filepath.Join(dir, "scaffoldfoo")
	for _, f := range []string{"plugin.go", "scaffoldfoo.go"} {
		path := filepath.Join(pkgDir, f)
		src, readErr := os.ReadFile(path) //nolint:gosec // test-controlled path under t.TempDir()
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		fset := token.NewFileSet()
		if _, parseErr := parser.ParseFile(fset, path, src, parser.AllErrors); parseErr != nil {
			t.Fatalf("generated %s is not valid Go: %v\n%s", f, parseErr, src)
		}
	}
}

func TestAppScaffoldMultiFlag(t *testing.T) {
	dir := t.TempDir()

	if _, err := runScaffold(t, "scaffoldmulti", dir, true); err != nil {
		t.Fatalf("runAppScaffold: %v", err)
	}

	src, err := os.ReadFile(filepath.Join(dir, "scaffoldmulti", "plugin.go")) //nolint:gosec // test-controlled path under t.TempDir()
	if err != nil {
		t.Fatalf("read plugin.go: %v", err)
	}
	if !bytes.Contains(src, []byte("Multi: true")) {
		t.Errorf("expected Multi: true in generated plugin.go, got:\n%s", src)
	}
}

func TestAppScaffoldRefusesExistingPackage(t *testing.T) {
	dir := t.TempDir()

	if _, err := runScaffold(t, "scaffolddup", dir, false); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}

	if _, err := runScaffold(t, "scaffolddup", dir, false); err == nil {
		t.Fatal("expected error when scaffolding an existing package, got nil")
	}
}

func TestAppScaffoldValidatesName(t *testing.T) {
	dir := t.TempDir()

	cases := []string{"BadName", "bad name", "bad-name", "", "123bad"}
	for _, name := range cases {
		if _, err := runScaffold(t, name, dir, false); err == nil {
			t.Errorf("expected error for invalid name %q, got nil", name)
		}
	}
}
