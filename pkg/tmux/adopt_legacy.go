package tmux

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
)

// legacySessionName matches a session named the way mycel named them when
// session names carried a hash of the repo the daemon booted in:
// "mycel-13c6e9-fast-crane". The hash is the first three bytes of a SHA-256,
// hex-encoded, so exactly six hex digits.
var legacySessionName = regexp.MustCompile(`^([0-9a-f]{6})-(.+)$`)

// AdoptLegacySessions renames sessions left over from repo-scoped naming so the
// running agents inside them are found again.
//
// Without this, dropping the repo hash from session names would orphan every
// session already running: the daemon would look for "mycel-fast-crane", find
// nothing, and report an agent as gone while tmux still had it. Renaming keeps
// the process alive — tmux rename-session only changes the label.
//
// Returns the number of sessions adopted. Errors are logged and skipped rather
// than returned: one session that cannot be renamed must not stop the rest from
// being found.
func (m *Manager) AdoptLegacySessions(ctx context.Context) int {
	sessions, err := m.listAllSessionNames(ctx)
	if err != nil {
		log.Warn("could not list tmux sessions to adopt older ones — agents from a previous version may appear stopped", "error", err)
		return 0
	}

	adopted := 0
	for _, full := range sessions {
		if len(full) <= len(m.SessionPrefix) || full[:len(m.SessionPrefix)] != m.SessionPrefix {
			continue
		}
		match := legacySessionName.FindStringSubmatch(full[len(m.SessionPrefix):])
		if match == nil {
			continue
		}
		agent := match[2]
		want := m.SessionName(agent)

		// A session already under the new name is the one the daemon will use, so
		// renaming onto it would fail and, worse, would leave two sessions for one
		// agent if tmux allowed it.
		if m.HasSession(ctx, agent) {
			log.Warn("two tmux sessions for one agent — leaving the older one alone",
				"agent", agent, "older", full, "current", want)
			continue
		}

		// Renamed by full name on both sides: RenameSession applies the prefix to
		// what it is given, and a legacy name is precisely one that does not fit
		// prefix-plus-agent.
		out, err := m.command(ctx, "tmux", "rename-session", "-t", full, want).CombinedOutput()
		if err != nil {
			log.Warn("could not adopt a tmux session from an older version — that agent will look stopped until it is restarted",
				"session", full, "agent", agent, "error", err, "output", strings.TrimSpace(string(out)))
			continue
		}
		m.invalidateCache()
		log.Info("adopted a tmux session named by an older version", "from", full, "to", want, "agent", agent)
		adopted++
	}
	return adopted
}

// listAllSessionNames returns every session on the tmux server, unfiltered.
// ListSessions only reports sessions matching this manager's prefix and strips
// it, which is precisely what the legacy names do not match.
func (m *Manager) listAllSessionNames(ctx context.Context) ([]string, error) {
	cmd := m.command(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// tmux exits non-zero when no server is running, which means no sessions
		// rather than a failure to list them.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}
