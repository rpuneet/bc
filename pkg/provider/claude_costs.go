package provider

// claude_costs.go — the Claude Code CostReader.
//
// Claude Code writes JSONL session transcripts containing per-message
// token usage. ReadCosts parses them directly (source-direct — no
// ledger) from two locations:
//
//   - <Home>/.claude/projects/**.jsonl        (host/tmux sessions)
//   - <AgentsDir>/<name>/session/claude/projects/**.jsonl
//     (docker agents; the session dir is bind-mounted into the
//     container as ~/.claude)
//
// Pricing is applied from the model table below at read time.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelPricing holds per-token pricing for a Claude model in USD per 1M tokens.
type ModelPricing struct {
	InputPerM      float64
	OutputPerM     float64
	CacheWritePerM float64 // cache_creation_input_tokens
	CacheReadPerM  float64 // cache_read_input_tokens
}

// claudeModelPrices maps model ID prefixes to pricing. Prefix matching is
// used so minor version variants (e.g. claude-opus-4-6) hit the right
// tier. More specific prefixes MUST come before shorter ones (walked in
// order).
//
// Cache pricing follows the standard Anthropic ratios: cache write (5m
// TTL) = 1.25x input, cache read = 0.1x input.
var claudeModelPrices = []struct {
	prefix  string
	pricing ModelPricing
}{
	// Claude Fable 5 / Mythos 5
	{"claude-fable-5", ModelPricing{InputPerM: 10.00, OutputPerM: 50.00, CacheWritePerM: 12.50, CacheReadPerM: 1.00}},
	{"claude-mythos-5", ModelPricing{InputPerM: 10.00, OutputPerM: 50.00, CacheWritePerM: 12.50, CacheReadPerM: 1.00}},
	// Claude Opus 4.5 and later dropped to $5/$25 (before generic claude-opus-4)
	{"claude-opus-4-5", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-6", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-7", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-8", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	// Claude Opus 4.0 / 4.1 (legacy $15/$75 tier)
	{"claude-opus-4", ModelPricing{InputPerM: 15.00, OutputPerM: 75.00, CacheWritePerM: 18.75, CacheReadPerM: 1.50}},
	// Claude 4 Sonnet / 4.5 / 4.6 Sonnet
	{"claude-sonnet-4", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.7 Sonnet
	{"claude-3-7-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.5 Sonnet
	{"claude-3-5-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.5 Haiku
	{"claude-3-5-haiku", ModelPricing{InputPerM: 0.80, OutputPerM: 4.00, CacheWritePerM: 1.00, CacheReadPerM: 0.08}},
	// Claude Haiku 4.5
	{"claude-haiku-4", ModelPricing{InputPerM: 1.00, OutputPerM: 5.00, CacheWritePerM: 1.25, CacheReadPerM: 0.10}},
	// Claude 3 Opus
	{"claude-3-opus", ModelPricing{InputPerM: 15.00, OutputPerM: 75.00, CacheWritePerM: 18.75, CacheReadPerM: 1.50}},
	// Claude 3 Sonnet
	{"claude-3-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3 Haiku
	{"claude-3-haiku", ModelPricing{InputPerM: 0.25, OutputPerM: 1.25, CacheWritePerM: 0.30, CacheReadPerM: 0.03}},
}

// defaultClaudePricing is used when a model is not recognized.
var defaultClaudePricing = ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}

// ClaudePricingFor returns the pricing for a Claude model (matched by
// prefix walk).
func ClaudePricingFor(model string) ModelPricing {
	for _, p := range claudeModelPrices {
		if strings.HasPrefix(model, p.prefix) {
			return p.pricing
		}
	}
	return defaultClaudePricing
}

// ClaudeCalcCost returns the USD cost for the given token counts and model.
func ClaudeCalcCost(model string, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) float64 {
	p := ClaudePricingFor(model)
	const perM = 1_000_000.0
	return float64(inputTokens)*p.InputPerM/perM +
		float64(outputTokens)*p.OutputPerM/perM +
		float64(cacheWriteTokens)*p.CacheWritePerM/perM +
		float64(cacheReadTokens)*p.CacheReadPerM/perM
}

// claudeSessionEntry is a parsed assistant message from a Claude Code
// JSONL session file.
type claudeSessionEntry struct {
	Timestamp           time.Time
	SessionID           string
	Model               string
	CWD                 string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// claudeJSONLEvent is the minimal structure decoded from each JSONL line.
type claudeJSONLEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	CWD       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`
}

type claudeJSONLMessage struct {
	Model string           `json:"model"`
	Usage claudeJSONLUsage `json:"usage"`
}

type claudeJSONLUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// parseClaudeSessionFile reads a Claude Code JSONL session file and
// returns all assistant message entries that contain token usage.
func parseClaudeSessionFile(path string) ([]claudeSessionEntry, error) {
	f, err := os.Open(path) //nolint:gosec // path enumerated from trusted roots
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return parseClaudeSessionReader(f)
}

func parseClaudeSessionReader(r io.Reader) ([]claudeSessionEntry, error) {
	var entries []claudeSessionEntry
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // 10 MiB max line length

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var evt claudeJSONLEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue // skip malformed lines
		}
		if evt.Type != "assistant" || len(evt.Message) == 0 {
			continue
		}

		var msg claudeJSONLMessage
		if err := json.Unmarshal(evt.Message, &msg); err != nil {
			continue
		}
		// Only include entries that have actual token usage recorded.
		if msg.Usage.InputTokens == 0 && msg.Usage.OutputTokens == 0 &&
			msg.Usage.CacheCreationInputTokens == 0 && msg.Usage.CacheReadInputTokens == 0 {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, evt.Timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}

		entries = append(entries, claudeSessionEntry{
			Timestamp:           ts,
			SessionID:           evt.SessionID,
			Model:               msg.Model,
			CWD:                 evt.CWD,
			InputTokens:         msg.Usage.InputTokens,
			OutputTokens:        msg.Usage.OutputTokens,
			CacheCreationTokens: msg.Usage.CacheCreationInputTokens,
			CacheReadTokens:     msg.Usage.CacheReadInputTokens,
		})
	}
	return entries, scanner.Err()
}

// findClaudeSessionFiles returns all .jsonl session file paths under the
// given Claude projects root directory.
func findClaudeSessionFiles(claudeProjectsDir string) []string {
	var files []string
	_ = filepath.WalkDir(claudeProjectsDir, func(path string, d fs.DirEntry, err error) error { //nolint:errcheck // inaccessible dirs are skipped
		if err != nil {
			return nil // skip inaccessible directories
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") && d.Name() != "history.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	return files
}

// claudeCostRoot pairs a projects directory with the agent name its
// entries belong to ("" = resolve from the session CWD).
type claudeCostRoot struct {
	dir   string
	agent string
}

// ReadCosts implements CostReader for Claude Code. It scans the host
// projects dir and every agent session dir, prices each usage entry,
// and dedups identical usage records that appear in more than one file
// (Claude Code writes compaction sidechain files that replicate the
// parent session's assistant messages verbatim).
func (p *ClaudeProvider) ReadCosts(ctx context.Context, opts CostReadOptions) ([]CostEntry, error) {
	var roots []claudeCostRoot

	if opts.Home != "" {
		roots = append(roots, claudeCostRoot{dir: filepath.Join(opts.Home, ".claude", "projects")})
	}
	if opts.AgentsDir != "" {
		entries, err := os.ReadDir(opts.AgentsDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				roots = append(roots, claudeCostRoot{
					dir:   filepath.Join(opts.AgentsDir, e.Name(), "session", "claude", "projects"),
					agent: e.Name(),
				})
			}
		}
	}

	seen := make(map[claudeUsageKey]struct{})
	var out []CostEntry
	for _, root := range roots {
		if _, err := os.Stat(root.dir); err != nil {
			continue
		}
		for _, file := range findClaudeSessionFiles(root.dir) {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			entries, err := parseClaudeSessionFile(file)
			if err != nil {
				continue // unreadable file — skip, sources are best-effort
			}
			for _, e := range entries {
				if !opts.Since.IsZero() && e.Timestamp.Before(opts.Since) {
					continue
				}
				key := claudeUsageKey{
					session: e.SessionID,
					ts:      e.Timestamp.UnixNano(),
					model:   e.Model,
					in:      e.InputTokens,
					out:     e.OutputTokens,
					cacheW:  e.CacheCreationTokens,
					cacheR:  e.CacheReadTokens,
				}
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}

				out = append(out, CostEntry{
					Timestamp:        e.Timestamp,
					Agent:            resolveClaudeAgent(root.agent, e.CWD, opts.AgentsDir),
					Repo:             e.CWD,
					Model:            e.Model,
					SessionID:        e.SessionID,
					InputTokens:      e.InputTokens,
					OutputTokens:     e.OutputTokens,
					CacheReadTokens:  e.CacheReadTokens,
					CacheWriteTokens: e.CacheCreationTokens,
					CostUSD:          ClaudeCalcCost(e.Model, e.InputTokens, e.OutputTokens, e.CacheCreationTokens, e.CacheReadTokens),
				})
			}
		}
	}
	return out, nil
}

// claudeUsageKey dedups identical usage entries across files.
type claudeUsageKey struct {
	session string
	model   string
	ts      int64
	in      int64
	out     int64
	cacheW  int64
	cacheR  int64
}

// resolveClaudeAgent attributes a session entry to a mycel agent. Docker
// agent roots carry the entity name directly. Host sessions are matched
// by CWD: a session running inside <agentsDir>/<name>/worktree belongs
// to <name>; anything else gets the CWD basename as a loose label.
func resolveClaudeAgent(rootAgent, cwd, agentsDir string) string {
	if rootAgent != "" {
		return rootAgent
	}
	if agentsDir != "" && cwd != "" {
		prefix := agentsDir + string(filepath.Separator)
		if strings.HasPrefix(cwd, prefix) {
			rest := strings.TrimPrefix(cwd, prefix)
			if name, _, ok := strings.Cut(rest, string(filepath.Separator)); ok && name != "" {
				return name
			} else if !ok && rest != "" {
				return rest
			}
		}
	}
	if cwd != "" {
		return filepath.Base(cwd)
	}
	return "unknown"
}

// Ensure ClaudeProvider implements CostReader.
var _ CostReader = (*ClaudeProvider)(nil)
