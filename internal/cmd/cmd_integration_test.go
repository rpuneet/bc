package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

func durationFromSeconds(s int) time.Duration {
	return time.Duration(s) * time.Second
}

// resetFlags recursively resets all flags on a command and its subcommands
// to their default values, preventing state from leaking between tests.
func resetFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

// setupIntegrationWorkspace creates a temporary bc workspace and changes into it.
// Returns the workspace root path and a cleanup function that restores
// the original working directory and BC_WORKSPACE env var.
func setupIntegrationWorkspace(t *testing.T) (string, func()) {
	t.Helper()

	if os.Getenv("BC_TEST_DAEMON") == "" {
		t.Skip("skipping: requires BC_TEST_DAEMON=1 (dedicated test bcd instance)")
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()

	// Initialize a workspace using the workspace package
	ws, err := workspace.Init(tmpDir)
	if err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}
	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("failed to ensure dirs: %v", err)
	}

	// Point BC_WORKSPACE at the temp workspace so getRepo() finds it
	// regardless of cwd races between parallel tests.
	t.Setenv("BC_WORKSPACE", tmpDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp workspace: %v", err)
	}

	return tmpDir, func() {
		_ = os.Chdir(origDir)
	}
}

// executeIntegrationCmdT runs rootCmd with an isolated fake bcd server.
// If handler is non-nil, it overrides the package-level fake bcd handler
// for the duration of the test. The default handler (set in TestMain)
// returns 200 on /health and 404 elsewhere — enough to exercise CLI
// error paths without ever reaching the real bcd at :9374.
//
// Prefer this over executeIntegrationCmd when a test wants to assert
// against bcd responses (e.g. a successful agent send). Otherwise the
// plain executeIntegrationCmd is fine — TestMain pins BC_DAEMON_ADDR
// at the fake server for the whole test process.
func executeIntegrationCmdT(t *testing.T, handler http.HandlerFunc, args ...string) (string, string, error) {
	t.Helper()
	if handler != nil {
		setTestBcdHandler(t, handler)
	}
	return executeIntegrationCmd(args...)
}

// executeIntegrationCmd runs rootCmd with the given args, capturing real stdout output.
// Commands use fmt.Printf/Println (writing to os.Stdout), so we redirect
// os.Stdout to a pipe to capture output. Returns captured stdout and any error.
//
// All callers are automatically isolated from the production bcd: TestMain
// starts a fake httptest.Server and pins BC_DAEMON_ADDR to it for the
// entire test process.
func executeIntegrationCmd(args ...string) (string, string, error) {
	// Save and redirect os.Stdout
	origStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		return "", "", pipeErr
	}
	os.Stdout = w

	// Also redirect pkg/ui output which uses its own writer
	ui.SetOutput(w)
	defer ui.SetOutput(os.Stdout)

	stderr := new(bytes.Buffer)
	rootCmd.SetOut(w)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs(args)

	// Reset persistent flags to prevent leaking between tests
	_ = rootCmd.PersistentFlags().Set("json", "false")
	_ = rootCmd.PersistentFlags().Set("verbose", "false")
	defer func() { _ = rootCmd.PersistentFlags().Set("json", "false") }()
	defer func() { _ = rootCmd.PersistentFlags().Set("verbose", "false") }()

	// Reset all subcommand flags to default values to prevent state leaking
	// between tests. Must recurse into nested subcommands (e.g. channel history).
	resetFlags(rootCmd)

	err := rootCmd.Execute()

	// Close writer and read all captured output
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = origStdout

	return buf.String(), stderr.String(), err
}

// seedAgents writes an agents.json file in the workspace's agents directory.
// The Manager stores agents as map[string]*Agent.
func seedAgents(t *testing.T, wsDir string, agents map[string]*agent.Agent) {
	t.Helper()
	agentsDir := filepath.Join(wsDir, ".bc", "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.json"), data, 0600); err != nil {
		t.Fatalf("failed to write agents.json: %v", err)
	}
}

// --- Agent Send command tests ---

func TestAgentSendNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Use the per-test handler variant so this test additionally asserts
	// no bcd traffic is generated when there's no workspace: any request
	// the CLI makes would be a bug (workspace check must run first).
	bcdHit := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			bcdHit = true
		}
		w.WriteHeader(http.StatusOK)
	}
	_, _, err = executeIntegrationCmdT(t, handler, "agent", "send", "worker-01", "hello")
	if err == nil {
		t.Fatal("expected error when not in workspace, got nil")
	}
	if !strings.Contains(err.Error(), "not in a mycel workspace") {
		t.Errorf("expected workspace error, got: %v", err)
	}
	if bcdHit {
		t.Error("CLI should not contact bcd when there is no workspace")
	}
}

func TestAgentSendAgentNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "send", "nonexistent-agent", "hello")
	if err == nil {
		t.Fatal("expected error for missing agent, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected agent not found error, got: %v", err)
	}
}

func TestAgentSendRequiresArgs(t *testing.T) {
	_, _, err := executeIntegrationCmd("agent", "send")
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

// --- Agent Report command tests ---

func TestAgentReportNoAgentID(t *testing.T) {
	// Ensure BC_AGENT_ID is not set
	orig := os.Getenv("BC_AGENT_ID")
	if err := os.Unsetenv("BC_AGENT_ID"); err != nil {
		t.Fatalf("failed to unsetenv: %v", err)
	}
	defer func() {
		if orig != "" {
			_ = os.Setenv("BC_AGENT_ID", orig)
		}
	}()

	_, _, err := executeIntegrationCmd("agent", "report", "working", "testing")
	if err == nil {
		t.Fatal("expected error when BC_AGENT_ID not set, got nil")
	}
	if !strings.Contains(err.Error(), "this command can only be run by agents in the bc system") {
		t.Errorf("expected agent-only command error, got: %v", err)
	}
}

func TestAgentReportInvalidState(t *testing.T) {
	t.Setenv("BC_AGENT_ID", "test-agent")

	_, _, err := executeIntegrationCmd("agent", "report", "invalid-state")
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected invalid state error, got: %v", err)
	}
}

func TestAgentReportNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Clear workspace env vars to ensure workspace lookup fails (#1668)
	t.Setenv("BC_WORKSPACE", "")
	t.Setenv("BC_AGENT_WORKTREE", "")
	t.Setenv("BC_AGENT_ID", "test-agent")

	_, _, err = executeIntegrationCmd("agent", "report", "working", "testing")
	if err == nil {
		t.Fatal("expected error when not in workspace, got nil")
	}
	if !strings.Contains(err.Error(), "not in a mycel workspace") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestAgentReportValidStates(t *testing.T) {
	validStates := []string{"idle", "working", "done", "stuck", "error"}

	for _, state := range validStates {
		t.Run(state, func(t *testing.T) {
			t.Setenv("BC_AGENT_ID", "test-agent")
			t.Setenv("BC_WORKSPACE", "")      // Clear workspace env to test cwd-based discovery
			t.Setenv("BC_AGENT_WORKTREE", "") // Clear worktree env to avoid spurious warnings (#1668)

			// State validation happens before workspace lookup, but
			// invalid states are rejected. Valid states proceed to
			// workspace lookup, which we test fails correctly outside a workspace.
			origDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get cwd: %v", err)
			}
			tmpDir := t.TempDir()
			if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
				t.Fatalf("failed to chdir: %v", chdirErr)
			}
			defer func() { _ = os.Chdir(origDir) }()

			_, _, err = executeIntegrationCmd("agent", "report", state, "test message")
			// Should fail with workspace error, NOT invalid state error
			if err == nil {
				t.Fatal("expected workspace error, got nil")
			}
			if strings.Contains(err.Error(), "invalid state") {
				t.Errorf("state %q should be valid but got invalid state error", state)
			}
		})
	}
}

