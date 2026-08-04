package provider

// cursor_costs.go — CostReader for Cursor Agent.
//
// Cursor does not write token usage into its agent-transcript JSONL (those
// files are conversation turns only). It *does* put input/output/cache token
// counts on the stop-hook payload the mycel reporter already POSTs. Those
// counts are persisted under:
//
//	<AgentsDir>/<name>/session/cursor/usage.jsonl
//
// by the hook ingestion path, one line per completed turn. ReadCosts prices
// each line the same way Claude's reader does for its session files
// (cost.CostBasisPriced — local model rate tables, not Cursor billing).
//
// Scope note: this is mycel-agent usage only. Cursor's own account Usage
// dashboard counts every Cursor surface (IDE chat, Tab, Cloud, etc.) and
// will not match these totals; dollars here will also not match Cursor
// Spend / invoices until a CostBasisBilled reader exists.

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CursorUsageRelDir is the session subdirectory that holds Cursor usage
// records written from stop-hook payloads.
const CursorUsageRelDir = "session/cursor"

// CursorUsageFile is the JSONL filename under CursorUsageRelDir.
const CursorUsageFile = "usage.jsonl"

// CursorUsageRecord is one turn's usage as stored on disk and as read back
// by ReadCosts. Field names match the stop-hook payload where possible.
type CursorUsageRecord struct {
	Timestamp        time.Time `json:"timestamp"`
	Model            string    `json:"model"`
	SessionID        string    `json:"session_id,omitempty"`
	GenerationID     string    `json:"generation_id,omitempty"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens"`
	CacheWriteTokens int64     `json:"cache_write_tokens"`
	CostUSD          float64   `json:"cost_usd"`
}

// CursorCalcCost returns the USD cost for the given token counts and model.
// Claude-named models reuse Claude pricing; everything else uses the Cursor
// table below. Unknown models get a conservative mid-tier default so a
// figure is never silently zero when tokens were reported.
func CursorCalcCost(model string, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) float64 {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(m, "claude-") {
		return ClaudeCalcCost(model, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens)
	}
	p := cursorPricingFor(m)
	const perM = 1_000_000.0
	return float64(inputTokens)*p.InputPerM/perM +
		float64(outputTokens)*p.OutputPerM/perM +
		float64(cacheWriteTokens)*p.CacheWritePerM/perM +
		float64(cacheReadTokens)*p.CacheReadPerM/perM
}

// cursorModelPrices is walked in order; more specific prefixes first.
// Rates are USD per 1M tokens. Cursor's "default" and composer-family
// models do not publish a stable public table the way Anthropic does, so
// these are best-effort and should be revisited when Cursor documents
// them. Prefer wrong-but-visible over $0.
var cursorModelPrices = []struct {
	prefix  string
	pricing ModelPricing
}{
	// Cursor composer / default agent models (approximate).
	{"composer", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	{"kimi", ModelPricing{InputPerM: 0.60, OutputPerM: 2.50, CacheWritePerM: 0.75, CacheReadPerM: 0.06}},
	{"gpt-5", ModelPricing{InputPerM: 1.25, OutputPerM: 10.00, CacheWritePerM: 1.25, CacheReadPerM: 0.125}},
	{"gpt-4.1", ModelPricing{InputPerM: 2.00, OutputPerM: 8.00, CacheWritePerM: 2.00, CacheReadPerM: 0.50}},
	{"gpt-4o", ModelPricing{InputPerM: 2.50, OutputPerM: 10.00, CacheWritePerM: 2.50, CacheReadPerM: 1.25}},
	{"o3", ModelPricing{InputPerM: 2.00, OutputPerM: 8.00, CacheWritePerM: 2.00, CacheReadPerM: 0.50}},
	{"o4", ModelPricing{InputPerM: 1.10, OutputPerM: 4.40, CacheWritePerM: 1.10, CacheReadPerM: 0.275}},
}

var defaultCursorPricing = ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}

func cursorPricingFor(model string) ModelPricing {
	if model == "" || model == "default" {
		return defaultCursorPricing
	}
	for _, p := range cursorModelPrices {
		if strings.HasPrefix(model, p.prefix) {
			return p.pricing
		}
	}
	return defaultCursorPricing
}

// AppendCursorUsage writes one priced usage record for agent under
// AgentsDir. Called from hook ingestion when a Stop payload carries
// token counts. Empty AgentsDir or a record with no tokens is a no-op.
func AppendCursorUsage(agentsDir, agent string, rec CursorUsageRecord) error {
	if agentsDir == "" || agent == "" {
		return nil
	}
	if rec.InputTokens == 0 && rec.OutputTokens == 0 &&
		rec.CacheReadTokens == 0 && rec.CacheWriteTokens == 0 {
		return nil
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.CostUSD == 0 {
		rec.CostUSD = CursorCalcCost(rec.Model, rec.InputTokens, rec.OutputTokens, rec.CacheWriteTokens, rec.CacheReadTokens)
	}
	dir := filepath.Join(agentsDir, agent, CursorUsageRelDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(dir, CursorUsageFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path under mycel home
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// ReadCosts implements CostReader for Cursor Agent.
func (p *CursorProvider) ReadCosts(ctx context.Context, opts CostReadOptions) ([]CostEntry, error) {
	if opts.AgentsDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(opts.AgentsDir)
	if err != nil {
		return nil, nil // best-effort source
	}

	seen := make(map[string]struct{})
	var out []CostEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		path := filepath.Join(opts.AgentsDir, e.Name(), CursorUsageRelDir, CursorUsageFile)
		recs, readErr := readCursorUsageFile(path)
		if readErr != nil {
			continue
		}
		for _, rec := range recs {
			if !opts.Since.IsZero() && rec.Timestamp.Before(opts.Since) {
				continue
			}
			key := rec.GenerationID
			if key == "" {
				key = rec.SessionID + "|" + rec.Timestamp.UTC().Format(time.RFC3339Nano) + "|" +
					strconv.FormatInt(rec.InputTokens, 10) + "|" + strconv.FormatInt(rec.OutputTokens, 10)
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			cost := rec.CostUSD
			if cost == 0 {
				cost = CursorCalcCost(rec.Model, rec.InputTokens, rec.OutputTokens, rec.CacheWriteTokens, rec.CacheReadTokens)
			}
			out = append(out, CostEntry{
				Timestamp:        rec.Timestamp,
				Agent:            e.Name(),
				Model:            rec.Model,
				SessionID:        rec.SessionID,
				InputTokens:      rec.InputTokens,
				OutputTokens:     rec.OutputTokens,
				CacheReadTokens:  rec.CacheReadTokens,
				CacheWriteTokens: rec.CacheWriteTokens,
				CostUSD:          cost,
			})
		}
	}
	return out, nil
}

func readCursorUsageFile(path string) ([]CursorUsageRecord, error) {
	f, err := os.Open(path) //nolint:gosec // path under mycel agents dir
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var out []CursorUsageRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec CursorUsageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

// Ensure CursorProvider implements CostReader.
var _ CostReader = (*CursorProvider)(nil)
