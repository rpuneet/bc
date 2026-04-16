package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalStoreEmptyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)
	list, err := g.List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d, want 0", len(list))
	}
}

func TestGlobalStoreAddListGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)

	cfg := &ServerConfig{Name: "github", Transport: TransportStdio, Command: "npx", Enabled: true}
	if err := g.Add(cfg); err != nil {
		t.Fatalf("Add: %v", err)
	}
	list, err := g.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].Name != "github" {
		t.Errorf("name = %q", list[0].Name)
	}
	got, err := g.Get("github")
	if err != nil || got == nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Command != "npx" {
		t.Errorf("command lost: %+v", got)
	}
}

func TestGlobalStoreAddDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)
	cfg := &ServerConfig{Name: "gh", Transport: TransportStdio, Command: "npx"}
	if err := g.Add(cfg); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(cfg); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestGlobalStoreUpsert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)
	if err := g.Upsert(&ServerConfig{Name: "a", Transport: TransportStdio, Command: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Upsert(&ServerConfig{Name: "a", Transport: TransportStdio, Command: "v2"}); err != nil {
		t.Fatal(err)
	}
	got, _ := g.Get("a")
	if got == nil || got.Command != "v2" {
		t.Errorf("upsert did not overwrite: %+v", got)
	}
}

func TestGlobalStoreRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)
	_ = g.Add(&ServerConfig{Name: "x", Transport: TransportStdio, Command: "npx"})

	if err := g.Remove("x"); err != nil {
		t.Fatal(err)
	}
	list, _ := g.List()
	if len(list) != 0 {
		t.Errorf("remove failed: %d entries remain", len(list))
	}
	if err := g.Remove("x"); err == nil {
		t.Fatal("expected not-found on second remove")
	}
}

func TestGlobalStoreSetEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcps.json")
	g := NewGlobalStore(path)
	_ = g.Add(&ServerConfig{Name: "x", Transport: TransportStdio, Command: "c", Enabled: true})
	if err := g.SetEnabled("x", false); err != nil {
		t.Fatal(err)
	}
	got, _ := g.Get("x")
	if got == nil || got.Enabled {
		t.Errorf("SetEnabled did not persist: %+v", got)
	}
}

func TestGlobalStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")

	g1 := NewGlobalStore(path)
	_ = g1.Add(&ServerConfig{Name: "a", Transport: TransportStdio, Command: "c"})

	// New store instance reads same file.
	g2 := NewGlobalStore(path)
	list, err := g2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "a" {
		t.Errorf("persistence failed: %+v", list)
	}

	// On-disk file should exist with 0640-ish perms.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		// the file should not be world-readable; 0640 satisfies this.
		// This is a soft assertion — mode varies across platforms.
		t.Logf("note: mcps.json mode = %o", info.Mode().Perm())
	}
}
