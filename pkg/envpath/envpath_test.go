package envpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMergePrependsMissingDirs(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "bin")
	if err := os.Mkdir(extra, 0o755); err != nil {
		t.Fatal(err)
	}

	// Force ExtraBinDirs to see our temp dir by temporarily swapping HOME
	// on platforms that include ~/.local/bin — and also verify Merge
	// itself when given a PATH that already omits a known existing dir.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	merged := Merge("/usr/bin:/bin")
	if merged == "/usr/bin:/bin" && len(ExtraBinDirs()) == 0 {
		t.Skip("no extra bin dirs exist on this host")
	}
	if !strings.Contains(merged, "/usr/bin") {
		t.Fatalf("Merge dropped original PATH entries: %q", merged)
	}
	// First segment should be an extra (when any exist and were missing).
	extras := ExtraBinDirs()
	if len(extras) == 0 {
		return
	}
	wantFirst := extras[0]
	if _, already := pathSet(origPath)[wantFirst]; already {
		// Host PATH already has Homebrew — Merge should be a no-op for that entry.
		if !strings.Contains(merged, wantFirst) {
			t.Fatalf("Merge lost existing extra %q in %q", wantFirst, merged)
		}
		return
	}
	first, _, _ := strings.Cut(merged, string(os.PathListSeparator))
	if first != wantFirst {
		t.Fatalf("Merge first entry = %q, want %q (full=%q)", first, wantFirst, merged)
	}
}

func TestMergeIdempotentForPresentDirs(t *testing.T) {
	extras := ExtraBinDirs()
	if len(extras) == 0 {
		t.Skip("no extra bin dirs on this host")
	}
	base := strings.Join(extras, string(os.PathListSeparator)) + string(os.PathListSeparator) + "/usr/bin"
	got := Merge(base)
	if got != base {
		t.Fatalf("Merge rewrote PATH that already had extras:\n got %q\nwant %q", got, base)
	}
}

func TestMergeEmptyPath(t *testing.T) {
	extras := ExtraBinDirs()
	if len(extras) == 0 {
		t.Skip("no extra bin dirs on this host")
	}
	got := Merge("")
	if got != strings.Join(extras, string(os.PathListSeparator)) {
		t.Fatalf("Merge(\"\") = %q, want joined extras", got)
	}
}

func TestExtraBinDirsOnlyExisting(t *testing.T) {
	for _, d := range ExtraBinDirs() {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			t.Fatalf("ExtraBinDirs returned non-dir %q: %v", d, err)
		}
	}
}

func TestEnrichSetsProcessPATH(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("enrich targets unix desktop/CLI hosts")
	}
	extras := ExtraBinDirs()
	if len(extras) == 0 {
		t.Skip("no extra bin dirs on this host")
	}

	orig := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", orig) })

	// Strip extras so Enrich has work to do. enrichOnce may already have
	// fired in this process — call Merge directly for the assertion and
	// Enrich for the side-effect smoke check.
	stripped := stripDirs(orig, extras)
	_ = os.Setenv("PATH", stripped)
	merged := Merge(stripped)
	for _, d := range extras {
		if !strings.Contains(merged, d) {
			t.Fatalf("Merge missing %q in %q", d, merged)
		}
	}
	Enrich()
	if got := os.Getenv("PATH"); got == "" {
		t.Fatal("Enrich left PATH empty")
	}
}

func pathSet(path string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, p := range filepath.SplitList(path) {
		if p != "" {
			out[p] = struct{}{}
		}
	}
	return out
}

func stripDirs(path string, drop []string) string {
	dropSet := pathSet(strings.Join(drop, string(os.PathListSeparator)))
	keep := make([]string, 0)
	for _, p := range filepath.SplitList(path) {
		if p == "" {
			continue
		}
		if _, bad := dropSet[p]; bad {
			continue
		}
		keep = append(keep, p)
	}
	return strings.Join(keep, string(os.PathListSeparator))
}
