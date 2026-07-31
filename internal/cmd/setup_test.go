package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
)

// testDaemonHandler is the handler used by the package-level fake daemon server.
// Tests can swap it via setTestDaemonHandler / resetTestDaemonHandler to assert
// against the daemon interactions; the default returns 404 for every path so that
// "no agent found" / "not in a repo" code paths are exercised without
// reaching a real daemon.
//
// IMPORTANT: this server protects production the daemon from `go test` runs.
// Without it, executeIntegrationCmd would resolve MYCEL_DAEMON_ADDR to the
// real daemon at 127.0.0.1:9374 and hammer it during CI / dev test runs.
var testDaemonHandler atomic.Value // stores http.HandlerFunc

func defaultTestDaemonHandler() http.HandlerFunc {
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

// setTestDaemonHandler swaps the active fake-the daemon handler for the duration
// of the test, restoring the default on cleanup.
func setTestDaemonHandler(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	prev := testDaemonHandler.Load()
	testDaemonHandler.Store(h)
	t.Cleanup(func() {
		if prev != nil {
			testDaemonHandler.Store(prev)
		} else {
			testDaemonHandler.Store(defaultTestDaemonHandler())
		}
	})
}

func TestMain(m *testing.M) {
	// Start a fake daemon server for the entire test process so no test
	// accidentally reaches the real daemon. Individual tests can override
	// the handler via setTestDaemonHandler.
	testDaemonHandler.Store(defaultTestDaemonHandler())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, _ := testDaemonHandler.Load().(http.HandlerFunc)
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Force the mycel client to talk to our fake server, not the real daemon.
	_ = os.Setenv("MYCEL_DAEMON_ADDR", srv.URL)

	// Clear repo env vars inherited from the dev's shell so tests
	// that intentionally chdir to a tmpDir (and expect "not in a mycel
	// workspace") don't accidentally pick up the developer's MYCEL_WORKSPACE
	// pointing at the mycel repo. Tests that need a repo set this
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
