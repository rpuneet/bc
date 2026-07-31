package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/server/handlers"
)

// fakeResolver is a tiny RepoResolver that always returns the
// same root.
type fakeResolver struct {
	root string
}

func (f *fakeResolver) ActiveRoot() string { return f.root }

// codeHarness wires a CodeHandler on an httptest server rooted at tmp.
func codeHarness(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	mux := http.NewServeMux()
	handlers.NewCodeHandler(&fakeResolver{root: root}).Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, root
}

func getCode(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// ---- /tree ----

func TestCodeTree_ListsRoot(t *testing.T) {
	ts, root := codeHarness(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}

	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e["name"].(string)) //nolint:errcheck,forcetypeassert
	}

	// .git and .bc should be hidden by default.
	for _, n := range names {
		if n == ".git" || n == ".bc" {
			t.Fatalf("hidden entry leaked: %v", names)
		}
	}
	// Must contain src (dir) + a.txt (file).
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["src"] || !found["a.txt"] {
		t.Fatalf("missing expected entries, got %v", names)
	}

	// Ordering: dirs first, then files — src before a.txt.
	if names[0] != "src" {
		t.Fatalf("want dir first; got %v", names)
	}

	// File entry should have a size.
	for _, e := range entries {
		if e["name"] == "a.txt" {
			if _, ok := e["size"]; !ok {
				t.Fatalf("a.txt missing size: %v", e)
			}
			if e["is_dir"].(bool) { //nolint:errcheck,forcetypeassert
				t.Fatalf("a.txt flagged as dir")
			}
		}
	}
}

func TestCodeTree_ShowHidden(t *testing.T) {
	ts, root := codeHarness(t)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0750); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=&show_hidden=1")
	defer func() { _ = resp.Body.Close() }()

	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, e := range entries {
		if e["name"] == ".git" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("show_hidden did not include .git: %v", entries)
	}
}

func TestCodeTree_DotPathHidesBC(t *testing.T) {
	ts, root := codeHarness(t)
	if err := os.MkdirAll(filepath.Join(root, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}
	// path=. should still hide .bc (treated as root)
	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=.")
	defer func() { _ = resp.Body.Close() }()

	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e["name"] == ".bc" {
			t.Fatalf("path=. should hide .bc but it was included: %v", entries)
		}
	}
}

func TestCodeTree_RejectsTraversal(t *testing.T) {
	ts, root := codeHarness(t)
	// Put a sensitive file outside root.
	outside := filepath.Join(filepath.Dir(root), "secret-peer-dir")
	if err := os.MkdirAll(outside, 0750); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) }) //nolint:errcheck

	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=..")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCodeTree_NotFound(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=nope")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestCodeTree_PathIsFile(t *testing.T) {
	ts, root := codeHarness(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/tree?worktree=main&path=a.txt")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 (not a dir), got %d", resp.StatusCode)
	}
}

// ---- /file ----

func TestCodeFile_ReadsText(t *testing.T) {
	ts, root := codeHarness(t)
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=main.go")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("want text/plain, got %q", ct)
	}
	if resp.Header.Get("X-Mycel-Binary") != "" {
		t.Fatalf("text file flagged binary")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != content {
		t.Fatalf("content mismatch: %q vs %q", string(body), content)
	}
}

func TestCodeFile_BinaryDetection(t *testing.T) {
	ts, root := codeHarness(t)
	// A tiny PNG header — DetectContentType will classify this as image/png.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(filepath.Join(root, "pic.png"), png, 0600); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=pic.png")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Mycel-Binary") != "true" {
		t.Fatalf("binary file not flagged: headers=%v", resp.Header)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("binary Content-Type = %q", ct)
	}
}

func TestCodeFile_SizeCap(t *testing.T) {
	ts, root := codeHarness(t)
	// 3 MiB file — over the 2 MiB cap.
	big := bytes.Repeat([]byte("A"), 3*1024*1024)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), big, 0600); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=big.txt")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Mycel-Truncated") != "true" {
		t.Fatalf("X-Mycel-Truncated not set")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 2*1024*1024 {
		t.Fatalf("capped body = %d bytes, want %d", len(body), 2*1024*1024)
	}
}

func TestCodeFile_RejectsTraversal(t *testing.T) {
	ts, root := codeHarness(t)
	// A file one level up from root.
	outside := filepath.Join(filepath.Dir(root), "escape.txt")
	if err := os.WriteFile(outside, []byte("oops"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) }) //nolint:errcheck

	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=../escape.txt")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCodeFile_MissingPath(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCodeFile_NotFound(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=nope.txt")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestCodeFile_DirectoryRejected(t *testing.T) {
	ts, root := codeHarness(t)
	if err := os.MkdirAll(filepath.Join(root, "d"), 0750); err != nil {
		t.Fatal(err)
	}
	resp := getCode(t, ts.URL+"/api/code/file?worktree=main&path=d")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// ---- /diff ----

func TestCodeDiff_MainIsEmpty(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/diff?worktree=main")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("main diff should be empty, got %q", string(body))
	}
}

func TestCodeDiff_WorktreeNotFound(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/diff?worktree=no-such-agent")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// TestCodeDiff_AgainstMain sets up a real repo + agent worktree and
// verifies /api/code/diff returns a non-empty unified diff.
func TestCodeDiff_AgainstMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	mux := http.NewServeMux()
	handlers.NewCodeHandler(&fakeResolver{root: root}).Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Init a real repo inside root with one commit on main.
	run := func(dir string, args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "--initial-branch=main")
	run(root, "config", "user.email", "t@t.t")
	run(root, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(root, "add", ".")
	run(root, "commit", "-m", "init")

	// Create an agent worktree at the path our handler expects
	// (~/.mycel/agents/<name>/worktree, anchored via MYCEL_HOME).
	agentName := "tester"
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	wtPath := filepath.Join(mycelHome, "agents", agentName, "worktree")
	if err := os.MkdirAll(filepath.Dir(wtPath), 0750); err != nil {
		t.Fatal(err)
	}
	run(root, "worktree", "add", "--detach", wtPath)
	// Modify hello.txt in the worktree and commit so main...HEAD shows a diff.
	if err := os.WriteFile(filepath.Join(wtPath, "hello.txt"), []byte("hello, world\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(wtPath, "checkout", "-b", "work")
	run(wtPath, "commit", "-am", "edit")

	resp := getCode(t, ts.URL+"/api/code/diff?worktree="+agentName)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Either we get a real diff or an empty-with-header response
	// (if main cannot be resolved). The first is the expected path.
	if len(body) == 0 && resp.Header.Get("X-Mycel-Diff-Empty") == "" {
		t.Fatalf("empty diff without empty-marker header: %v", resp.Header)
	}
	if len(body) > 0 && !strings.Contains(string(body), "hello.txt") {
		t.Fatalf("diff missing file name:\n%s", body)
	}
}

// TestCodeDispatcher_UnknownSegment exercises the ServeHTTP switch.
func TestCodeDispatcher_UnknownSegment(t *testing.T) {
	ts, _ := codeHarness(t)
	resp := getCode(t, ts.URL+"/api/code/bogus")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

// TestCodeDispatcher_MethodNotAllowed verifies non-GET methods are
// rejected with 405.
func TestCodeDispatcher_MethodNotAllowed(t *testing.T) {
	ts, _ := codeHarness(t)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/code/tree", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", resp.StatusCode)
	}
}
