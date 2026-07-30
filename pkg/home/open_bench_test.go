package home

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Benchmark helpers ---

// setupBenchHome bootstraps a home in an isolated MYCEL_HOME
// anchored on a fresh git-repo dir and returns both.
func setupBenchHome(b *testing.B) (*Home, string) {
	b.Helper()
	b.Setenv("MYCEL_HOME", b.TempDir())
	dir := b.TempDir()
	gitInitDir(b, dir)
	h, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	return h, dir
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
	_, dir := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Find benchmarks ---

func BenchmarkFind_Immediate(b *testing.B) {
	_, dir := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		if _, err := Find(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind_OneLevel(b *testing.B) {
	_, dir := setupBenchHome(b)
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
	_, dir := setupBenchHome(b)
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
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		if err := h.Save(); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Directory accessor benchmarks ---

func BenchmarkStateDir(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		_ = h.StateDir()
	}
}

func BenchmarkAgentsDir(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		_ = h.AgentsDir()
	}
}

// --- RoleManager benchmarks ---

func BenchmarkRoleManager_LoadRole(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		// Clear cache to force reload
		h.RoleManager.roles = make(map[string]*Role)
		if _, err := h.RoleManager.LoadRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoleManager_LoadRole_Cached(b *testing.B) {
	h, _ := setupBenchHome(b)
	// Pre-load to cache
	if _, err := h.RoleManager.LoadRole("root"); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := h.RoleManager.LoadRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoleManager_LoadAllRoles(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		// Clear cache to force reload
		h.RoleManager.roles = make(map[string]*Role)
		if _, err := h.RoleManager.LoadAllRoles(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRole(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		if _, err := h.GetRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRolePrompt(b *testing.B) {
	h, _ := setupBenchHome(b)

	b.ResetTimer()
	for range b.N {
		_ = h.GetRolePrompt("root")
	}
}
