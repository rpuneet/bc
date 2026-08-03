package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/rpuneet/mycel/pkg/client"
)

// `mycel agent create --runtime tmux` marshals its request through
// client.CreateAgentReq, which names the field "runtime". POST /api/agents read
// only "runtime_backend", so the flag was accepted by the CLI, sent, and
// discarded by the daemon — and on a machine where docker was reachable the
// agent silently went into a container instead of the tmux session that was
// asked for.
//
// Nothing failed: no error, no warning, just a different runtime than the one
// requested. This pins the contract that made it possible, mirroring the
// handler's decode so a rename on either side has to break here.
func TestCreateAgentRequestRuntimeUsesAKeyTheHandlerReads(t *testing.T) {
	raw, err := json.Marshal(client.CreateAgentReq{Name: "eng-01", Runtime: "tmux"})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}

	// Both names the create handler accepts.
	var decoded struct {
		Runtime    string `json:"runtime_backend"`
		RuntimeAlt string `json:"runtime"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal into the handler's shape: %v", err)
	}

	got := decoded.Runtime
	if got == "" {
		got = decoded.RuntimeAlt
	}
	if got != "tmux" {
		t.Errorf("the runtime the CLI sent reaches the handler as %q, want %q\nrequest body: %s", got, "tmux", raw)
	}
}