func TestAgentReportRequiresArgs(t *testing.T) {
	_, _, err := executeIntegrationCmd("agent", "report")
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

// --- Status command tests ---

func TestStatusNoWorkspace(t *testing.T) {
	t.Setenv("BC_WORKSPACE", "") // Clear workspace env to test cwd-based discovery

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	_, _, err = executeIntegrationCmd("status")
	if err == nil {
		t.Fatal("expected error when not in workspace, got nil")
	}
	if !strings.Contains(err.Error(), "not in a mycel workspace") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestStatusEmptyWorkspace(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Create agents dir so LoadState doesn't warn
	if err := os.MkdirAll(filepath.Join(wsDir, ".bc", "agents"), 0750); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	stdout, _, err := executeIntegrationCmd("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	if !strings.Contains(stdout, "No agents configured") {
		t.Errorf("expected 'No agents configured', got: %s", stdout)
	}
	if !strings.Contains(stdout, "Workspace:") {
		t.Errorf("expected workspace path in output, got: %s", stdout)
	}
}

// --- Helper function tests ---

func TestFormatDurationIntegration(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		seconds  int
	}{
		{"zero", "0s", 0},
		{"seconds", "45s", 45},
		{"minutes", "2m 5s", 125},
		{"hours", "1h 2m", 3725},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := formatDuration(durationFromSeconds(tt.seconds))
			if d != tt.expected {
				t.Errorf("formatDuration(%ds) = %q, want %q", tt.seconds, d, tt.expected)
			}
		})
	}
}

func TestColorStateIntegration(t *testing.T) {
	tests := []struct {
		state    string
		contains string
	}{
		{"idle", "idle"},
		{"working", "working"},
		{"done", "done"},
		{"stuck", "stuck"},
		{"error", "error"},
		{"stopped", "stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := colorStateStr(tt.state)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("colorStateStr(%s) = %q, should contain %q", tt.state, result, tt.contains)
			}
		})
	}
}

func TestColorStateStr_Default(t *testing.T) {
	result := colorStateStr("unknown")
	if !strings.Contains(result, "unknown") {
		t.Errorf("colorStateStr(unknown) = %q, should contain 'unknown'", result)
	}
	// Default should NOT contain ANSI escape codes
	if strings.Contains(result, "\033[") {
		t.Errorf("colorStateStr(unknown) should not have color codes, got: %q", result)
	}
}

// --- Logs command tests ---

// seedEvents writes events to the workspace state.db SQLite database.
func seedEvents(t *testing.T, wsDir string, evts []events.Event) {
	t.Helper()
	d, _, err := db.Global(nil)
	if err != nil {
		t.Fatalf("failed to open global db: %v", err)
	}
	evtLog, err := events.NewSQLiteLog(d)
	if err != nil {
		t.Fatalf("failed to open event log: %v", err)
	}
	defer func() { _ = evtLog.Close() }()
	for _, ev := range evts {
		if err := evtLog.Append(ev); err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}
}

func TestLogsNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	_, _, err = executeIntegrationCmd("logs")
	if err == nil {
		t.Fatal("expected error when not in workspace, got nil")
	}
	if !strings.Contains(err.Error(), "not in a mycel workspace") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestLogsEmpty(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	stdout, _, err := executeIntegrationCmd("logs")
	if err != nil {
		t.Fatalf("logs returned error: %v", err)
	}
	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected 'No events found', got: %s", stdout)
	}
}

func TestLogsWithEvents(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedEvents(t, wsDir, []events.Event{
		{
			Timestamp: time.Now().Add(-5 * time.Minute),
			Type:      events.AgentSpawned,
			Agent:     "worker-01",
			Message:   "spawned worker",
		},
		{
			Timestamp: time.Now().Add(-3 * time.Minute),
			Type:      events.WorkStarted,
			Agent:     "worker-01",
			Message:   "started task",
		},
		{
			Timestamp: time.Now().Add(-1 * time.Minute),
			Type:      events.WorkCompleted,
			Agent:     "worker-01",
			Message:   "completed task",
		},
	})

	stdout, _, err := executeIntegrationCmd("logs")
	if err != nil {
		t.Fatalf("logs returned error: %v", err)
	}
	if !strings.Contains(stdout, "agent.spawned") {
		t.Errorf("expected event type in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "[worker-01]") {
		t.Errorf("expected agent name in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "spawned worker") {
		t.Errorf("expected message in output, got: %s", stdout)
	}
}

func TestLogsTail(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed 5 events
	for i := 0; i < 5; i++ {
		seedEvents(t, wsDir, []events.Event{
			{
				Timestamp: time.Now().Add(time.Duration(-5+i) * time.Minute),
				Type:      events.AgentReport,
				Agent:     "worker-01",
				Message:   "event-" + string(rune('A'+i)),
			},
		})
	}

	// Reset the logsTail flag before test
	logsTail = 2
	defer func() { logsTail = 0 }()

	stdout, _, err := executeIntegrationCmd("logs", "--tail", "2")
	if err != nil {
		t.Fatalf("logs --tail returned error: %v", err)
	}

	// Should show last 2 events
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	// Filter to only lines with actual event content
	eventLines := 0
	for _, l := range lines {
		if strings.Contains(l, "agent.report") {
			eventLines++
		}
	}
	if eventLines != 2 {
		t.Errorf("expected 2 event lines with --tail 2, got %d\noutput: %s", eventLines, stdout)
	}
}

func TestLogsAgentFilter(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "worker-01", Message: "w1 spawned"},
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "worker-02", Message: "w2 spawned"},
		{Timestamp: time.Now(), Type: events.WorkStarted, Agent: "worker-01", Message: "w1 working"},
	})

	// Reset the logsAgent flag before test
	logsAgent = "worker-01"
	defer func() { logsAgent = "" }()

	stdout, _, err := executeIntegrationCmd("logs", "--agent", "worker-01")
	if err != nil {
		t.Fatalf("logs --agent returned error: %v", err)
	}
	if !strings.Contains(stdout, "w1 spawned") {
		t.Errorf("expected worker-01 events, got: %s", stdout)
	}
	if strings.Contains(stdout, "w2 spawned") {
		t.Errorf("should not contain worker-02 events, got: %s", stdout)
	}
}

