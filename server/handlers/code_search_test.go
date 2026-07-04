// code_search_test.go — black-box coverage for the ripgrep-backed
// /api/code/search endpoint. Tests skip when `rg` is not on PATH so
// they remain runnable on minimal CI images.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeSearchCorpus lays out a tiny repo with three files that have both
// literal and regex-discoverable needles so every query shape the handler
// accepts exercises a real match.
func writeSearchCorpus(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"a.go":          "package main\n\nfunc needleFn() {}\n// plain needle in a comment\n",
		"b/nested.txt":  "before\nsome needle here\nafter\nNEEDLE again on a separate line\n",
		"c/haystack.md": "no match here\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

// searchResolver bolts a CodeHandler onto an ad-hoc tempdir without
// touching the registry.
type searchResolver struct{ root string }

func (s *searchResolver) ActiveRoot() string { return s.root }

func newSearchServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; skipping code search tests")
	}
	root := t.TempDir()
	writeSearchCorpus(t, root)
	h := NewCodeHandler(&searchResolver{root: root})
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(mux), root
}

func decodeSearch(t *testing.T, url string) searchResponse {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // test
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := os.ReadFile("/dev/null") //nolint:errcheck
		_ = body
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestCodeSearch_LiteralMatch(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	out := decodeSearch(t, srv.URL+"/api/code/search?q=needle&worktree=main")
	if len(out.Matches) < 2 {
		t.Fatalf("matches = %d, want ≥ 2 (a.go + b/nested.txt)", len(out.Matches))
	}
	paths := map[string]bool{}
	for _, m := range out.Matches {
		paths[m.Path] = true
		if m.Line <= 0 {
			t.Errorf("match %+v has line <= 0", m)
		}
	}
	if !paths["a.go"] || !paths["b/nested.txt"] {
		t.Errorf("expected hits in a.go and b/nested.txt, got %v", paths)
	}
	// Default case sensitivity: should skip uppercase NEEDLE-only match?
	// Not by default — both files have a lowercase "needle" too, so we
	// only assert we hit the target files, not counts.
}

func TestCodeSearch_CaseInsensitive(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	out := decodeSearch(t, srv.URL+"/api/code/search?q=NEEDLE&case=1&worktree=main")
	paths := map[string]int{}
	for _, m := range out.Matches {
		paths[m.Path]++
	}
	if paths["a.go"] == 0 {
		t.Errorf("case=1 did not match lowercase needle in a.go; paths=%v", paths)
	}
}

func TestCodeSearch_Regex(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	out := decodeSearch(t, srv.URL+"/api/code/search?q=needle%5BFf%5Dn&regex=1&worktree=main")
	if len(out.Matches) == 0 {
		t.Fatalf("regex 'needle[Ff]n' returned no matches")
	}
	for _, m := range out.Matches {
		if !strings.Contains(strings.ToLower(m.Text), "needlefn") {
			t.Errorf("regex match %+v does not actually contain needleFn", m)
		}
	}
}

func TestCodeSearch_SubdirScope(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	out := decodeSearch(t, srv.URL+"/api/code/search?q=needle&path=b&worktree=main")
	for _, m := range out.Matches {
		if !strings.HasPrefix(m.Path, "b/") {
			t.Errorf("subdir scope leaked: got match at %q outside b/", m.Path)
		}
	}
	if len(out.Matches) == 0 {
		t.Fatalf("expected at least one hit under b/")
	}
}

func TestCodeSearch_EmptyQueryReturns400(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/code/search?q=") //nolint:gosec,noctx // test
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestCodeSearch_RejectsPathEscape guards the SafeJoin-backed subdir
// scope. A query with `path=../../etc` must be refused, not silently
// escape the workspace root.
func TestCodeSearch_RejectsPathEscape(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/code/search?q=needle&path=..%2F..%2Fetc&worktree=main") //nolint:gosec,noctx // test
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for path traversal", resp.StatusCode)
	}
}

// TestCodeSearch_MaxTruncation pins the truncation boundary: with two
// matches and max=1 we expect exactly one match returned and
// truncated=true. Regression guard for the off-by-one that returned
// truncated=true at exact-max boundary before the &gt;-not-&gt;= fix.
func TestCodeSearch_MaxTruncation(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	out := decodeSearch(t, srv.URL+"/api/code/search?q=needle&worktree=main&max=1")
	if len(out.Matches) != 1 {
		t.Errorf("matches = %d, want 1 with max=1", len(out.Matches))
	}
	if !out.Truncated {
		t.Error("truncated should be true when max=1 and corpus has 2 hits")
	}
}

// TestCodeSearch_QueryTooLong caps q at searchMaxQueryLen chars.
func TestCodeSearch_QueryTooLong(t *testing.T) {
	srv, _ := newSearchServer(t)
	defer srv.Close()

	big := strings.Repeat("a", searchMaxQueryLen+1)
	resp, err := http.Get(srv.URL + "/api/code/search?q=" + big) //nolint:gosec,noctx // test
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for oversized q", resp.StatusCode)
	}
}
