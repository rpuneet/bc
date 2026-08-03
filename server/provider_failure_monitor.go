// provider_failure_monitor.go — noticing an agent whose provider CLI runs but
// cannot work.
//
// mycel derives agent state from what an agent reports. That works until the
// provider refuses every turn: no API key, a spent quota, a model the account
// isn't entitled to. Such an agent reports nothing, so it keeps whatever state
// it had — idle, or working since the last prompt it accepted — and its Live
// feed stays empty with no explanation. Three agents on one machine sat like
// that for days while mycel called them healthy (#3512).
//
// The reason exists only on the agent's terminal, so this monitor reads it: for
// agents that have already gone quiet, it captures recent pane output and asks
// the provider whether the text means "cannot serve a turn"
// (provider.FailureDetector). A match becomes a ProviderFailure event through
// the same ingest path as any hook, which moves the agent to error and puts the
// reason in the feed that was empty.
//
// Ordering matters here: silence is the cheap gate and the pane is the
// expensive, lower-confidence check. An agent that is reporting activity is
// never inspected, so a busy agent that merely mentions an API key cannot be
// mistaken for a broken one.
package server

import (
	"context"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
)

const (
	// failureSweepInterval is how often the monitor looks for quiet agents.
	// A stuck agent stays stuck, so there is nothing to gain from checking
	// often — this only decides how soon the user is told.
	failureSweepInterval = 30 * time.Second

	// failureQuietFor is how long an agent must have reported nothing before
	// its terminal is read.
	//
	// Long enough that a working agent between two tool calls is never
	// inspected, short enough that a user who just started a doomed agent
	// learns why while they are still looking at it.
	failureQuietFor = 2 * time.Minute

	// failurePaneLines is how much scrollback to capture. The detectors read
	// less than this; the surplus covers a provider that wraps a long message
	// above its prompt.
	failurePaneLines = 60
)

// failureMonitorDeps is what the sweep needs, narrowed to an interface so the
// tests can drive it without a tmux session or a real provider registry.
type failureMonitorDeps interface {
	List(ctx context.Context, opts agentpkg.ListOptions) ([]*agentpkg.Agent, error)
	Peek(ctx context.Context, name string, lines int) (string, error)
	IngestHookEvent(ctx context.Context, name string, payload agentpkg.HookPayload, raw []byte) error
}

// runProviderFailureMonitor reports agents whose provider CLI is up but cannot
// serve a turn. It returns when ctx is canceled.
func runProviderFailureMonitor(ctx context.Context, agents *agentpkg.AgentService) {
	if agents == nil {
		return
	}
	ticker := time.NewTicker(failureSweepInterval)
	defer ticker.Stop()

	// Remembers the reason already reported for an agent so one broken agent
	// produces one event, not one every sweep for as long as it stays broken.
	reported := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepForFailedProviders(ctx, agents, reported, time.Now())
		}
	}
}

// sweepForFailedProviders runs a single pass over the agent list.
func sweepForFailedProviders(ctx context.Context, deps failureMonitorDeps, reported map[string]string, now time.Time) {
	list, err := deps.List(ctx, agentpkg.ListOptions{})
	if err != nil {
		log.Debug("failure monitor: agent list failed", "error", err)
		return
	}

	live := make(map[string]struct{}, len(list))
	for _, a := range list {
		if a == nil {
			continue
		}
		live[a.Name] = struct{}{}

		detector := failureDetectorFor(a)
		if detector == nil {
			continue
		}
		if !worthReadingTerminal(a, now) {
			continue
		}

		pane, err := deps.Peek(ctx, a.Name, failurePaneLines)
		if err != nil {
			// A session that cannot be read is not evidence of anything: the
			// agent may be in a container, or already gone.
			log.Debug("failure monitor: peek failed", "agent", a.Name, "error", err)
			continue
		}
		reason, failed := detector.DetectFailure(pane)
		if !failed {
			// Recovered, or never broken. Forget any earlier reason so a
			// recurrence is reported again rather than silently deduped.
			delete(reported, a.Name)
			continue
		}
		if reported[a.Name] == reason {
			continue
		}
		reported[a.Name] = reason

		payload := agentpkg.HookPayload{
			Event:   agentpkg.HookProviderFailure,
			Error:   reason,
			Message: reason,
		}
		if err := deps.IngestHookEvent(ctx, a.Name, payload, nil); err != nil {
			// Let the next sweep try again rather than swallowing the report.
			delete(reported, a.Name)
			log.Debug("failure monitor: ingest failed", "agent", a.Name, "error", err)
			continue
		}
		log.Info("agent's provider cannot serve a turn", "agent", a.Name, "tool", a.Tool, "reason", reason)
	}

	// Drop bookkeeping for agents that no longer exist.
	for name := range reported {
		if _, ok := live[name]; !ok {
			delete(reported, name)
		}
	}
}

// failureDetectorFor returns the failure detector for an agent's provider, or
// nil when the provider does not offer one.
func failureDetectorFor(a *agentpkg.Agent) provider.FailureDetector {
	if a.Tool == "" {
		return nil
	}
	p, ok := provider.DefaultRegistry.Get(a.Tool)
	if !ok {
		return nil
	}
	d, ok := p.(provider.FailureDetector)
	if !ok {
		return nil
	}
	return d
}

// worthReadingTerminal reports whether an agent is quiet enough that reading its
// terminal is justified.
//
// Agents already in a terminal state are skipped: stopped and done agents have
// nothing to diagnose, and an agent already in error has been reported. Everyone
// else has to have been silent for failureQuietFor — UpdatedAt moves on every
// ingested event, so a working agent is never inspected mid-turn.
func worthReadingTerminal(a *agentpkg.Agent, now time.Time) bool {
	switch a.State {
	case agentpkg.StateStopped, agentpkg.StateDone, agentpkg.StateError:
		return false
	}
	if a.UpdatedAt.IsZero() {
		return false
	}
	return now.Sub(a.UpdatedAt) >= failureQuietFor
}
