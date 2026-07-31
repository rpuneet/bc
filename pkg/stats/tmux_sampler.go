// tmux_sampler.go — sample CPU/memory for tmux-backed agents.
//
// Docker-backed agents are sampled via `docker stats`. Tmux-backed agents
// live as plain processes under a tmux session, so we walk the PID tree
// from the tmux pane PID down through its descendants (shell → claude →
// any subprocesses) and sum %CPU + RSS.
//
// Network bytes are not populated here: per-process network accounting
// requires either DTrace (darwin) or cgroup access (linux container
// namespace only). The collector leaves those fields zero and the UI
// renders "Network tracking requires container runtime".
package stats

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// TmuxProcRunner abstracts the subprocess calls used by TmuxSampler so
// tests can inject a fake without needing a real tmux server.
type TmuxProcRunner interface {
	// PanePIDs returns the pane PIDs for the given tmux session. The
	// session name is resolved by the caller (e.g. `mycel-<hash>-<agent>`).
	// Returns an empty slice (and nil error) when the session does not
	// exist — callers treat that as "agent not running, record 0".
	PanePIDs(ctx context.Context, session string) ([]int, error)
	// ListSessions returns the list of live tmux session names. Used to
	// resolve the workspace-hashed session name from a bare agent name.
	ListSessions(ctx context.Context) ([]string, error)
	// Children returns the direct child PIDs of the given PID.
	Children(ctx context.Context, pid int) ([]int, error)
	// PSStats returns the (%cpu, rssBytes) pair for each supplied PID.
	// Missing PIDs are silently skipped (they may have exited between
	// the walk and the ps call).
	PSStats(ctx context.Context, pids []int) (cpuPercent float64, rssBytes int64, err error)
}

// TmuxSampler samples CPU/memory for a tmux-backed agent via its PID tree.
//
// Zero value is not usable; use NewTmuxSampler.
type TmuxSampler struct {
	runner TmuxProcRunner
	// warnNetOnce logs the "no per-process network stats" warning at
	// most once per sampler instance so we don't spam the journal.
	warnNetOnce sync.Once
	// maxDepth caps how deep we walk descendants. Claude typically
	// lives 2–3 levels under the pane shell; 6 is ample headroom
	// without risking infinite loops if a runner fakes a cycle.
	maxDepth int
}

// NewTmuxSampler constructs a sampler. Pass DefaultTmuxProcRunner for
// production or a mock for tests.
func NewTmuxSampler(runner TmuxProcRunner) *TmuxSampler {
	return &TmuxSampler{runner: runner, maxDepth: 6}
}

// TmuxSample is the result of sampling a tmux agent.
type TmuxSample struct {
	CPUPercent float64
	MemBytes   int64
	// PIDsWalked is the count of descendant PIDs included in the sum;
	// useful for debug logging and tests.
	PIDsWalked int
}

// Sample walks the PID tree rooted at the tmux panes for `session`
// (trying both the literal name and any name containing `agentName` if
// the literal miss). It returns a zero sample (no error) when the
// session has no panes — stopped agents should read 0, not error.
func (s *TmuxSampler) Sample(ctx context.Context, session, agentName string) (TmuxSample, error) {
	if s == nil || s.runner == nil {
		return TmuxSample{}, fmt.Errorf("tmux sampler not initialized")
	}

	panePIDs, err := s.runner.PanePIDs(ctx, session)
	if err != nil || len(panePIDs) == 0 {
		// Retry via list-sessions: the caller may have passed the
		// bare agent name while the real session is `mycel-<hash>-<name>`.
		sessions, lsErr := s.runner.ListSessions(ctx)
		if lsErr != nil {
			return TmuxSample{}, nil //nolint:nilerr // no session = 0, not an error
		}
		var resolved string
		for _, s2 := range sessions {
			if s2 == session || strings.Contains(s2, agentName) || strings.Contains(s2, session) {
				resolved = s2
				break
			}
		}
		if resolved == "" {
			return TmuxSample{}, nil
		}
		panePIDs, err = s.runner.PanePIDs(ctx, resolved)
		if err != nil || len(panePIDs) == 0 {
			return TmuxSample{}, nil //nolint:nilerr
		}
	}

	// BFS walk descendants. Dedup visited PIDs to survive runner bugs.
	visited := make(map[int]struct{}, len(panePIDs)*4)
	queue := append([]int(nil), panePIDs...)
	for _, p := range panePIDs {
		visited[p] = struct{}{}
	}
	depth := 0
	for len(queue) > 0 && depth < s.maxDepth {
		next := make([]int, 0, len(queue))
		for _, pid := range queue {
			kids, kerr := s.runner.Children(ctx, pid)
			if kerr != nil {
				continue
			}
			for _, k := range kids {
				if _, seen := visited[k]; seen {
					continue
				}
				visited[k] = struct{}{}
				next = append(next, k)
			}
		}
		queue = next
		depth++
	}

	// Exclude the tmux pane shell itself (first-level pane PIDs) from
	// the CPU sum? We keep it — `docker stats` includes the shell too,
	// so parity is better than surgical exclusion and the shell is
	// near-zero CPU when idle.
	pids := make([]int, 0, len(visited))
	for pid := range visited {
		pids = append(pids, pid)
	}

	cpu, rss, err := s.runner.PSStats(ctx, pids)
	if err != nil {
		return TmuxSample{PIDsWalked: len(pids)}, err
	}
	return TmuxSample{CPUPercent: cpu, MemBytes: rss, PIDsWalked: len(pids)}, nil
}

