package cost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestImporter_ImportAll_IngestsRecords(t *testing.T) {
	s := openTestStore(t)
	projectsDir := t.TempDir()

	// Write a fake JSONL session file.
	sessionDir := filepath.Join(projectsDir, "-my-project")
	if err := os.MkdirAll(sessionDir, 0750); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"assistant","sessionId":"sess1","timestamp":"2026-03-10T12:00:00Z","cwd":"/my-project","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"assistant","sessionId":"sess1","timestamp":"2026-03-10T12:00:01Z","cwd":"/my-project","message":{"model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":80}}}
`
	jsonlPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}

	// Override dirs by calling importFile directly via a helper.
	n, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 records imported, got %d", n)
	}

	summary, err := s.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 2 {
		t.Errorf("want 2 records in store, got %d", summary.RecordCount)
	}
	if summary.InputTokens != 300 {
		t.Errorf("want 300 input tokens, got %d", summary.InputTokens)
	}
}

func TestImporter_Idempotent(t *testing.T) {
	s := openTestStore(t)
	content := `{"type":"assistant","sessionId":"s2","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":50,"output_tokens":20}}}
`
	jsonlPath := filepath.Join(t.TempDir(), "s2.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}

	n1, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	if n1 != 1 {
		t.Errorf("first import: want 1, got %d", n1)
	}
	if n2 != 0 {
		t.Errorf("second import should be 0 (idempotent), got %d", n2)
	}
}

func TestImporter_NewEntriesAfterWatermark(t *testing.T) {
	s := openTestStore(t)

	jsonlPath := filepath.Join(t.TempDir(), "s3.jsonl")
	line1 := `{"type":"assistant","sessionId":"s3","timestamp":"2026-03-10T10:00:00Z","cwd":"/p","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(line1), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}

	if _, err := imp.importFile(context.Background(), jsonlPath); err != nil {
		t.Fatal(err)
	}

	// Append a new entry with a later timestamp.
	line2 := `{"type":"assistant","sessionId":"s3","timestamp":"2026-03-10T11:00:00Z","cwd":"/p","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":8}}}` + "\n"
	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // test file, path is controlled
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(line2)
	_ = f.Close()

	n, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 new record after watermark, got %d", n)
	}
}

func TestImporter_ResolveAgent_DockerPath(t *testing.T) {
	wsDir := t.TempDir()
	imp := &Importer{store: nil, workspaceDir: wsDir}

	agentsDir := filepath.Join(wsDir, ".bc", "agents")
	dockerPath := filepath.Join(agentsDir, "my-agent", "auth", ".claude", "projects", "-proj", "sess.jsonl")

	agent := imp.resolveAgent("/some/cwd", dockerPath)
	if agent != "my-agent" {
		t.Errorf("want agent 'my-agent', got %q", agent)
	}
}

func TestImporter_ResolveAgent_HostPath(t *testing.T) {
	wsDir := t.TempDir()
	imp := &Importer{store: nil, workspaceDir: wsDir}

	hostPath := "/home/user/.claude/projects/-some-project/sess.jsonl"
	agent := imp.resolveAgent("/workspace/my-project", hostPath)
	if agent != "my-project" {
		t.Errorf("want 'my-project' from CWD, got %q", agent)
	}
}

func TestNewImporter(t *testing.T) {
	s := openTestStore(t)
	wsDir := t.TempDir()
	imp := NewImporter(s, wsDir)
	if imp == nil {
		t.Fatal("NewImporter returned nil")
	}
	if imp.store != s {
		t.Error("store not set correctly")
	}
	if imp.workspaceDir != wsDir {
		t.Errorf("workspaceDir = %q, want %q", imp.workspaceDir, wsDir)
	}
}

func TestImporter_ImportAll_NoError(t *testing.T) {
	s := openTestStore(t)
	wsDir := t.TempDir()
	imp := NewImporter(s, wsDir)

	// ImportAll scans the host ~/.claude/projects/ dir plus any Docker agent
	// dirs. It should complete without error regardless of what is on disk.
	_, err := imp.ImportAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestImporter_ImportAll_WithDockerAgentSessions(t *testing.T) {
	s := openTestStore(t)
	wsDir := t.TempDir()

	// Create a Docker agent projects directory with a JSONL file.
	agentDir := filepath.Join(wsDir, ".bc", "agents", "docker-agent", "auth", ".claude", "projects", "-proj")
	if err := os.MkdirAll(agentDir, 0750); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"assistant","sessionId":"ds1","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":50,"output_tokens":20}}}
