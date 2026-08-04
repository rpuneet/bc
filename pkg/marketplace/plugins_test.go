package marketplace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallClaudePluginRecordsAndLists(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	if err := InstallClaudePlugin("pdf", "https://example.com/pdf", "PDF skill"); err != nil {
		t.Fatalf("InstallClaudePlugin: %v", err)
	}
	if err := InstallClaudePlugin("pdf", "https://example.com/pdf2", "PDF skill v2"); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if err := InstallClaudePlugin("xlsx", "", "Excel"); err != nil {
		t.Fatalf("InstallClaudePlugin xlsx: %v", err)
	}

	list, err := ListInstalledPlugins()
	if err != nil {
		t.Fatalf("ListInstalledPlugins: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d plugins, want 2: %+v", len(list), list)
	}
	if list[0].Name != "pdf" || list[0].Source != OfficialClaudePluginSource {
		t.Fatalf("first = %+v", list[0])
	}
	if list[0].URL != "https://example.com/pdf2" {
		t.Fatalf("url not updated on reinstall: %q", list[0].URL)
	}
	if list[1].Name != "xlsx" {
		t.Fatalf("second = %+v", list[1])
	}

	path, err := GlobalPluginsPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("plugins.json missing: %v", err)
	}
	if filepath.Base(path) != "plugins.json" {
		t.Fatalf("path = %q", path)
	}
}

func TestInstallClaudePluginRejectsUnsafeNames(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	for _, name := range []string{"", "../x", "a/b"} {
		if err := InstallClaudePlugin(name, "", ""); err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
}
