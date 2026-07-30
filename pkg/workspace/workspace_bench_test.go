package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Benchmark helpers ---

// setupBenchWorkspace bootstraps a workspace in an isolated MYCEL_HOME
// anchored on a fresh git-repo dir and returns both.
func setupBenchWorkspace(b *testing.B) (*Workspace, string) {
	b.Helper()
	b.Setenv("MYCEL_HOME", b.TempDir())
	dir := b.TempDir()
	gitInitDir(b, dir)
	ws, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	return ws, dir
}

// --- Open benchmarks ---

func BenchmarkOpen(b *testing.B) {
	b.Setenv("MYCEL_HOME", b.TempDir())
	dir := b.TempDir()
	gitInitDir(b, dir)

	b.ResetTimer()
	for range b.N {
		if _, err := Open(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Load benchmarks ---

func BenchmarkLoad(b *testing.B) {
	_, dir := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Find benchmarks ---

func BenchmarkFind_Immediate(b *testing.B) {
	_, dir := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		if _, err := Find(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind_OneLevel(b *testing.B) {
	_, dir := setupBenchWorkspace(b)
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0750); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Find(subdir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind_ThreeLevels(b *testing.B) {
	_, dir := setupBenchWorkspace(b)
	subdir := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(subdir, 0750); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Find(subdir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Save benchmark ---

func BenchmarkSave(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		if err := ws.Save(); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Directory accessor benchmarks ---

func BenchmarkStateDir(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.StateDir()
	}
}

func BenchmarkAgentsDir(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.AgentsDir()
	}
}

// --- RoleManager benchmarks ---

func BenchmarkRoleManager_LoadRole(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		// Clear cache to force reload
		ws.RoleManager.roles = make(map[string]*Role)
		if _, err := ws.RoleManager.LoadRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoleManager_LoadRole_Cached(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)
	// Pre-load to cache
	if _, err := ws.RoleManager.LoadRole("root"); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := ws.RoleManager.LoadRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoleManager_LoadAllRoles(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		// Clear cache to force reload
		ws.RoleManager.roles = make(map[string]*Role)
		if _, err := ws.RoleManager.LoadAllRoles(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRole(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		if _, err := ws.GetRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRolePrompt(b *testing.B) {
	ws, _ := setupBenchWorkspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.GetRolePrompt("root")
	}
}
