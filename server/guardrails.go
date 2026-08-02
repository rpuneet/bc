// guardrails.go — background enforcement of template-defined agent
// guardrails (#3423): Template.MaxCostUSD and Template.StuckTimeoutMin were
// persisted and shown in the CLI/UI but never read anywhere, so a runaway or
// hung agent ran forever unless a human noticed. This loop closes that gap.
package server

import (
	"context"
	"fmt"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	eventspkg "github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/log"
	templatepkg "github.com/rpuneet/mycel/pkg/template"
)

// DefaultGuardrailInterval is how often the guardrail loop re-evaluates
// running agents against their template's MaxCostUSD / StuckTimeoutMin.
// Cost lookups ride the cost.Service's own cache (see costServiceAdapter),
// so a 60s tick does not translate into re-scanning provider session files
// every minute.
const DefaultGuardrailInterval = 60 * time.Second

// runGuardrailLoop periodically enforces template-defined guardrails for
// every active agent that carries a Template name (agent.Template, set at
// spawn from CreateOptions.Template — see server/handlers/agents.go):
//
//   - MaxCostUSD: once the agent's cumulative session spend reaches the
//     limit, the agent is stopped via AgentService.StopForGuardrail so a
//     runaway session can't keep burning money unattended. This is a hard
//     stop — cost overrun is an unambiguous signal.
//   - StuckTimeoutMin: once a StateWorking agent has produced no new event
//     (prompt, tool call, hook) for the timeout, it is flagged StateStuck
//     via Manager.MarkStuck — NOT stopped. Idle-detection is inherently
//     fuzzier than a cost overrun (a slow tool call, a long-running test
//     suite, or a permission prompt can all look identical to "hung" from
//     the outside), so the default here is flag + notify: the state change
//     publishes an SSE event and lands in the agent's activity timeline for
//     a human (or #ops loop) to act on, while leaving the agent free to
//     resume — any further hook event flips it back to StateWorking and
//     resets the idle timer. Auto-stopping a possibly-still-useful agent on
//     a heuristic is a worse failure mode than leaving it flagged.
//
// A limit of 0 (the Template zero value) disables that guardrail for that
// template. Agents with no Template (spawned without one) are never
// touched — MaxCostUSD/StuckTimeoutMin are opt-in per template, not global
// defaults.
func runGuardrailLoop(ctx context.Context, agents *agentpkg.AgentService, tmplStore *templatepkg.Store, eventLog eventspkg.EventStore, interval time.Duration) {
	if agents == nil || tmplStore == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultGuardrailInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("agent guardrail loop started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			log.Info("agent guardrail loop stopped")
			return
		case <-ticker.C:
			checkAllGuardrails(ctx, agents, tmplStore, eventLog)
		}
	}
}

// checkAllGuardrails evaluates every active, templated agent once. Template
// lookups are cached for the duration of the tick since a fleet commonly
// spawns many agents off a handful of templates.
func checkAllGuardrails(ctx context.Context, agents *agentpkg.AgentService, tmplStore *templatepkg.Store, eventLog eventspkg.EventStore) {
	list, err := agents.List(ctx, agentpkg.ListOptions{})
	if err != nil {
		log.Debug("guardrail: agent list failed", "error", err)
		return
	}

	templates := map[string]*templatepkg.Template{}

	for _, a := range list {
		if a.Template == "" {
			continue
		}
		// Nothing to enforce on an agent that isn't actually running.
		if a.State == agentpkg.StateStopped || a.State == agentpkg.StateError {
			continue
		}

		tmpl, cached := templates[a.Template]
		if !cached {
			t, _, getErr := tmplStore.Get(a.Template)
			if getErr != nil {
				log.Debug("guardrail: template lookup failed", "agent", a.Name, "template", a.Template, "error", getErr)
			}
			tmpl = t
			templates[a.Template] = tmpl
		}
		if tmpl == nil {
			continue
		}

		if checkCostGuardrail(ctx, agents, a, tmpl) {
			// Agent was just stopped — skip the stuck check this tick.
			continue
		}
		checkStuckGuardrail(ctx, agents, eventLog, a, tmpl)
	}
}

// checkCostGuardrail stops the agent once its cumulative session spend
// reaches the template's MaxCostUSD. Returns true when the agent was
// stopped (so the caller can skip the now-moot stuck check).
func checkCostGuardrail(ctx context.Context, agents *agentpkg.AgentService, a *agentpkg.Agent, tmpl *templatepkg.Template) bool {
	if tmpl.MaxCostUSD <= 0 {
		return false
	}

	summary, err := agents.Cost(ctx, a.Name)
	if err != nil {
		log.Debug("guardrail: cost lookup failed", "agent", a.Name, "error", err)
		return false
	}
	if summary.TotalCostUSD < tmpl.MaxCostUSD {
		return false
	}

	reason := fmt.Sprintf("guardrail: cost limit $%.2f reached ($%.2f spent), stopped", tmpl.MaxCostUSD, summary.TotalCostUSD)
	if err := agents.StopForGuardrail(ctx, a.Name, reason); err != nil {
		log.Warn("guardrail: failed to stop agent over cost limit", "agent", a.Name, "template", a.Template, "error", err)
		return false
	}
	log.Info("guardrail: stopped agent over cost limit",
		"agent", a.Name, "template", a.Template, "limit_usd", tmpl.MaxCostUSD, "spent_usd", summary.TotalCostUSD)
	return true
}

// checkStuckGuardrail flags (never stops) a StateWorking agent that has
// produced no new event for at least the template's StuckTimeoutMin. Only
// StateWorking agents are considered: one already flagged StateStuck is left
// alone until a fresh event moves it back to StateWorking, which naturally
// restarts the idle clock and avoids re-flagging (and re-notifying) every
// tick.
func checkStuckGuardrail(ctx context.Context, agents *agentpkg.AgentService, eventLog eventspkg.EventStore, a *agentpkg.Agent, tmpl *templatepkg.Template) {
	if tmpl.StuckTimeoutMin <= 0 || a.State != agentpkg.StateWorking {
		return
	}

	// Last activity: the newest persisted event for this agent (prompts,
	// tool calls, and other hooks all append a row — see
	// AgentService.IngestHookEvent) — falling back to the agent's own
	// UpdatedAt when there is no event log wired or no events yet (e.g.
	// right after spawn).
	lastActivity := a.UpdatedAt
	if eventLog != nil {
		if latest, readErr := eventLog.ReadByAgentPage(a.Name, 1, 0); readErr == nil && len(latest) > 0 {
			lastActivity = latest[0].Timestamp
		}
	}
	if lastActivity.IsZero() {
		return
	}

	idleFor := time.Since(lastActivity)
	timeout := time.Duration(tmpl.StuckTimeoutMin) * time.Minute
	if idleFor < timeout {
		return
	}

	reason := fmt.Sprintf("guardrail: no activity for %s (stuck timeout %dm), flagged stuck", idleFor.Round(time.Second), tmpl.StuckTimeoutMin)
	if err := agents.Manager().MarkStuck(ctx, a.Name, reason); err != nil {
		log.Debug("guardrail: failed to flag stuck agent", "agent", a.Name, "error", err)
		return
	}
	log.Info("guardrail: flagged agent stuck",
		"agent", a.Name, "template", a.Template, "idle_for", idleFor, "timeout_min", tmpl.StuckTimeoutMin)
}