// --- Stats command tests ---

func TestStatsNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	_, _, err = executeIntegrationCmd("stats")
	// When bcd is running, stats works via API even without a local workspace.
	// When bcd is not running, should fail with workspace error.
	if err != nil && !strings.Contains(err.Error(), "not in a mycel workspace") {
		t.Errorf("expected either success (bcd running) or workspace error, got: %v", err)
	}
}

func TestStatsEmptyWorkspace(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Create agents dir
	if err := os.MkdirAll(filepath.Join(wsDir, ".bc", "agents"), 0750); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// Reset flags
	statsJSON = false
	statsSave = false

	stdout, _, err := executeIntegrationCmd("stats")
	if err != nil {
		t.Fatalf("workspace stats returned error: %v", err)
	}
	if !strings.Contains(stdout, "Workspace Stats") && !strings.Contains(stdout, "Stats") {
		t.Errorf("expected stats output, got: %s", stdout)
	}
}

func TestStatsSave(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(wsDir, ".bc", "agents"), 0750); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	statsSave = true
	statsJSON = false
	defer func() { statsSave = false }()

	stdout, _, err := executeIntegrationCmd("stats", "--save")
	if err != nil {
		t.Fatalf("workspace stats --save returned error: %v", err)
	}
	if !strings.Contains(stdout, "Stats saved") {
		t.Errorf("expected 'Stats saved' message, got: %s", stdout)
	}

	// Verify file was created
	statsPath := filepath.Join(wsDir, ".bc", "stats.json")
	if _, err := os.Stat(statsPath); os.IsNotExist(err) {
		t.Error("stats.json was not created")
	}
}

// --- Report command tests (with workspace) ---

func TestReportWorkingInWorkspace(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("worker"),
			State:     agent.StateIdle,
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	t.Setenv("BC_AGENT_ID", "test-agent")

	stdout, _, err := executeIntegrationCmd("agent", "report", "working", "fixing auth bug")
	if err != nil {
		t.Fatalf("report returned error: %v", err)
	}
	if !strings.Contains(stdout, "Reported: working fixing auth bug") {
		t.Errorf("expected report confirmation, got: %s", stdout)
	}
}

func TestReportDoneInWorkspace(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("worker"),
			State:     agent.StateWorking,
			Task:      "some task",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	t.Setenv("BC_AGENT_ID", "test-agent")

	stdout, _, err := executeIntegrationCmd("agent", "report", "done", "auth bug fixed")
	if err != nil {
		t.Fatalf("report returned error: %v", err)
	}
	if !strings.Contains(stdout, "Reported: done auth bug fixed") {
		t.Errorf("expected report confirmation, got: %s", stdout)
	}

	// Verify event was logged
	d, _, err := db.Global(nil)
	if err != nil {
		t.Fatalf("failed to open global db: %v", err)
	}
	evtLog, err := events.NewSQLiteLog(d)
	if err != nil {
		t.Fatalf("failed to open event log: %v", err)
	}
	defer func() { _ = evtLog.Close() }()
	evts, err := evtLog.Read()
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if len(evts) == 0 {
		t.Error("expected at least one event logged")
	}
	found := false
	for _, ev := range evts {
		if ev.Type == events.AgentReport && ev.Agent == "test-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected agent.report event for test-agent")
	}
}

