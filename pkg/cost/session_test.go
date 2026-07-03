package cost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSessionReader_AssistantEntries(t *testing.T) {
	jsonl := strings.NewReader(`
{"type":"user","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":{"role":"user","content":"hello"}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:01Z","cwd":"/ws","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":10}}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:02Z","cwd":"/ws","message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
`)

	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	// Second assistant entry has all-zero tokens and should be skipped.
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Model != "claude-opus-4-6" {
		t.Errorf("want model claude-opus-4-6, got %s", e.Model)
	}
	if e.InputTokens != 100 {
		t.Errorf("want 100 input tokens, got %d", e.InputTokens)
	}
	if e.OutputTokens != 50 {
		t.Errorf("want 50 output tokens, got %d", e.OutputTokens)
	}
	if e.CacheCreationTokens != 200 {
		t.Errorf("want 200 cache_creation tokens, got %d", e.CacheCreationTokens)
	}
	if e.CacheReadTokens != 10 {
		t.Errorf("want 10 cache_read tokens, got %d", e.CacheReadTokens)
	}
	if e.SessionID != "s1" {
		t.Errorf("want session s1, got %s", e.SessionID)
	}
}

func TestParseSessionReader_SkipsMalformedLines(t *testing.T) {
	jsonl := strings.NewReader(`not json
{"type":"assistant","sessionId":"s2","timestamp":"2026-03-01T10:00:01Z","cwd":"/ws","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":5,"output_tokens":3}}}
`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
}

func TestParseSessionReader_TimestampParsed(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"assistant","sessionId":"s3","timestamp":"2026-03-15T14:30:00.123Z","cwd":"/","message":{"model":"claude-opus-4-6","usage":{"input_tokens":1,"output_tokens":1}}}`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	want := time.Date(2026, 3, 15, 14, 30, 0, 123_000_000, time.UTC)
	if !entries[0].Timestamp.Equal(want) {
		t.Errorf("want timestamp %v, got %v", want, entries[0].Timestamp)
	}
}

func TestCalcCost_KnownModel(t *testing.T) {
	// claude-opus-4-6: $5/M input, $25/M output
	cost := CalcCost("claude-opus-4-6", 1_000_000, 0, 0, 0)
	if cost < 4.9 || cost > 5.1 {
		t.Errorf("want ~$5 for 1M input tokens on opus-4-6, got %.4f", cost)
	}
}

func TestCalcCost_CacheTokens(t *testing.T) {
	// cache_write for opus-4-6: $6.25/M
	cost := CalcCost("claude-opus-4-6", 0, 0, 1_000_000, 0)
	if cost < 6.2 || cost > 6.3 {
		t.Errorf("want ~$6.25 for 1M cache_write tokens on opus-4-6, got %.4f", cost)
	}
}

func TestCalcCost_CacheReadCheaperThanInput(t *testing.T) {
	// Cache reads are 0.1x input pricing — the $109k bug came from
	// pricing 62B cache-read tokens at the legacy opus input rate.
	models := []string{"claude-opus-4-6", "claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5", "claude-fable-5"}
	for _, m := range models {
		input := CalcCost(m, 1_000_000, 0, 0, 0)
		read := CalcCost(m, 0, 0, 0, 1_000_000)
		if read >= input/5 {
			t.Errorf("model %s: cache read $%.4f should be ~10x cheaper than input $%.4f", m, read, input)
		}
	}
}

func TestCalcCost_UnknownModelFallsBack(t *testing.T) {
	// Unknown model should use defaultPricing (sonnet-tier)
	cost := CalcCost("claude-unknown-99", 1_000_000, 0, 0, 0)
	if cost <= 0 {
		t.Errorf("expected positive cost for unknown model, got %f", cost)
	}
}

func TestFindSessionFiles_ReturnsJSONLFiles(t *testing.T) {
	root := t.TempDir()

	// Create nested dirs with .jsonl files and non-.jsonl files.
	sub := filepath.Join(root, "project-a")
	if err := os.MkdirAll(sub, 0750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sess1.jsonl", "sess2.jsonl", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Deeper nesting.
	deep := filepath.Join(root, "project-b", "nested")
	if err := os.MkdirAll(deep, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "sess3.jsonl"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := FindSessionFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("want 3 .jsonl files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if filepath.Ext(f) != ".jsonl" {
			t.Errorf("unexpected non-.jsonl file: %s", f)
		}
	}
}

func TestFindSessionFiles_EmptyDir(t *testing.T) {
	root := t.TempDir()
	files, err := FindSessionFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("want 0 files in empty dir, got %d", len(files))
	}
}

func TestFindSessionFiles_NonExistentDir(t *testing.T) {
	files, err := FindSessionFiles("/nonexistent/path/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("want 0 files for nonexistent dir, got %d", len(files))
	}
}

func TestParseSessionFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	content := `{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":{"model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}}
`
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseSessionFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].InputTokens != 100 {
		t.Errorf("want 100 input tokens, got %d", entries[0].InputTokens)
	}
}

func TestParseSessionFile_NonExistentFile(t *testing.T) {
	_, err := ParseSessionFile("/nonexistent/test.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseSessionReader_EmptyInput(t *testing.T) {
	entries, err := parseSessionReader(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries for empty input, got %d", len(entries))
	}
}

func TestParseSessionReader_SkipsNonAssistantTypes(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"user","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":{"role":"user"}}
{"type":"system","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":{"role":"system"}}
`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries for non-assistant types, got %d", len(entries))
	}
}

