package provider

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJSONL writes a Claude Code session transcript with the given
// usage lines.
func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func usageLine(session, ts, cwd, model string, in, out, cacheW, cacheR int64) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":%q,"timestamp":%q,"cwd":%q,"message":{"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`,
		session, ts, cwd, model, in, out, cacheW, cacheR)
}

func TestClaudeReadCostsHostAndAgentSessions(t *testing.T) {
	home := t.TempDir()
	agents := t.TempDir()

	// Host session — attributed by CWD basename.
	writeJSONL(t, filepath.Join(home, ".claude", "projects", "p1", "11111111-1111-1111-1111-111111111111.jsonl"),
		usageLine("s-host", "2026-07-01T10:00:00Z", "/repos/myproj", "claude-sonnet-4-20250514", 1000, 500, 0, 0),
		`{"type":"user","text":"no usage line"}`,
	)
	// Docker agent session — attributed to the entity dir name.
	writeJSONL(t, filepath.Join(agents, "zen-zebra", "session", "claude", "projects", "p2", "22222222-2222-2222-2222-222222222222.jsonl"),
		usageLine("s-agent", "2026-07-02T10:00:00Z", "/workspace", "claude-opus-4-5-20251101", 200, 100, 50, 400),
	)

	p := NewClaudeProvider()
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{Home: home, AgentsDir: agents})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}

	byAgent := map[string]CostEntry{}
	for _, e := range entries {
		byAgent[e.Agent] = e
	}

	host, ok := byAgent["myproj"]
	if !ok {
		t.Fatalf("host entry not attributed by cwd basename: %+v", byAgent)
	}
	if host.Repo != "/repos/myproj" || host.Model != "claude-sonnet-4-20250514" || host.SessionID != "s-host" {
		t.Errorf("host entry fields wrong: %+v", host)
	}
	// claude-sonnet-4: $3/M input + $15/M output.
	wantHostCost := 1000*3.0/1e6 + 500*15.0/1e6
	if math.Abs(host.CostUSD-wantHostCost) > 1e-9 {
		t.Errorf("host cost = %v, want %v", host.CostUSD, wantHostCost)
	}

	ag, ok := byAgent["zen-zebra"]
	if !ok {
		t.Fatalf("agent entry not attributed to entity dir: %+v", byAgent)
	}
	if ag.CacheWriteTokens != 50 || ag.CacheReadTokens != 400 {
		t.Errorf("cache tokens wrong: %+v", ag)
	}
	// claude-opus-4-5: $5/$25, cache write 6.25, cache read 0.5 per M.
	wantAgentCost := 200*5.0/1e6 + 100*25.0/1e6 + 50*6.25/1e6 + 400*0.5/1e6
	if math.Abs(ag.CostUSD-wantAgentCost) > 1e-9 {
		t.Errorf("agent cost = %v, want %v", ag.CostUSD, wantAgentCost)
	}
}

func TestClaudeReadCostsDedupsCompactionSidechains(t *testing.T) {
	agents := t.TempDir()
	line := usageLine("s1", "2026-07-01T10:00:00Z", "/workspace", "claude-sonnet-4-20250514", 100, 50, 0, 0)

	base := filepath.Join(agents, "a1", "session", "claude", "projects", "p")
	writeJSONL(t, filepath.Join(base, "33333333-3333-3333-3333-333333333333.jsonl"), line)
	// Compaction sidechain replicates the same usage verbatim.
	writeJSONL(t, filepath.Join(base, "33333333-3333-3333-3333-333333333333", "subagents", "agent-acompact-1.jsonl"), line)

	p := NewClaudeProvider()
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{AgentsDir: agents})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("duplicate usage not deduped: got %d entries", len(entries))
	}
}

func TestClaudeReadCostsSinceFilter(t *testing.T) {
	agents := t.TempDir()
	writeJSONL(t, filepath.Join(agents, "a1", "session", "claude", "projects", "p", "44444444-4444-4444-4444-444444444444.jsonl"),
		usageLine("s1", "2026-06-01T10:00:00Z", "/w", "claude-sonnet-4-20250514", 10, 5, 0, 0),
		usageLine("s1", "2026-07-01T10:00:00Z", "/w", "claude-sonnet-4-20250514", 20, 10, 0, 0),
	)

	p := NewClaudeProvider()
	since := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	entries, err := p.ReadCosts(context.Background(), CostReadOptions{AgentsDir: agents, Since: since})
	if err != nil {
		t.Fatalf("ReadCosts: %v", err)
	}
	if len(entries) != 1 || entries[0].InputTokens != 20 {
		t.Fatalf("since filter wrong: %+v", entries)
	}
}

func TestClaudePricingPrefixOrder(t *testing.T) {
	// Opus 4.5 must hit the cheaper tier, not the generic claude-opus-4.
	if got := ClaudePricingFor("claude-opus-4-5-20251101").InputPerM; got != 5.00 {
		t.Errorf("opus-4-5 input = %v, want 5.00", got)
	}
	if got := ClaudePricingFor("claude-opus-4-1-20250805").InputPerM; got != 15.00 {
		t.Errorf("opus-4-1 input = %v, want 15.00", got)
	}
	// Unknown models get the default tier.
	if got := ClaudePricingFor("wat-model").InputPerM; got != 3.00 {
		t.Errorf("default input = %v, want 3.00", got)
	}
}

func TestResolveClaudeAgentWorktreeMatch(t *testing.T) {
	agentsDir := filepath.Join(string(filepath.Separator), "mycel", "agents")
	cwd := filepath.Join(agentsDir, "zen-zebra", "worktree", "sub")
	if got := resolveClaudeAgent("", cwd, agentsDir); got != "zen-zebra" {
		t.Errorf("resolveClaudeAgent = %q, want zen-zebra", got)
	}
	if got := resolveClaudeAgent("explicit", cwd, agentsDir); got != "explicit" {
		t.Errorf("explicit root agent must win, got %q", got)
	}
	if got := resolveClaudeAgent("", "", agentsDir); got != "unknown" {
		t.Errorf("empty cwd = %q, want unknown", got)
	}
}
