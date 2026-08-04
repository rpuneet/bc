// session_reconciler.go — keeping agent state answerable to tmux.
//
// mycel derives agent state from what an agent reports, which holds until the
// session dies without reporting anything: killed pane, a machine that slept, a
// tmux server that went away. The agent then keeps the state it last reported,
// forever. `zen-zebra` was listed as working with no session behind it, and only
// the terminal tab admitted it, returning 409 while every other surface showed a
// working agent (#3570).
//
// AgentService.SyncSessions has always been able to notice this. Nothing called
// it: the only trigger was POST /api/agents/sync, which no part of mycel invokes,
// so reconciliation existed and never ran. This loop runs it — once at startup,
// because that is when records are most likely to be stale, and then on a timer.
package server

import (
	"context"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/log"
)

// sessionSyncInterval is how often agent state is checked against the runtime.
// A vanished session does not come back, so this only decides how long a stale
// "working" can be believed.
const sessionSyncInterval = 30 * time.Second

// sessionSyncer is the part of AgentService this loop needs, narrowed so a test
// can drive it without a runtime.
type sessionSyncer interface {
	SyncSessions(ctx context.Context) (synced, stopped, resumed int)
}

// runSessionReconciler reconciles agent state against live sessions until ctx is
// canceled.
func runSessionReconciler(ctx context.Context, agents sessionSyncer) {
	if agents == nil {
		return
	}

	// Startup first: records loaded from disk describe the world as it was when
	// the daemon last ran, which may be several reboots ago.
	syncOnce(ctx, agents)

	ticker := time.NewTicker(sessionSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce(ctx, agents)
		}
	}
}

func syncOnce(ctx context.Context, agents sessionSyncer) {
	synced, stopped, resumed := agents.SyncSessions(ctx)
	if stopped > 0 || resumed > 0 {
		log.Info("reconciled agent state against live sessions",
			"inspected", synced, "stopped", stopped, "resumed", resumed)
	}
}

// Compile-time check that the real service satisfies what the loop asks for.
var _ sessionSyncer = (*agentpkg.AgentService)(nil)
