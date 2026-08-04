package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// tmuxStub answers tmux subcommands and records what was asked of it, so a test
// can assert which sessions were renamed rather than only how many.
type tmuxStub struct {
	sessions map[string]bool // session names tmux reports as existing
	calls    []string        // full argument lines, in order
	mu       sync.Mutex
	failNext bool // make the next rename fail
}

func (s *tmuxStub) exec(name string, args ...string) *exec.Cmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, strings.Join(args, " "))

	stdout, exit := "", 0
	switch {
	case len(args) > 0 && args[0] == "list-sessions":
		names := make([]string, 0, len(s.sessions))
		for n := range s.sessions {
			names = append(names, n)
		}
		if len(names) == 0 {
			exit = 1 // tmux exits non-zero when there is no server
			break
		}
		stdout = strings.Join(names, "\n") + "\n"
	case len(args) > 0 && args[0] == "has-session":
		target := ""
		for i, a := range args {
			if a == "-t" && i+1 < len(args) {
				target = args[i+1]
			}
		}
		if !s.sessions[target] {
			exit = 1
		}
	case len(args) > 0 && args[0] == "rename-session":
		if s.failNext {
			s.failNext = false
			exit = 1
			break
		}
		if len(args) >= 4 {
			delete(s.sessions, args[2])
			s.sessions[args[3]] = true
		}
	}

	cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
	cmd := exec.CommandContext(context.Background(), os.Args[0], cs...) //nolint:gosec // test helper
	cmd.Env = []string{
		"GO_WANT_HELPER_PROCESS=1",
		"MOCK_STDOUT=" + stdout,
		fmt.Sprintf("MOCK_EXIT_CODE=%d", exit),
	}
	return cmd
}

func (s *tmuxStub) renamed() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, c := range s.calls {
		if strings.HasPrefix(c, "rename-session") {
			out = append(out, c)
		}
	}
	return out
}

func stubManager(sessions ...string) (*Manager, *tmuxStub) {
	stub := &tmuxStub{sessions: map[string]bool{}}
	for _, s := range sessions {
		stub.sessions[s] = true
	}
	m := &Manager{SessionPrefix: DefaultPrefix, execCommand: stub.exec, hasSessionCache: map[string]bool{}}
	return m, stub
}

// The whole point: a session running under the old repo-scoped name is found
// again. Without this the daemon looks for "mycel-fast-crane", finds nothing,
// and reports a running agent as stopped.
func TestAdoptRenamesARepoScopedSession(t *testing.T) {
	m, stub := stubManager("mycel-13c6e9-fast-crane")

	if n := m.AdoptLegacySessions(t.Context()); n != 1 {
		t.Fatalf("adopted %d sessions, want 1", n)
	}

	if got := stub.renamed(); len(got) != 1 || !strings.Contains(got[0], "mycel-13c6e9-fast-crane mycel-fast-crane") {
		t.Errorf("rename calls = %v, want the legacy name renamed to mycel-fast-crane", got)
	}
	if !stub.sessions["mycel-fast-crane"] {
		t.Error("the session is not under the new name")
	}
}

// An agent name containing a hyphen must survive: the hash is the first segment,
// the rest is the agent, however many hyphens it has.
func TestAdoptKeepsTheWholeAgentName(t *testing.T) {
	m, stub := stubManager("mycel-13c6e9-bright-finch-2")

	if n := m.AdoptLegacySessions(t.Context()); n != 1 {
		t.Fatalf("adopted %d sessions, want 1", n)
	}
	if !stub.sessions["mycel-bright-finch-2"] {
		t.Errorf("sessions = %v, want mycel-bright-finch-2", stub.sessions)
	}
}

// Running twice must not rename anything the second time, or a daemon restart
// churns tmux for no reason.
func TestAdoptIsIdempotent(t *testing.T) {
	m, stub := stubManager("mycel-13c6e9-fast-crane")

	m.AdoptLegacySessions(t.Context())
	first := len(stub.renamed())

	m.invalidateCache()
	if n := m.AdoptLegacySessions(t.Context()); n != 0 {
		t.Errorf("second run adopted %d sessions, want 0", n)
	}
	if len(stub.renamed()) != first {
		t.Errorf("second run renamed again: %v", stub.renamed())
	}
}

// A session already under the new name is the one the daemon will use. Renaming
// the older one onto it would fail, and must not be attempted.
func TestAdoptLeavesADuplicateAlone(t *testing.T) {
	m, stub := stubManager("mycel-13c6e9-fast-crane", "mycel-fast-crane")

	if n := m.AdoptLegacySessions(t.Context()); n != 0 {
		t.Errorf("adopted %d sessions, want 0 when the new name is taken", n)
	}
	if got := stub.renamed(); len(got) != 0 {
		t.Errorf("attempted a rename onto an existing session: %v", got)
	}
	if !stub.sessions["mycel-13c6e9-fast-crane"] {
		t.Error("the older session was lost")
	}
}

// Sessions that are not mycel's, and mycel sessions already correctly named, are
// left exactly as they are.
func TestAdoptTouchesNothingElse(t *testing.T) {
	m, stub := stubManager("mycel-fast-crane", "bc-13c6e9-old", "my-editor", "mycel-agent")

	if n := m.AdoptLegacySessions(t.Context()); n != 0 {
		t.Errorf("adopted %d sessions, want 0", n)
	}
	if got := stub.renamed(); len(got) != 0 {
		t.Errorf("renamed something it should not have: %v", got)
	}
}

// A name that merely looks like a hash — wrong length, or not hex — is an agent
// name, not a namespace, and renaming it would lose the agent.
func TestAdoptIgnoresSomethingThatIsNotAHash(t *testing.T) {
	for _, session := range []string{
		"mycel-13c6e-fast-crane",   // five hex digits
		"mycel-13c6e9a-fast-crane", // seven
		"mycel-13c6eg-fast-crane",  // g is not hex
		"mycel-abcdef",             // no agent after it
	} {
		m, stub := stubManager(session)
		if n := m.AdoptLegacySessions(t.Context()); n != 0 {
			t.Errorf("%s: adopted %d, want 0", session, n)
		}
		if got := stub.renamed(); len(got) != 0 {
			t.Errorf("%s: renamed %v", session, got)
		}
	}
}

// One session that cannot be renamed must not stop the others from being found.
func TestAdoptContinuesPastAFailedRename(t *testing.T) {
	m, stub := stubManager("mycel-13c6e9-fast-crane", "mycel-13c6e9-cool-otter")
	stub.failNext = true

	if n := m.AdoptLegacySessions(t.Context()); n != 1 {
		t.Errorf("adopted %d sessions, want 1 of the two", n)
	}
}

// No tmux server at all is not a failure.
func TestAdoptWithNoSessions(t *testing.T) {
	m, _ := stubManager()

	if n := m.AdoptLegacySessions(t.Context()); n != 0 {
		t.Errorf("adopted %d sessions, want 0", n)
	}
}