func TestParseSessionReader_SkipsEmptyMessage(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws"}
`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries when message is empty, got %d", len(entries))
	}
}

func TestParseSessionReader_InvalidTimestampFallback(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"assistant","sessionId":"s1","timestamp":"not-a-timestamp","cwd":"/ws","message":{"model":"claude-opus-4-6","usage":{"input_tokens":10,"output_tokens":5}}}
`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	// When timestamp parsing fails, it falls back to time.Now().
	if entries[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp fallback")
	}
	// Should be close to now (within a few seconds).
	if time.Since(entries[0].Timestamp) > 10*time.Second {
		t.Errorf("timestamp too far from now: %v", entries[0].Timestamp)
	}
}

func TestParseSessionReader_BadMessageJSON(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":"not an object"}
`)
	entries, err := parseSessionReader(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries when message is not valid JSON object, got %d", len(entries))
	}
}

func TestParseSessionReader_LongLines(t *testing.T) {
	// Simulate a JSONL line with a very large message field (>1MB) to verify
	// the scanner buffer can handle large tool results or base64 content.
	padding := strings.Repeat("x", 2*1024*1024) // 2 MiB of padding
	line := `{"type":"assistant","sessionId":"s1","timestamp":"2026-03-01T10:00:00Z","cwd":"/ws","message":{"model":"claude-opus-4-6","usage":{"input_tokens":42,"output_tokens":7},"content":"` + padding + `"}}` + "\n"
	entries, err := parseSessionReader(strings.NewReader(line))
	if err != nil {
		t.Fatalf("expected no error for long line, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].InputTokens != 42 {
		t.Errorf("want 42 input tokens, got %d", entries[0].InputTokens)
	}
}

func TestPricingFor_Prefixes(t *testing.T) {
	cases := []struct {
		model     string
		wantInput float64
	}{
		// Opus 4.5+ tier ($5/$25)
		{"claude-opus-4-5-20251101", 5.00},
		{"claude-opus-4-6", 5.00},
		{"claude-opus-4-7", 5.00},
		{"claude-opus-4-8", 5.00},
		// Legacy Opus 4.0/4.1 tier ($15/$75)
		{"claude-opus-4-20250514", 15.00},
		{"claude-opus-4-1-20250805", 15.00},
		// Fable / Mythos 5 ($10/$50)
		{"claude-fable-5", 10.00},
		{"claude-mythos-5", 10.00},
		// Sonnet tier
		{"claude-sonnet-4-5-20250514", 3.00},
		{"claude-sonnet-4-6", 3.00},
		// Haiku 4.5 ($1/$5)
		{"claude-haiku-4-5-20251001", 1.00},
		// Claude 3.x legacy
		{"claude-3-5-sonnet-20241022", 3.00},
		{"claude-3-5-haiku-20241022", 0.80},
		{"claude-3-opus-20240229", 15.00},
	}
	for _, tc := range cases {
		p := PricingFor(tc.model)
		if p.InputPerM != tc.wantInput {
			t.Errorf("model %s: want input $%.2f/M, got $%.2f/M", tc.model, tc.wantInput, p.InputPerM)
		}
	}
}
