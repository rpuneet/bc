// Package token reads Claude Code session JSONL files from agent volume
// mounts and aggregates token usage per agent. It is independent of the
// cost package — no pricing, no USD, just raw token counts.
package token

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Usage represents aggregated token usage for a single agent.
type Usage struct {
	AgentName    string `json:"agent_name"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheCreate  int64  `json:"cache_create"`
	TotalTokens  int64  `json:"total_tokens"`
	Entries      int    `json:"entries"`
}

// Entry represents a single token usage event from a JSONL session file.
type Entry struct {
	Timestamp    time.Time
	AgentName    string
	SessionID    string
	Model        string
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheCreate  int64
}

// CollectAll scans all agent directories under agentsDir and returns
// aggregated token usage per agent. The expected layout is:
//
//	agentsDir/<agent>/session/claude/projects/*/*.jsonl
func CollectAll(agentsDir string) ([]Usage, error) {
	agents, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, err
	}

	var result []Usage
	for _, agent := range agents {
		if !agent.IsDir() {
			continue
		}
		name := agent.Name()
		projectsDir := filepath.Join(agentsDir, name, "session", "claude", "projects")
		if _, statErr := os.Stat(projectsDir); statErr != nil {
			continue
		}

		entries, scanErr := scanAgentSessions(name, projectsDir)
		if scanErr != nil || len(entries) == 0 {
			continue
		}

		// Aggregate by model
		byModel := make(map[string]*Usage)
		for _, e := range entries {
			model := e.Model
			if model == "" {
				model = "unknown"
			}
			u, ok := byModel[model]
			if !ok {
				u = &Usage{AgentName: name, Model: model}
				byModel[model] = u
			}
			u.InputTokens += e.InputTokens
			u.OutputTokens += e.OutputTokens
			u.CacheRead += e.CacheRead
			u.CacheCreate += e.CacheCreate
			u.TotalTokens += e.InputTokens + e.OutputTokens
			u.Entries++
		}

		for _, u := range byModel {
			result = append(result, *u)
		}
	}
	return result, nil
}

// CollectAgent returns token entries for a single agent, useful for
// timeseries recording where individual timestamps matter.
func CollectAgent(agentsDir, agentName string) ([]Entry, error) {
	projectsDir := filepath.Join(agentsDir, agentName, "claude", "projects")
	if _, err := os.Stat(projectsDir); err != nil {
		return nil, nil // no session data
	}
	return scanAgentSessions(agentName, projectsDir)
}

// CollectAgentSince returns only entries after the given timestamp.
func CollectAgentSince(agentsDir, agentName string, since time.Time) ([]Entry, error) {
	all, err := CollectAgent(agentsDir, agentName)
	if err != nil {
		return nil, err
	}
	var filtered []Entry
	for _, e := range all {
		if e.Timestamp.After(since) {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

// scanAgentSessions walks the projects directory for JSONL files and parses them.
func scanAgentSessions(agentName, projectsDir string) ([]Entry, error) {
	var files []string
	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip inaccessible
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") && d.Name() != "history.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, f := range files {
		parsed, parseErr := parseSessionFile(agentName, f)
		if parseErr != nil {
			continue // skip malformed files
		}
		entries = append(entries, parsed...)
	}
	return entries, nil
}

// JSONL structures — minimal decode of Claude Code session format.
type jsonlEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type jsonlMessage struct {
	Model string     `json:"model"`
	Usage jsonlUsage `json:"usage"`
}

type jsonlUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheCreate  int64 `json:"cache_creation_input_tokens"`
	CacheRead    int64 `json:"cache_read_input_tokens"`
}

func parseSessionFile(agentName, path string) ([]Entry, error) {
	f, err := os.Open(path) //nolint:gosec // path from the mycel home
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var evt jsonlEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type != "assistant" || len(evt.Message) == 0 {
			continue
		}

		var msg jsonlMessage
		if err := json.Unmarshal(evt.Message, &msg); err != nil {
			continue
		}
		if msg.Usage.InputTokens == 0 && msg.Usage.OutputTokens == 0 &&
			msg.Usage.CacheCreate == 0 && msg.Usage.CacheRead == 0 {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, evt.Timestamp)
		if ts.IsZero() {
			continue // skip entries with unparseable timestamps
		}

		entries = append(entries, Entry{
			Timestamp:    ts,
			AgentName:    agentName,
			SessionID:    evt.SessionID,
			Model:        msg.Model,
			InputTokens:  msg.Usage.InputTokens,
			OutputTokens: msg.Usage.OutputTokens,
			CacheRead:    msg.Usage.CacheRead,
			CacheCreate:  msg.Usage.CacheCreate,
		})
	}
	return entries, scanner.Err()
}
