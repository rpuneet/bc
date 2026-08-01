package agent

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/events"
)

// appendRecorder captures events persisted through the HookEventAppender.
type appendRecorder struct {
	evts []events.Event
}

func (r *appendRecorder) Append(e events.Event) error {
	r.evts = append(r.evts, e)
	return nil
}

// TestPublishEventPersistsLifecycle locks the #37 fix: lifecycle events
// must land in the event store keyed by agent, so hook-less providers
// (cursor, agy, pi) still get an activity timeline from birth.
func TestPublishEventPersistsLifecycle(t *testing.T) {
	rec := &appendRecorder{}
	svc := &AgentService{hookStore: rec}

	svc.publishEvent("agent.created", map[string]any{"name": "keen-lemur", "tool": "cursor"})
	svc.publishEvent("agent.state_changed", map[string]any{"name": "keen-lemur", "state": "idle"})
	// No agent name — must not persist a row with an empty agent key.
	svc.publishEvent("agents.stopped_all", map[string]any{"count": 3})

	if len(rec.evts) != 2 {
		t.Fatalf("persisted %d events, want 2", len(rec.evts))
	}
	for _, e := range rec.evts {
		if e.Agent != "keen-lemur" {
			t.Errorf("event %s agent = %q, want keen-lemur", e.Type, e.Agent)
		}
		if e.Timestamp.IsZero() {
			t.Errorf("event %s has zero timestamp", e.Type)
		}
	}
	if rec.evts[0].Type != "agent.created" || rec.evts[1].Type != "agent.state_changed" {
		t.Errorf("types = %s, %s", rec.evts[0].Type, rec.evts[1].Type)
	}
}
