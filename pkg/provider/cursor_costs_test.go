package provider

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadCursorUsage(t *testing.T) {
	agents := t.TempDir()
	rec := CursorUsageRecord{
		Timestamp:        time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
		Model:            "kimi-k3",
		SessionID:        "sess-1",
		GenerationID:     "gen-1",
		InputTokens:      1000,
		OutputTokens:     200,
		CacheReadTokens:  500,
		CacheWriteTokens: 50,
	}
	if err := AppendCursorUsage(agents, "swift-fox", rec); err != nil {
		t.Fatalf("AppendCursorUsage: %v", err)
	}

	p := NewCursorProvider()
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{AgentsDir: agents})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	got := entries[0]
	if got.Agent != "swift-fox" {
		t.Errorf("agent = %q", got.Agent)
	}
	if got.InputTokens != 1000 || got.OutputTokens != 200 {
		t.Errorf("tokens = %d/%d", got.InputTokens, got.OutputTokens)
	}
	want := CursorCalcCost(rec.Model, rec.InputTokens, rec.OutputTokens, rec.CacheWriteTokens, rec.CacheReadTokens)
	if math.Abs(got.CostUSD-want) > 1e-9 {
		t.Errorf("cost = %v, want %v", got.CostUSD, want)
	}
}

func TestReadCostsDedupsByGenerationID(t *testing.T) {
	agents := t.TempDir()
	rec := CursorUsageRecord{
		Timestamp:    time.Now().UTC(),
		Model:        "default",
		GenerationID: "same",
		InputTokens:  10,
		OutputTokens: 5,
	}
	_ = AppendCursorUsage(agents, "a", rec)
	_ = AppendCursorUsage(agents, "a", rec)

	p := NewCursorProvider()
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{AgentsDir: agents})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 after dedup", len(entries))
	}
}

func TestAppendCursorUsageSkipsEmpty(t *testing.T) {
	agents := t.TempDir()
	if err := AppendCursorUsage(agents, "a", CursorUsageRecord{Model: "default"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(agents, "a", CursorUsageRelDir, CursorUsageFile)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file for zero-token record, err=%v", err)
	}
}

func TestCursorCalcCostUsesClaudePricingForClaudeModels(t *testing.T) {
	got := CursorCalcCost("claude-sonnet-4", 1_000_000, 0, 0, 0)
	want := ClaudeCalcCost("claude-sonnet-4", 1_000_000, 0, 0, 0)
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCursorUsageBackfill(t *testing.T) {
	// Set up test directories
	agentsDir := t.TempDir()
	eventsFile := filepath.Join(t.TempDir(), "events.jsonl")

	// Create events.jsonl with 3 Stop events (2 will be new, 1 already exists)
	eventsData := []string{
		// Event 1: gen-1 (will be backfilled)
		`{"type":"agent.hook","data":{"agent":"test-agent","event":"Stop","generation_id":"gen-1","session_id":"sess-1","model":"default","input_tokens":1000,"output_tokens":100}}`,
		// Event 2: gen-2 (will be backfilled)
		`{"type":"agent.hook","data":{"agent":"test-agent","event":"Stop","generation_id":"gen-2","session_id":"sess-1","model":"default","input_tokens":2000,"output_tokens":200}}`,
		// Event 3: gen-3 (already exists, should be skipped)
		`{"type":"agent.hook","data":{"agent":"test-agent","event":"Stop","generation_id":"gen-3","session_id":"sess-1","model":"default","input_tokens":3000,"output_tokens":300}}`,
	}
	f, err := os.Create(eventsFile) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("create events file: %v", err)
	}
	for _, line := range eventsData {
		_, werr := f.WriteString(line + "\n")
		if werr != nil {
			t.Fatalf("write event: %v", werr)
		}
	}
	f.Close() //nolint:errcheck

	// Create existing usage.jsonl with gen-3 already present
	usageDir := filepath.Join(agentsDir, "test-agent", CursorUsageRelDir)
	if merr := os.MkdirAll(usageDir, 0o750); merr != nil {
		t.Fatalf("mkdir usage dir: %v", merr)
	}
	existingRec := CursorUsageRecord{
		GenerationID: "gen-3",
		SessionID:    "sess-1",
		Model:        "default",
		InputTokens:  3000,
		OutputTokens: 300,
	}
	usageFile := filepath.Join(usageDir, CursorUsageFile)
	existingData, _ := json.Marshal(existingRec)
	if wferr := os.WriteFile(usageFile, append(existingData, '\n'), 0o600); wferr != nil { //nolint:gosec // test file
		t.Fatalf("write existing usage: %v", wferr)
	}

	// Run backfill
	appended, err := CursorUsageBackfill(eventsFile, agentsDir)
	if err != nil {
		t.Fatalf("CursorUsageBackfill: %v", err)
	}
	if appended != 2 {
		t.Errorf("appended = %d, want 2 (gen-1 and gen-2)", appended)
	}

	// Read back usage.jsonl and verify 3 records exist
	recs, err := readCursorUsageFile(usageFile)
	if err != nil {
		t.Fatalf("read usage file: %v", err)
	}
	if len(recs) != 3 {
		t.Errorf("got %d records, want 3", len(recs))
	}

	// Verify expected generation IDs
	expected := map[string]bool{"gen-1": true, "gen-2": true, "gen-3": true}
	for _, rec := range recs {
		if rec.GenerationID == "" {
			t.Errorf("record missing generation_id")
			continue
		}
		if !expected[rec.GenerationID] {
			t.Errorf("unexpected generation_id: %s", rec.GenerationID)
		}
	}
}
