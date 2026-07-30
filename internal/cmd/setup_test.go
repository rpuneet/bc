package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
)

// testBcdHandler is the handler used by the package-level fake bcd server.
// Tests can swap it via setTestBcdHandler / resetTestBcdHandler to assert
// against bcd interactions; the default returns 404 for every path so that
// "no agent found" / "not in a repo" code paths are exercised without
// reaching a real bcd.
//
// IMPORTANT: this server protects production bcd from `go test` runs.
// Without it, executeIntegrationCmd would resolve MYCEL_DAEMON_ADDR to the
// real daemon at 127.0.0.1:9374 and hammer it during CI / dev test runs.
var testBcdHandler atomic.Value // stores http.HandlerFunc

func defaultTestBcdHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// /health must succeed so newDaemonClient.Ping() doesn't fail
		// before repo checks complete.
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.NotFound(w, r)
	}
}

// setTestBcdHandler swaps the active fake-bcd handler for the duration
// of the test, restoring the default on cleanup.
func setTestBcdHandler(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	prev := testBcdHandler.Load()
	testBcdHandler.Store(h)
	t.Cleanup(func() {
		if prev != nil {
			testBcdHandler.Store(prev)
		} else {
			testBcdHandler.Store(defaultTestBcdHandler())
		}
	})
}

func TestMain(m *testing.M) {
	// Start a fake bcd server for the entire test process so no test
	// accidentally reaches the real bcd. Individual tests can override
	// the handler via setTestBcdHandler.
	testBcdHandler.Store(defaultTestBcdHandler())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, _ := testBcdHandler.Load().(http.HandlerFunc)
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Force the bc client to talk to our fake server, not the real bcd.
	_ = os.Setenv("MYCEL_DAEMON_ADDR", srv.URL)

	// Clear repo env vars inherited from the dev's shell so tests
	// that intentionally chdir to a tmpDir (and expect "not in a bc
	// workspace") don't accidentally pick up the developer's MYCEL_WORKSPACE
	// pointing at the bc repo. Tests that need a repo set this
	// explicitly via t.Setenv in setupIntegrationHome.
	_ = os.Unsetenv("MYCEL_WORKSPACE")
	_ = os.Unsetenv("MYCEL_AGENT_WORKTREE")

	// Point the single global database (and everything else under
	// MYCEL_HOME) at a throwaway dir so tests never touch ~/.mycel.
	var testHome string
	if home, homeErr := os.MkdirTemp("", "mycel-test-home-*"); homeErr == nil {
		testHome = home
		_ = os.Setenv("MYCEL_HOME", home)
	}

	// Setup roles for tests - mirrors pkg/agent/agent_test.go TestMain
	agent.RoleCapabilities[agent.Role("engineer")] = []agent.Capability{agent.CapImplementTasks}
	agent.RoleCapabilities[agent.Role("manager")] = []agent.Capability{agent.CapAssignWork, agent.CapCreateAgents}
	agent.RoleCapabilities[agent.Role("qa")] = []agent.Capability{agent.CapTestWork, agent.CapReviewWork}
	agent.RoleCapabilities[agent.Role("product-manager")] = []agent.Capability{agent.CapCreateEpics, agent.CapCreateAgents}
	agent.RoleCapabilities[agent.Role("worker")] = []agent.Capability{agent.CapImplementTasks}
	agent.RoleCapabilities[agent.Role("tech-lead")] = []agent.Capability{agent.CapReviewWork, agent.CapCreateAgents}

	agent.RoleHierarchy[agent.Role("manager")] = []agent.Role{
		agent.Role("engineer"),
		agent.Role("qa"),
		agent.Role("tech-lead"),
	}
	agent.RoleHierarchy[agent.Role("tech-lead")] = []agent.Role{
		agent.Role("engineer"),
	}
	agent.RoleHierarchy[agent.Role("product-manager")] = []agent.Role{agent.Role("manager")}
	agent.RoleHierarchy[agent.RoleRoot] = []agent.Role{
		agent.Role("product-manager"),
		agent.Role("manager"),
		agent.Role("engineer"),
		agent.Role("qa"),
		agent.Role("worker"),
		agent.Role("tech-lead"),
	}

	code := m.Run()
	if testHome != "" {
		_ = os.RemoveAll(testHome)
	}
	os.Exit(code)
}