// WarnNoNetworkOnce invokes `log` exactly once per sampler instance.
// The server wires this to pkg/log so we avoid an import cycle.
func (s *TmuxSampler) WarnNoNetworkOnce(log func()) {
	if s == nil || log == nil {
		return
	}
	s.warnNetOnce.Do(log)
}

// ── Default runner (real subprocesses) ─────────────────────────────────

// DefaultTmuxProcRunner is the production runner that shells out to
// `tmux`, `pgrep`, and `ps`. Works on darwin + linux.
type DefaultTmuxProcRunner struct{}

// PanePIDs shells to `tmux list-panes -t <session> -F '#{pane_pid}'`.
func (DefaultTmuxProcRunner) PanePIDs(ctx context.Context, session string) ([]int, error) {
	if session == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "tmux", "list-panes", "-t", session, "-F", "#{pane_pid}") //nolint:gosec // session validated by caller
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePIDList(out), nil
}

// ListSessions shells to `tmux list-sessions -F '#{session_name}'`.
func (DefaultTmuxProcRunner) ListSessions(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// `no server running` is an expected non-error state.
		return nil, nil //nolint:nilerr
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// Children shells to `pgrep -P <pid>`. Works the same on darwin + linux.
// On linux we could read /proc/<pid>/task/<tid>/children for fewer forks,
// but pgrep is consistent and the sample interval (30s) makes the cost
// irrelevant.
func (DefaultTmuxProcRunner) Children(ctx context.Context, pid int) ([]int, error) {
	if pid <= 0 {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "pgrep", "-P", strconv.Itoa(pid)) //nolint:gosec // pid is int, not user-controlled
	out, err := cmd.Output()
	if err != nil {
		// pgrep exits 1 when no matches — that's "no children", not an error.
		return nil, nil //nolint:nilerr
	}
	return parsePIDList(out), nil
}

// PSStats shells to `ps -p <csv> -o pid,%cpu,rss`. darwin and linux both
// accept this format. RSS is in kibibytes on both platforms.
func (DefaultTmuxProcRunner) PSStats(ctx context.Context, pids []int) (float64, int64, error) {
	if len(pids) == 0 {
		return 0, 0, nil
	}
	parts := make([]string, 0, len(pids))
	for _, p := range pids {
		if p > 0 {
			parts = append(parts, strconv.Itoa(p))
		}
	}
	if len(parts) == 0 {
		return 0, 0, nil
	}
	cmd := exec.CommandContext(ctx, "ps", "-p", strings.Join(parts, ","), "-o", "pid,%cpu,rss") //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		// ps exits non-zero when none of the supplied PIDs exist — treat as 0.
		return 0, 0, nil //nolint:nilerr
	}
	var totalCPU float64
	var totalRSSKB int64
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Scan() // header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if cpu, perr := strconv.ParseFloat(fields[1], 64); perr == nil {
			totalCPU += cpu
		}
		if rss, perr := strconv.ParseInt(fields[2], 10, 64); perr == nil {
			totalRSSKB += rss
		}
	}
	return totalCPU, totalRSSKB * 1024, nil
}

// parsePIDList splits `ps`/`pgrep` output into a slice of ints.
func parsePIDList(out []byte) []int {
	var pids []int
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// When the line has multiple fields (e.g. `ps -o pid,%cpu` header
		// rows mixed in), take the first.
		if idx := strings.IndexAny(line, " \t"); idx > 0 {
			line = line[:idx]
		}
		if v, err := strconv.Atoi(line); err == nil && v > 0 {
			pids = append(pids, v)
		}
	}
	return pids
}
