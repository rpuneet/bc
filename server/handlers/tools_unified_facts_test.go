package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/tool"
)

// unifiedToolsFor serves GET /api/tools/unified over a store holding just the
// given tools, and returns the tool named want.
func unifiedToolsFor(t *testing.T, want string, tools ...*tool.Tool) UnifiedTool {
	t.Helper()

	d, err := db.Open(filepath.Join(t.TempDir(), "mycel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	store := tool.NewStore(d, "sqlite")
	if err := store.Open(); err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	for _, tl := range tools {
		if err := store.Add(ctx, tl); err != nil {
			t.Fatalf("add tool %s: %v", tl.Name, err)
		}
	}

	h := NewUnifiedToolsHandler(nil, store, nil, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tools/unified", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []UnifiedTool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, ut := range got {
		if ut.Name == want {
			return ut
		}
	}
	t.Fatalf("tool %q not in response (%d tools)", want, len(got))
	return UnifiedTool{}
}

// The API must report where a binary actually is, separately from what was
// configured. The UI used to label the configured command as the path, which
// only ever echoed back the bare name the user had typed.
func TestUnifiedToolsReportsResolvedPathAndManager(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture relies on a POSIX executable bit and Cellar layout")
	}

	// A fake brew install: a bin/ on PATH symlinked into a Cellar.
	root := t.TempDir()
	cellar := filepath.Join(root, "Cellar", "factfixture", "9.9.9", "bin")
	if err := os.MkdirAll(cellar, 0o750); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cellar, "factfixture")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho 9.9.9\n"), 0o755); err != nil { //nolint:gosec // fixture must be executable
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "factfixture")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	got := unifiedToolsFor(t, "factfixture", &tool.Tool{
		Name:    "factfixture",
		Type:    "cli",
		Command: "factfixture",
		Enabled: true,
	})

	if got.Status != "installed" {
		t.Errorf("status = %q, want installed", got.Status)
	}
	// Command stays what was configured; Path carries the resolution.
	if got.Command != "factfixture" {
		t.Errorf("command = %q, want the configured name", got.Command)
	}
	if got.Path != link {
		t.Errorf("path = %q, want %q", got.Path, link)
	}
	if got.Manager != "brew" {
		t.Errorf("manager = %q, want brew (inferred through the Cellar symlink)", got.Manager)
	}
}

// A tool that isn't installed has no path, and its owner can only be inferred
// from the install command — which is what makes the install hint honest.
func TestUnifiedToolsMissingBinaryHasNoPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	got := unifiedToolsFor(t, "absentfixture", &tool.Tool{
		Name:       "absentfixture",
		Type:       "cli",
		Command:    "absentfixture",
		InstallCmd: "npm install -g absentfixture",
		Enabled:    true,
	})

	if got.Status != "not_installed" {
		t.Errorf("status = %q, want not_installed", got.Status)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want empty", got.Path)
	}
	if got.Manager != "npm" {
		t.Errorf("manager = %q, want npm from the install command", got.Manager)
	}
}
