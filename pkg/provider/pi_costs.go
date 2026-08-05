package provider

// pi_costs.go — CostReader for pi (including Amazon Bedrock models).
//
// pi embeds token counts and a priced usage.cost.total on every assistant
// message in its session JSONL under ~/.pi/agent/sessions (or
// PI_CODING_AGENT_SESSION_DIR). Budgets and MaxCostUSD guardrails need those
// figures in pkg/cost; without a CostReader they stay at $0 (#3630).
//
// Prefer pi's own cost.total when present (already priced for the model /
// provider, including Bedrock). Fall back to ClaudeCalcCost only when the
// model looks like a Claude id and cost is missing.

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// piCostLine is the subset of a pi JSONL entry needed for cost attribution.
type piCostLine struct {
	Message   *piCostMessage `json:"message,omitempty"`
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	ID        string         `json:"id,omitempty"`
	CWD       string         `json:"cwd,omitempty"`
}

type piCostMessage struct {
	Usage    *piCostUsage `json:"usage,omitempty"`
	Role     string       `json:"role"`
	Provider string       `json:"provider,omitempty"`
	Model    string       `json:"model,omitempty"`
}

type piCostUsage struct {
	Cost       *piCostUSD `json:"cost,omitempty"`
	Input      int64      `json:"input"`
	Output     int64      `json:"output"`
	CacheRead  int64      `json:"cacheRead"`
	CacheWrite int64      `json:"cacheWrite"`
}

type piCostUSD struct {
	Total float64 `json:"total"`
}

// ReadCosts implements CostReader for pi. It walks pi's session tree and
// emits one CostEntry per assistant turn that reports usage.
//
// When opts.Home is set (daemon / tests), sessions are read from
// <Home>/.pi/agent/sessions so a throwaway CostReadOptions.Home cannot
// leak the developer's real ~/.pi tree into cost fixtures (#3011).
// PI_CODING_AGENT_SESSION_DIR still wins when set.
func (p *PiProvider) ReadCosts(ctx context.Context, opts CostReadOptions) ([]CostEntry, error) {
	root := ""
	if dir := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); dir != "" {
		root = dir
	} else if opts.Home != "" {
		root = filepath.Join(opts.Home, ".pi", "agent", "sessions")
	} else {
		root = piSessionsRoot()
	}
	if root == "" {
		return nil, nil
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, nil // best-effort source
	}

	var out []CostEntry
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		dir := filepath.Join(root, e.Name())
		files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if err != nil {
			continue
		}
		for _, path := range files {
			entries, readErr := readPiCostFile(path, opts)
			if readErr != nil {
				continue
			}
			out = append(out, entries...)
		}
	}
	return out, nil
}

func readPiCostFile(path string, opts CostReadOptions) ([]CostEntry, error) {
	f, err := os.Open(path) //nolint:gosec // path under pi sessions dir
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var (
		sessionID string
		cwd       string
		out       []CostEntry
	)
	// Session id is also encoded in the filename: <ts>_<uuid>.jsonl
	base := filepath.Base(path)
	if i := strings.Index(base, "_"); i >= 0 {
		sessionID = strings.TrimSuffix(base[i+1:], ".jsonl")
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec piCostLine
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		switch rec.Type {
		case "session":
			if rec.ID != "" {
				sessionID = rec.ID
			}
			if rec.CWD != "" {
				cwd = rec.CWD
			}
			continue
		case "message":
			// handled below
		default:
			continue
		}
		if rec.Message == nil || rec.Message.Role != "assistant" || rec.Message.Usage == nil {
			continue
		}
		u := rec.Message.Usage
		if u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheWrite == 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
		if err != nil {
			ts, err = time.Parse(time.RFC3339, rec.Timestamp)
			if err != nil {
				continue
			}
		}
		if !opts.Since.IsZero() && ts.Before(opts.Since) {
			continue
		}

		model := rec.Message.Model
		if rec.Message.Provider != "" && model != "" {
			model = rec.Message.Provider + "/" + model
		}
		cost := 0.0
		if u.Cost != nil {
			cost = u.Cost.Total
		}
		if cost == 0 {
			// Fall back for Claude-shaped ids when pi omitted pricing.
			bare := rec.Message.Model
			if i := strings.LastIndex(bare, "claude-"); i >= 0 {
				cost = ClaudeCalcCost(bare[i:], u.Input, u.Output, u.CacheWrite, u.CacheRead)
			}
		}

		out = append(out, CostEntry{
			Timestamp:        ts,
			Agent:            resolveClaudeAgent("", cwd, opts.AgentsDir),
			Repo:             cwd,
			Model:            model,
			SessionID:        sessionID,
			InputTokens:      u.Input,
			OutputTokens:     u.Output,
			CacheReadTokens:  u.CacheRead,
			CacheWriteTokens: u.CacheWrite,
			CostUSD:          cost,
		})
	}
	return out, sc.Err()
}

// Ensure PiProvider implements CostReader.
var _ CostReader = (*PiProvider)(nil)
