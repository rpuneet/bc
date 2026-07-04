package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// --- Benchmark helpers ---

// newBenchDir creates a temporary directory for benchmarking.
func newBenchDir(b *testing.B) string {
	b.Helper()
	return b.TempDir()
}

// setupV2Workspace creates a v2 workspace for benchmarking.
func setupV2Workspace(b *testing.B) *Workspace {
	b.Helper()
	dir := newBenchDir(b)
	ws, err := Init(dir)
	if err != nil {
		b.Fatal(err)
	}
	return ws
}

// --- Init benchmarks ---

func BenchmarkInit(b *testing.B) {
	baseDir := newBenchDir(b)

	b.ResetTimer()
	for i := range b.N {
		dir := filepath.Join(baseDir, fmt.Sprintf("ws-%d", i))
		if _, err := Init(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInitV2Format(b *testing.B) {
	baseDir := newBenchDir(b)

	b.ResetTimer()
	for i := range b.N {
		dir := filepath.Join(baseDir, fmt.Sprintf("ws-%d", i))
		if _, err := Init(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Load benchmarks ---

func BenchmarkLoad_V1(b *testing.B) {
	dir := newBenchDir(b)
	if _, err := Init(dir); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoad_V2(b *testing.B) {
	dir := newBenchDir(b)
	if _, err := Init(dir); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoad_V2_WithRoles(b *testing.B) {
	dir := newBenchDir(b)
	ws, err := Init(dir)
	if err != nil {
		b.Fatal(err)
	}
	// Add additional roles
	for i := range 5 {
		rolePath := filepath.Join(ws.RolesDir(), fmt.Sprintf("role-%d.md", i))
		roleContent := fmt.Sprintf("---\nname: role-%d\n---\n\n# Role %d\n\nTest role content.", i, i)
		if err := os.WriteFile(rolePath, []byte(roleContent), 0600); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Find benchmarks ---

func BenchmarkFind_Immediate(b *testing.B) {
	dir := newBenchDir(b)
	if _, err := Init(dir); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := Find(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind_OneLevel(b *testing.B) {
	dir := newBenchDir(b)
	if _, err := Init(dir); err != nil {
		b.Fatal(err)
	}
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
	dir := newBenchDir(b)
	if _, err := Init(dir); err != nil {
		b.Fatal(err)
	}
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

// --- Save benchmarks ---

func BenchmarkSave_V1(b *testing.B) {
	dir := newBenchDir(b)
	ws, err := Init(dir)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if err := ws.Save(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSave_V2(b *testing.B) {
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		if err := ws.Save(); err != nil {
			b.Fatal(err)
		}
	}
}

// --- EnsureDirs benchmark ---

func BenchmarkEnsureDirs(b *testing.B) {
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		if err := ws.EnsureDirs(); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Directory accessor benchmarks ---

func BenchmarkStateDir(b *testing.B) {
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.StateDir()
	}
}

func BenchmarkAgentsDir(b *testing.B) {
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.AgentsDir()
	}
}

// --- RoleManager benchmarks ---

func BenchmarkRoleManager_LoadRole(b *testing.B) {
	ws := setupV2Workspace(b)

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
	ws := setupV2Workspace(b)
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
	ws := setupV2Workspace(b)
	// Add additional roles
	for i := range 5 {
		rolePath := filepath.Join(ws.RolesDir(), fmt.Sprintf("role-%d.md", i))
		roleContent := fmt.Sprintf("---\nname: role-%d\nlevel: 1\ncapabilities:\n  - implement_tasks\n---\n\n# Role %d\n\nTest role content.", i, i)
		if err := os.WriteFile(rolePath, []byte(roleContent), 0600); err != nil {
			b.Fatal(err)
		}
	}

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
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		if _, err := ws.GetRole("root"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRolePrompt(b *testing.B) {
	ws := setupV2Workspace(b)

	b.ResetTimer()
	for range b.N {
		_ = ws.GetRolePrompt("root")
	}
}
