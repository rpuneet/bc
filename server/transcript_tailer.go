// transcript_tailer.go — background capture of Live activity for providers
// that write a readable session transcript instead of invoking hooks.
//
// Hook-based providers (Claude, agy) POST lifecycle events to the daemon's
// /api/agents/{name}/hook endpoint, which flow into the Live feed via
// AgentService.IngestHookEvent. Providers in ActivityModeTranscript (pi, codex)
// have no such push channel — they write a JSONL session log on disk. This
// collector tails those logs, parses newly-appended lines into the same hook
// events, and ingests them through the exact same path, so both kinds of
// provider feed one Live feed with no parallel UI.
//
// A provider's transcript is turned into events by either a stateless
// provider.TranscriptParser (pi — each line self-describing) or a stateful
// provider.TranscriptSession created per file (codex — its result lines
// reference an earlier call by id and carry no tool name). The file to follow is
// located by a cwd-encoded path glob (pi) or a provider.TranscriptFileSelector
// that reads the cwd out of the file (codex).
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
)

// tailCursor tracks how far the tailer has consumed one transcript file.
// session holds per-file parse state for session-based providers (codex) and is
// nil for stateless ones (pi); it is recreated whenever the followed file
// rotates so state never leaks across sessions.
type tailCursor struct {
	session provider.TranscriptSession
	path    string
	offset  int64
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
		p := transcriptProviderFor(a)
		if p == nil {
			continue
		}
		file := resolveTranscriptFile(p, a.WorktreeDir)
		if file == "" {
			continue
		}
		tailAgentFile(ctx, agents, cursors, a.Name, file, p)
	}
	// Drop cursors for agents that no longer exist so the map can't grow
	// unbounded across the daemon's lifetime.
	for name := range cursors {
		if _, ok := live[name]; !ok {
			delete(cursors, name)
		}
	}
}

// transcriptProviderFor returns the agent's provider when it is a parseable
// transcript source (ActivityModeTranscript with a stateless TranscriptParser or
// a stateful TranscriptSessionParser), or nil otherwise.
func transcriptProviderFor(a *agentpkg.Agent) provider.Provider {
	if a == nil || a.Tool == "" || a.WorktreeDir == "" {
		return nil
	}
	p, ok := provider.DefaultRegistry.Get(a.Tool)
	if !ok {
		return nil
	}
	src, ok := p.(provider.ActivitySource)
	if !ok || src.ActivityMode() != provider.ActivityModeTranscript {
		return nil
	}
	_, stateless := p.(provider.TranscriptParser)
	_, session := p.(provider.TranscriptSessionParser)
	if !stateless && !session {
		return nil
	}
	return p
}

// resolveTranscriptFile locates the transcript file to tail for a provider whose
// agent works in cwd. Providers that record cwd inside the file implement
// TranscriptFileSelector (codex); the rest are located by a cwd-encoded path
// glob (pi).
func resolveTranscriptFile(p provider.Provider, cwd string) string {
	if sel, ok := p.(provider.TranscriptFileSelector); ok {
		return sel.SelectTranscript(cwd)
	}
	if src, ok := p.(provider.ActivitySource); ok {
		return newestMatch(src.TranscriptGlobs(cwd))
	}
	return ""
}

// newTranscriptSession returns a fresh per-file session for a session-based
// provider, or nil for a stateless one.
func newTranscriptSession(p provider.Provider) provider.TranscriptSession {
	if sp, ok := p.(provider.TranscriptSessionParser); ok {
		return sp.NewTranscriptSession()
	}
	return nil
}

// parseTranscriptLine dispatches one line to the cursor's per-file session
// (session-based providers) or the provider's stateless parser.
func parseTranscriptLine(cur *tailCursor, p provider.Provider, line []byte) ([]provider.TranscriptActivity, error) {
	if cur.session != nil {
		return cur.session.ParseLine(line)
	}
	if sp, ok := p.(provider.TranscriptParser); ok {
		return sp.ParseTranscriptLine(line)
	}
	return nil, nil
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
	p provider.Provider,
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
		cursors[name] = &tailCursor{path: path, offset: size, session: newTranscriptSession(p)}
		return
	case cur.path != path:
		// A newer session file appeared: capture it from the start with fresh
		// per-file state.
		cur.path = path
		cur.offset = 0
		cur.session = newTranscriptSession(p)
	case cur.offset > size:
		// File was truncated/rotated in place: restart from the top and reset
		// per-file state.
		cur.offset = 0
		cur.session = newTranscriptSession(p)
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
		acts, perr := parseTranscriptLine(cur, p, line)
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
	// Only the prompt is forwarded. Deriving the task line from it is
	// IngestHookEvent's job, so transcript-mode and hooks-mode providers get an
	// identical task line from identical input — and setting "task" here would
	// have had no effect anyway, since ingest reads the prompt.
	if act.Prompt != "" {
		m["prompt"] = act.Prompt
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
