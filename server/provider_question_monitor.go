// provider_question_monitor.go — noticing an agent that has stopped to ask its
// user something.
//
// A provider holding a permission prompt or a choice menu open is blocked until
// a person answers it, and mycel has a state for that. The ones that say so
// through a hook never reach this file. cursor does not: nothing it emits fires
// while it waits, so an agent sitting on a question looks exactly like an agent
// thinking, right up until the guardrail loop flags it stuck half an hour later
// and blames its own timer (#3582).
//
// So the terminal is read, the same way provider_failure_monitor.go reads it,
// and with the same ordering: silence first because it is cheap, the pane
// second because it is the low-confidence part. An agent still reporting
// activity is never inspected, which is most of what keeps a working agent that
// happens to print "(y/n)" from being flagged.
//
// Unlike a failure, a question ends. Agents already flagged here keep being
// read, so the moment their prompt is gone they are handed back to working
// rather than left flagged for a turn they are busy serving.
package server

import (
	"context"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
)

const (
	// questionSweepInterval is how often the monitor looks for agents that
	// have gone quiet. Tighter than the failure sweep, because this decides
	// how long a person is not told that an agent is waiting on them.
	questionSweepInterval = 15 * time.Second

	// questionQuietFor is how long an agent must have reported nothing before
	// its terminal is read.
	//
	// Long enough that a working agent between two tool calls is never
	// inspected; short enough that "your agent needs you" arrives while the
	// person who started it is still nearby.
	questionQuietFor = 45 * time.Second

	// questionPaneLines is how much scrollback to capture. The detectors read
	// less; the surplus covers a provider that wraps a long question above the
	// prompt it is waiting on.
	questionPaneLines = 40
)

// questionMonitorDeps is what the sweep needs, narrowed to an interface so the
// tests can drive it without a tmux session or a real provider registry.
type questionMonitorDeps interface {
	List(ctx context.Context, opts agentpkg.ListOptions) ([]*agentpkg.Agent, error)
	Peek(ctx context.Context, name string, lines int) (string, error)
	IngestHookEvent(ctx context.Context, name string, payload agentpkg.HookPayload, raw []byte) error
}

// runProviderQuestionMonitor flags agents whose provider CLI is waiting for a
// person, and unflags them once it is not. It returns when ctx is canceled.
func runProviderQuestionMonitor(ctx context.Context, agents *agentpkg.AgentService) {
	if agents == nil {
		return
	}
	ticker := time.NewTicker(questionSweepInterval)
	defer ticker.Stop()

	// The question each agent was last reported as waiting on, which is both
	// how one open prompt produces one event rather than one per sweep, and
	// how the sweep knows to keep watching an agent it already flagged.
	asked := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepForOpenQuestions(ctx, agents, asked, time.Now())
		}
	}
}

// sweepForOpenQuestions runs a single pass over the agent list.
func sweepForOpenQuestions(ctx context.Context, deps questionMonitorDeps, asked map[string]string, now time.Time) {
	list, err := deps.List(ctx, agentpkg.ListOptions{})
	if err != nil {
		log.Debug("question monitor: agent list failed", "error", err)
		return
	}

	live := make(map[string]struct{}, len(list))
	for _, a := range list {
		if a == nil {
			continue
		}
		live[a.Name] = struct{}{}

		detector := questionDetectorFor(a)
		if detector == nil {
			continue
		}
		previous, flagged := asked[a.Name]
		if !answerableState(a.State) {
			delete(asked, a.Name)
			continue
		}
		// A flagged agent is read every sweep regardless of how recently its
		// state changed: flagging it moved its own clock, so the quiet gate
		// would otherwise stop the monitor noticing the answer it is watching
		// for.
		if !flagged && now.Sub(a.UpdatedAt) < questionQuietFor {
			continue
		}

		pane, err := deps.Peek(ctx, a.Name, questionPaneLines)
		if err != nil {
			// A session that cannot be read is not evidence of anything: the
			// agent may be in a container, or already gone.
			log.Debug("question monitor: peek failed", "agent", a.Name, "error", err)
			continue
		}

		question, waiting := detector.DetectQuestion(pane)
		switch {
		case !waiting && flagged:
			delete(asked, a.Name)
			releaseAnsweredAgent(ctx, deps, a)
		case !waiting:
		case previous == question:
		default:
			asked[a.Name] = question
			payload := agentpkg.HookPayload{
				Event:   agentpkg.HookAwaitingInput,
				Message: question,
			}
			if err := deps.IngestHookEvent(ctx, a.Name, payload, nil); err != nil {
				// Let the next sweep try again rather than swallowing the
				// report.
				delete(asked, a.Name)
				log.Debug("question monitor: ingest failed", "agent", a.Name, "error", err)
				continue
			}
			log.Info("agent is waiting for a person", "agent", a.Name, "tool", a.Tool, "question", question)
		}
	}

	// Drop bookkeeping for agents that no longer exist.
	for name := range asked {
		if _, ok := live[name]; !ok {
			delete(asked, name)
		}
	}
}

// releaseAnsweredAgent hands an agent back to working once the prompt it was
// holding open is gone.
//
// Only an agent still sitting in stuck is moved. Anything else has already been
// corrected by its own hooks — a cursor agent that answers a question and
// finishes reports Stop, which is idle, and overwriting that with working would
// be the monitor undoing a fact with an inference.
func releaseAnsweredAgent(ctx context.Context, deps questionMonitorDeps, a *agentpkg.Agent) {
	if a.State != agentpkg.StateStuck {
		return
	}
	payload := agentpkg.HookPayload{
		Event:   agentpkg.HookInputProvided,
		Message: "the prompt the provider was waiting on is gone",
	}
	if err := deps.IngestHookEvent(ctx, a.Name, payload, nil); err != nil {
		log.Debug("question monitor: release failed", "agent", a.Name, "error", err)
		return
	}
	log.Info("agent's question was answered", "agent", a.Name, "tool", a.Tool)
}

// questionDetectorFor returns the question detector for an agent's provider, or
// nil when the provider does not offer one.
func questionDetectorFor(a *agentpkg.Agent) provider.QuestionDetector {
	if a.Tool == "" {
		return nil
	}
	p, ok := provider.DefaultRegistry.Get(a.Tool)
	if !ok {
		return nil
	}
	d, ok := p.(provider.QuestionDetector)
	if !ok {
		return nil
	}
	return d
}

// answerableState reports whether an agent in this state could be sitting on a
// question worth flagging.
//
// Terminal states have nobody left to answer. starting is excluded for a
// different reason: the state machine does not allow starting → stuck, so
// flagging one would be rejected anyway, and an agent that has not finished
// booting has not asked anything yet.
func answerableState(s agentpkg.State) bool {
	switch s {
	case agentpkg.StateIdle, agentpkg.StateWorking, agentpkg.StateStuck:
		return true
	}
	return false
}
