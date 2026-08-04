package provider

import (
	"context"
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