func TestReportStuckInWorkspace(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("worker"),
			State:     agent.StateWorking,
			Task:      "some task",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	t.Setenv("BC_AGENT_ID", "test-agent")

	stdout, _, err := executeIntegrationCmd("agent", "report", "stuck", "need database credentials")
	if err != nil {
		t.Fatalf("report returned error: %v", err)
	}
	if !strings.Contains(stdout, "Reported: stuck") {
		t.Errorf("expected report confirmation, got: %s", stdout)
	}
}

// --- Version command tests ---

func TestVersionOutput(t *testing.T) {
	SetVersionInfo("2.0.0", "def456", "2025-06-01")
	defer SetVersionInfo("dev", "none", "unknown")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "2.0.0") {
		t.Errorf("Expected version '2.0.0', got: %s", output)
	}
	if !strings.Contains(output, "def456") {
		t.Errorf("Expected commit 'def456', got: %s", output)
	}
	if !strings.Contains(output, "2025-06-01") {
		t.Errorf("Expected date '2025-06-01', got: %s", output)
	}
}

// --- Status with agents written to disk ---

// --- JSON output tests ---

func TestLogsJSON(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "w-01", Message: "spawned"},
	})

	stdout, _, err := executeIntegrationCmd("logs", "--json")
	if err != nil {
		t.Fatalf("logs --json returned error: %v", err)
	}

	var evts []events.Event
	if err := json.Unmarshal([]byte(stdout), &evts); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, stdout)
	}
	if len(evts) != 1 {
		t.Errorf("expected 1 event, got %d", len(evts))
	}
}

func TestStatsJSON(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(wsDir, ".bc", "agents"), 0750); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	statsJSON = true
	statsSave = false
	defer func() { statsJSON = false }()

	stdout, _, err := executeIntegrationCmd("stats", "--json")
	if err != nil {
		t.Fatalf("stats --json returned error: %v", err)
	}

	// Should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, stdout)
	}
	// When bcd is running, API returns agents_total; otherwise local stats returns agents
	_, hasAgents := result["agents"]
	_, hasAgentsTotal := result["agents_total"]
	if !hasAgents && !hasAgentsTotal {
		t.Error("expected 'agents' or 'agents_total' key in JSON output")
	}
}

// --- Init command removal (epic #3265: `mycel up` bootstraps everything) ---

func TestInitCommandRemoved(t *testing.T) {
	_, _, err := executeIntegrationCmd("init")
	if err == nil {
		t.Fatal("expected unknown-command error for removed 'init' command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' error, got: %v", err)
	}
}

// --- Send command tests (more coverage) ---

func TestSendToStoppedAgent(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"worker-01": {
			Name:      "worker-01",
			Role:      agent.Role("worker"),
			State:     agent.StateStopped,
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	_, _, err := executeIntegrationCmd("agent", "send", "worker-01", "hello")
	if err == nil {
		t.Fatal("expected error for stopped agent, got nil")
	}
	if !strings.Contains(err.Error(), "stopped") {
		t.Errorf("expected 'stopped' error, got: %v", err)
	}
}

func TestStatusWithAgents(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"coordinator": {
			Name:      "coordinator",
			Role:      agent.RoleRoot,
			State:     agent.StateStopped,
			Session:   "bc-coord",
			StartedAt: time.Now().Add(-1 * time.Hour),
		},
		"worker-01": {
			Name:      "worker-01",
			Role:      agent.Role("worker"),
			State:     agent.StateStopped,
			Session:   "bc-worker-01",
			Task:      "fixing auth",
			StartedAt: time.Now().Add(-30 * time.Minute),
		},
	})

	stdout, _, err := executeIntegrationCmd("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	if !strings.Contains(stdout, "coordinator") {
		t.Errorf("expected 'coordinator' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "worker-01") {
		t.Errorf("expected 'worker-01' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "AGENT") {
		t.Errorf("expected table header, got: %s", stdout)
	}
}

// --- Cost command tests ---
// (Cost show/budget/usage tests are in cost_test.go and cost_usage_test.go)

// Note: Process command tests are in process_test.go