`
	if err := os.WriteFile(filepath.Join(agentDir, "ds1.jsonl"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	imp := NewImporter(s, wsDir)
	n, err := imp.ImportAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// At least 1 record from the Docker agent session (may be more from host
	// ~/.claude/projects/ if it exists on the machine).
	if n < 1 {
		t.Errorf("want at least 1 imported, got %d", n)
	}
}

func TestImporter_ImportAll_ContextCancelled(t *testing.T) {
	s := openTestStore(t)
	wsDir := t.TempDir()

	// Create a Docker agent projects directory with JSONL files.
	agentDir := filepath.Join(wsDir, ".bc", "agents", "agent1", "auth", ".claude", "projects", "-proj")
	if err := os.MkdirAll(agentDir, 0750); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"assistant","sessionId":"cs1","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5}}}
`
	if err := os.WriteFile(filepath.Join(agentDir, "cs1.jsonl"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	imp := NewImporter(s, wsDir)
	_, err := imp.ImportAll(ctx)
	if err != nil {
		// Context cancellation is returned as an error.
		if err != context.Canceled {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestImporter_ClaudeProjectsDirs(t *testing.T) {
	wsDir := t.TempDir()
	imp := &Importer{store: nil, workspaceDir: wsDir}

	dirs := imp.claudeProjectsDirs()
	// Should include at least the home/.claude/projects dir.
	if len(dirs) == 0 {
		t.Fatal("expected at least one directory")
	}

	// Now create a Docker agent auth dir and check it shows up.
	agentAuth := filepath.Join(wsDir, ".bc", "agents", "test-agent", "auth", ".claude", "projects")
	if err := os.MkdirAll(agentAuth, 0750); err != nil {
		t.Fatal(err)
	}
	dirs = imp.claudeProjectsDirs()
	found := false
	for _, d := range dirs {
		if d == agentAuth {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Docker agent projects dir %q in dirs: %v", agentAuth, dirs)
	}
}

func TestImporter_ClaudeProjectsDirs_SkipsFiles(t *testing.T) {
	wsDir := t.TempDir()
	agentsDir := filepath.Join(wsDir, ".bc", "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Create a file (not dir) in agents dir — should be skipped.
	if err := os.WriteFile(filepath.Join(agentsDir, "not-a-dir"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	imp := &Importer{store: nil, workspaceDir: wsDir}
	dirs := imp.claudeProjectsDirs()
	for _, d := range dirs {
		if filepath.Base(d) == "not-a-dir" {
			t.Error("file entry should be skipped, not listed as dir")
		}
	}
}

func TestImporter_ResolveAgent_EmptyCWD(t *testing.T) {
	wsDir := t.TempDir()
	imp := &Importer{store: nil, workspaceDir: wsDir}

	agent := imp.resolveAgent("", "/some/random/path/sess.jsonl")
	if agent != "unknown" {
		t.Errorf("want 'unknown' for empty CWD, got %q", agent)
	}
}

func TestImporter_ImportFile_MalformedJSONL(t *testing.T) {
	s := openTestStore(t)
	jsonlPath := filepath.Join(t.TempDir(), "bad.jsonl")
	// Write a completely malformed file — all lines are bad JSON.
	if err := os.WriteFile(jsonlPath, []byte("not json at all\nalso bad\n"), 0600); err != nil {
		t.Fatal(err)
	}
	imp := &Importer{store: s, workspaceDir: t.TempDir()}
	n, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("want 0 records from malformed JSONL, got %d", n)
	}
}

func TestImporter_ImportFile_NonExistentFile(t *testing.T) {
	s := openTestStore(t)
	imp := &Importer{store: s, workspaceDir: t.TempDir()}
	_, err := imp.importFile(context.Background(), "/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestImporter_ImportFile_CacheTokens(t *testing.T) {
	s := openTestStore(t)
	content := `{"type":"assistant","sessionId":"cache1","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":300,"cache_read_input_tokens":200}}}
`
	jsonlPath := filepath.Join(t.TempDir(), "cache.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}
	n, err := imp.importFile(context.Background(), jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 record imported, got %d", n)
	}

	// Cache tokens are reported separately — total is input + output only.
	summary, err := s.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Total = input(100) + output(50) = 150; cache tokens excluded.
	if summary.TotalTokens != 150 {
		t.Errorf("want 150 total tokens (input+output), got %d", summary.TotalTokens)
	}
	if summary.CacheWriteTokens != 300 {
		t.Errorf("want 300 cache write tokens, got %d", summary.CacheWriteTokens)
	}
	if summary.CacheReadTokens != 200 {
		t.Errorf("want 200 cache read tokens, got %d", summary.CacheReadTokens)
	}
}

// TestImporter_DeduplicatesAcrossFiles covers the compaction-sidechain bug:
// Claude Code writes <session>/subagents/agent-acompact-*.jsonl files that
// replicate the parent session's assistant messages verbatim. The per-file
// watermark can't catch that, so the insert-level dedup guard must.
func TestImporter_DeduplicatesAcrossFiles(t *testing.T) {
	s := openTestStore(t)
	entry := `{"type":"assistant","sessionId":"dup1","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":500}}}
{"type":"assistant","sessionId":"dup1","timestamp":"2026-03-10T12:00:05Z","cwd":"/proj","message":{"model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":80}}}
`
	dir := t.TempDir()
	parent := filepath.Join(dir, "dup1.jsonl")
	if err := os.WriteFile(parent, []byte(entry), 0600); err != nil {
		t.Fatal(err)
	}
	// Compaction sidechain file with identical content at a different path.
	subDir := filepath.Join(dir, "dup1", "subagents")
	if err := os.MkdirAll(subDir, 0750); err != nil {
		t.Fatal(err)
	}
	sidechain := filepath.Join(subDir, "agent-acompact-abc123.jsonl")
	if err := os.WriteFile(sidechain, []byte(entry), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}

	n1, err := imp.importFile(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 2 {
		t.Fatalf("want 2 records from parent file, got %d", n1)
	}

	n2, err := imp.importFile(context.Background(), sidechain)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("want 0 records from duplicate sidechain file, got %d", n2)
	}

	summary, err := s.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 2 {
		t.Errorf("want 2 records after dedup, got %d", summary.RecordCount)
	}
	if summary.InputTokens != 300 {
		t.Errorf("want 300 input tokens after dedup, got %d", summary.InputTokens)
	}

	// Re-importing the sidechain must stay a no-op (watermark advanced even
	// though nothing was inserted).
	n3, err := imp.importFile(context.Background(), sidechain)
	if err != nil {
		t.Fatal(err)
	}
	if n3 != 0 {
		t.Errorf("want 0 records on sidechain re-import, got %d", n3)
	}
}

// TestImporter_DistinctEntriesSameSessionNotDeduped ensures legitimate
// subagent usage (same sessionId, different timestamps/token counts) is
// still imported.
func TestImporter_DistinctEntriesSameSessionNotDeduped(t *testing.T) {
	s := openTestStore(t)
	dir := t.TempDir()
	parent := filepath.Join(dir, "p1.jsonl")
	if err := os.WriteFile(parent, []byte(
		`{"type":"assistant","sessionId":"p1","timestamp":"2026-03-10T12:00:00Z","cwd":"/proj","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}}
`), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub.jsonl")
	if err := os.WriteFile(sub, []byte(
		`{"type":"assistant","sessionId":"p1","timestamp":"2026-03-10T12:00:03Z","cwd":"/proj","message":{"model":"claude-opus-4-6","usage":{"input_tokens":40,"output_tokens":10}}}
`), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &Importer{store: s, workspaceDir: t.TempDir()}
	if _, err := imp.importFile(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	n, err := imp.importFile(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 record from distinct subagent entry, got %d", n)
	}

	summary, err := s.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 2 {
		t.Errorf("want 2 records, got %d", summary.RecordCount)
	}
}

func TestImporterSchema_MigratesColumns(t *testing.T) {
	s := openTestStore(t)
	// Verify the new columns exist by inserting a record that uses them.
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO cost_records (agent_id, model, session_id, input_tokens, output_tokens, total_tokens, cache_creation_tokens, cache_read_tokens, cost_usd, timestamp) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"agent-a", "claude-sonnet-4-6", "sess-x", 1, 1, 2, 3, 4, 0.001,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("expected migration to add columns, got: %v", err)
	}
}
