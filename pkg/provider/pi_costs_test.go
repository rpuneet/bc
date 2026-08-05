package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPiReadCostsFromSession(t *testing.T) {
	root := t.TempDir()
	agents := t.TempDir()
	agentWT := filepath.Join(agents, "fresh-otter", "worktree")
	if err := os.MkdirAll(agentWT, 0o750); err != nil {
		t.Fatal(err)
	}
	cwdEnc := encodePiCWD(agentWT)
	sessDir := filepath.Join(root, cwdEnc)
	if err := os.MkdirAll(sessDir, 0o750); err != nil {
		t.Fatal(err)
	}
	sid := "019fcf1b-cec6-778f-b1db-673f101f32ab"
	path := filepath.Join(sessDir, "2026-08-04T23-28-53-958Z_"+sid+".jsonl")
	content := `{"type":"session","version":3,"id":"` + sid + `","timestamp":"2026-08-04T23:28:53.958Z","cwd":"` + agentWT + `"}
{"type":"message","timestamp":"2026-08-04T23:29:01.199Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"message","timestamp":"2026-08-04T23:29:10.000Z","message":{"role":"assistant","api":"bedrock-converse-stream","provider":"amazon-bedrock","model":"minimax.minimax-m2.5","usage":{"input":2746,"output":108,"cacheRead":0,"cacheWrite":0,"totalTokens":2854,"cost":{"input":0.0008238,"output":0.0001296,"cacheRead":0,"cacheWrite":0,"total":0.0009534}},"content":[]}}
{"type":"message","timestamp":"2026-08-04T23:30:00.000Z","message":{"role":"assistant","provider":"amazon-bedrock","model":"deepseek.v3.2","usage":{"input":100,"output":10,"cacheRead":0,"cacheWrite":0,"cost":{"total":0.001}}}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	prev := piSessionsRoot
	piSessionsRoot = func() string { return root }
	t.Cleanup(func() { piSessionsRoot = prev })

	p := NewPiProvider()
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{AgentsDir: agents})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	e0 := entries[0]
	if e0.Agent != "fresh-otter" {
		t.Errorf("Agent = %q, want fresh-otter", e0.Agent)
	}
	if e0.Model != "amazon-bedrock/minimax.minimax-m2.5" {
		t.Errorf("Model = %q", e0.Model)
	}
	if e0.SessionID != sid {
		t.Errorf("SessionID = %q, want %s", e0.SessionID, sid)
	}
	if e0.InputTokens != 2746 || e0.OutputTokens != 108 {
		t.Errorf("tokens = %d/%d", e0.InputTokens, e0.OutputTokens)
	}
	if e0.CostUSD < 0.0009 || e0.CostUSD > 0.001 {
		t.Errorf("CostUSD = %v, want ~0.0009534", e0.CostUSD)
	}
	if entries[1].CostUSD != 0.001 {
		t.Errorf("second CostUSD = %v", entries[1].CostUSD)
	}
}

func TestPiReadCostsSinceFilter(t *testing.T) {
	root := t.TempDir()
	agents := t.TempDir()
	wt := filepath.Join(agents, "pi-bedrock", "worktree")
	if err := os.MkdirAll(wt, 0o750); err != nil {
		t.Fatal(err)
	}
	sessDir := filepath.Join(root, encodePiCWD(wt))
	if err := os.MkdirAll(sessDir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessDir, "sess_abc.jsonl")
	content := `{"type":"session","id":"abc","cwd":"` + wt + `","timestamp":"2026-08-04T23:00:00Z"}
{"type":"message","timestamp":"2026-08-04T23:01:00Z","message":{"role":"assistant","provider":"amazon-bedrock","model":"x","usage":{"input":1,"output":1,"cost":{"total":0.01}}}}
{"type":"message","timestamp":"2026-08-04T23:10:00Z","message":{"role":"assistant","provider":"amazon-bedrock","model":"x","usage":{"input":2,"output":2,"cost":{"total":0.02}}}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := piSessionsRoot
	piSessionsRoot = func() string { return root }
	t.Cleanup(func() { piSessionsRoot = prev })

	since, _ := time.Parse(time.RFC3339, "2026-08-04T23:05:00Z")
	entries, err := NewPiProvider().ReadCosts(context.Background(), CostReadOptions{
		AgentsDir: agents,
		Since:     since,
	})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d, want 1", len(entries))
	}
	if entries[0].CostUSD != 0.02 {
		t.Errorf("CostUSD = %v", entries[0].CostUSD)
	}
	if entries[0].Agent != "pi-bedrock" {
		t.Errorf("Agent = %q", entries[0].Agent)
	}
}
