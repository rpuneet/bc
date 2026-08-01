// transcript_tailer.go — background capture of Live activity for providers
// that write a readable session transcript instead of invoking hooks.
//
// Hook-based providers (Claude, agy) POST lifecycle events to the daemon's
// /api/agents/{name}/hook endpoint, which flow into the Live feed via
// AgentService.IngestHookEvent. Providers in ActivityModeTranscript (e.g. pi)
// have no such push channel — they write a JSONL session log on disk. This
// collector tails those logs, parses newly-appended lines into the same hook
// events (via provider.TranscriptParser), and ingests them through the exact
// same path, so both kinds of provider feed one Live feed with no parallel UI.
package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
)

const (
	// transcriptTailInterval is how often the tailer polls for new transcript
	// content. Live enough for a feed without hammering the filesystem.
	transcriptTailInterval = 2 * time.Second
	// transcriptMaxReadPerTick caps the bytes read from one file per tick so a
	// large backfill can't stall the loop; the remainder is picked up next tick.
	transcriptMaxReadPerTick = 512 * 1024
	// transcriptPromptTaskMax bounds the prompt text mirrored into the agent's
	// task field on a user turn.
	transcriptPromptTaskMax = 120
)

// tailCursor tracks how far the tailer has consumed one transcript file.
type tailCursor struct {
	path   string
	offset int64
}

// runTranscriptTailer polls transcript-mode agents and ingests newly-written
// session activity into the Live feed. It returns when ctx is canceled.
func runTranscriptTailer(ctx context.Context, agents *agentpkg.AgentService) {
	if agents == nil {
		return
	}
	ticker := time.NewTicker(transcriptTailInterval)
	defer ticker.Stop()

	// One cursor per agent. A fresh cursor seeds at end-of-file so we capture
	// only activity produced while the daemon is running rather than replaying
	// a whole pre-existing session as if it happened now.
	cursors := make(map[string]*tailCursor)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tailTranscriptsOnce(ctx, agents, cursors)
		}
	}
}

// tailTranscriptsOnce runs a single sweep over all transcript-mode agents.
func tailTranscriptsOnce(ctx context.Context, agents *agentpkg.AgentService, cursors map[string]*tailCursor) {
	list, err := agents.List(ctx, agentpkg.ListOptions{})
	if err != nil {
		log.Debug("transcript tailer: agent list failed", "error", err)
		return
	}
	live := make(map[string]struct{}, len(list))
	for _, a := range list {
		live[a.Name] = struct{}{}
		parser, globs := transcriptParserFor(a)
		if parser == nil || len(globs) == 0 {
			continue
		}
		newest := newestMatch(globs)
		if newest == "" {
			continue
		}
		tailAgentFile(ctx, agents, cursors, a.Name, newest, parser)
	}
	// Drop cursors for agents that no longer exist so the map can't grow
	// unbounded across the daemon's lifetime.
	for name := range cursors {
		if _, ok := live[name]; !ok {
			delete(cursors, name)
		}
	}
}

// transcriptParserFor returns the transcript parser and glob patterns for an
// agent, or (nil, nil) when the agent's provider is not a parseable
// transcript source.
func transcriptParserFor(a *agentpkg.Agent) (provider.TranscriptParser, []string) {
	if a == nil || a.Tool == "" || a.WorktreeDir == "" {
		return nil, nil
	}
	p, ok := provider.DefaultRegistry.Get(a.Tool)
	if !ok {
		return nil, nil
	}
	src, ok := p.(provider.ActivitySource)
	if !ok || src.ActivityMode() != provider.ActivityModeTranscript {
		return nil, nil
	}
	parser, ok := p.(provider.TranscriptParser)
	if !ok {
		return nil, nil
	}
	return parser, src.TranscriptGlobs(a.WorktreeDir)
}

// newestMatch returns the most-recently-modified file matching any glob, or
// "" when none match.
func newestMatch(globs []string) string {
	var matches []string
	for _, g := range globs {
		m, err := filepath.Glob(g)
		if err != nil {
			continue
		}
		matches = append(matches, m...)
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return fileModTime(matches[i]).After(fileModTime(matches[j]))
	})
	return matches[0]
}

func fileModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// tailAgentFile reads newly-appended lines from an agent's current transcript
// and ingests each parsed activity into the Live feed.
func tailAgentFile(
	ctx context.Context,
	agents *agentpkg.AgentService,
	cursors map[string]*tailCursor,
	name, path string,
	parser provider.TranscriptParser,
) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	size := fi.Size()

	cur, ok := cursors[name]
	switch {
	case !ok:
		// First sighting for this agent: start at EOF so we don't backfill a
		// pre-existing session's history as if it just happened.
		cursors[name] = &tailCursor{path: path, offset: size}
		return
	case cur.path != path:
		// A newer session file appeared: capture it from the start.
		cur.path = path
		cur.offset = 0
	case cur.offset > size:
		// File was truncated/rotated in place: restart from the top.
		cur.offset = 0
	}

	if cur.offset >= size {
		return
	}

	lines, consumed := readNewLines(path, cur.offset, size)
	cur.offset += consumed
	if len(lines) == 0 {
		return
	}

	for _, line := range lines {
		acts, perr := parser.ParseTranscriptLine(line)
		if perr != nil || len(acts) == 0 {
			continue
		}
		for i := range acts {
			ingestTranscriptActivity(ctx, agents, name, &acts[i])
		}
	}
}

// readNewLines reads complete newline-terminated lines from path in
// [offset, size), capped at transcriptMaxReadPerTick bytes. It returns the
// parsed lines and how many bytes were consumed (up to and including the last
// newline), leaving any trailing partial line for the next tick.
func readNewLines(path string, offset, size int64) ([][]byte, int64) {
	f, err := os.Open(path) //nolint:gosec // path comes from provider glob of the user's own session dir
	if err != nil {
		return nil, 0
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // read-only handle

	toRead := size - offset
	if toRead > transcriptMaxReadPerTick {
		toRead = transcriptMaxReadPerTick
	}
	buf := make([]byte, toRead)
	n, err := f.ReadAt(buf, offset)
	if n == 0 && err != nil {
		return nil, 0
	}
	buf = buf[:n]

	// Only consume up to the final newline so a half-written line is retried.
	lastNL := lastIndexByte(buf, '\n')
	if lastNL < 0 {
		return nil, 0
	}
	consumed := int64(lastNL + 1)

	var lines [][]byte
	start := 0
	for i := 0; i <= lastNL; i++ {
		if buf[i] == '\n' {
			if i > start {
				line := make([]byte, i-start)
				copy(line, buf[start:i])
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	return lines, consumed
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// ingestTranscriptActivity builds a hook payload from one parsed activity and
// feeds it through the shared IngestHookEvent path (state transition + event
// log + WebSocket/SSE broadcast), exactly like an HTTP hook.
func ingestTranscriptActivity(ctx context.Context, agents *agentpkg.AgentService, name string, act *provider.TranscriptActivity) {
	m := map[string]any{"event": act.Event}
	if act.ToolName != "" {
		m["tool_name"] = act.ToolName
	}
	if act.ToolInput != nil {
		m["tool_input"] = act.ToolInput
	}
	if act.ToolResponse != nil {
		m["tool_response"] = act.ToolResponse
	}
	if act.Prompt != "" {
		m["prompt"] = act.Prompt
		m["task"] = truncateRunes(act.Prompt, transcriptPromptTaskMax)
	}
	if act.Error != "" {
		m["error"] = act.Error
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return
	}
	var payload agentpkg.HookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}

	if err := agents.IngestHookEvent(ctx, name, payload, raw); err != nil {
		// Unknown events and skipped state transitions are expected and benign
		// (e.g. the agent stopped between listing and ingest); log at debug.
		log.Debug("transcript tailer: ingest failed", "agent", name, "event", act.Event, "error", err)
	}
}

// truncateRunes truncates s to at most max runes, appending an ellipsis when
// it was shortened.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
