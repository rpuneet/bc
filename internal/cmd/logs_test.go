package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// setupLogsWorkspace creates a temporary bc workspace, changes into it,
// and returns the root dir plus a cleanup function.
func setupLogsWorkspace(t *testing.T) (string, func()) {
	t.Helper()

	if os.Getenv("BC_TEST_DAEMON") == "" {
		t.Skip("skipping: requires BC_TEST_DAEMON=1 (dedicated test bcd instance)")
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	ws, err := workspace.Init(tmpDir)
	if err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}
	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("failed to ensure dirs: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	return tmpDir, func() { _ = os.Chdir(origDir) }
}

// seedLogsEvents writes events to the workspace state.db SQLite database.
func seedLogsEvents(t *testing.T, wsDir string, evts []events.Event) {
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

// runLogsCmd executes the logs command capturing stdout. It resets all logs
// flags to defaults before execution to avoid leaking between tests.
func runLogsCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	// Reset all logs flags to defaults (both variable and cobra Changed state)
	logsAgent = ""
	logsTail = 0
	logsType = ""
	logsSince = ""
	logsFull = false
	logsCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

	origStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	// Also redirect pkg/ui output which uses its own writer
	ui.SetOutput(w)
	defer ui.SetOutput(os.Stdout)

	rootCmd.SetOut(w)
	rootCmd.SetErr(w)
	rootCmd.SetArgs(args)

	// Reset persistent flags too
	_ = rootCmd.PersistentFlags().Set("json", "false")

	err := rootCmd.Execute()

	_ = w.Close()
	var buf [64 * 1024]byte
	n, _ := r.Read(buf[:])
	os.Stdout = origStdout

	return string(buf[:n]), err
}

// --- truncateMessage tests ---

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		maxLen int
	}{
		{"short message unchanged", "hello", "hello", 80},
		{"exact length unchanged", "abcde", "abcde", 5},
		{"long message truncated", "this is a long message that exceeds the limit", "this is a long me...", 20},
		{"very short limit", "hello world", "hel", 3},
		{"empty string", "", "", 80},
		{"limit of 4", "abcdefgh", "a...", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateMessage(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateMessage(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// --- parseSinceDuration tests ---

func TestParseSinceDuration(t *testing.T) {
	t.Run("valid 1h", func(t *testing.T) {
		cutoff, err := parseSinceDuration("1h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// cutoff should be approximately 1 hour ago
		diff := time.Since(cutoff)
		if diff < 59*time.Minute || diff > 61*time.Minute {
			t.Errorf("expected ~1h ago, got %v ago", diff)
		}
	})

	t.Run("valid 30m", func(t *testing.T) {
		cutoff, err := parseSinceDuration("30m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		diff := time.Since(cutoff)
		if diff < 29*time.Minute || diff > 31*time.Minute {
			t.Errorf("expected ~30m ago, got %v ago", diff)
		}
	})

	t.Run("valid 2h30m", func(t *testing.T) {
		cutoff, err := parseSinceDuration("2h30m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		diff := time.Since(cutoff)
		expected := 2*time.Hour + 30*time.Minute
		if diff < expected-time.Minute || diff > expected+time.Minute {
			t.Errorf("expected ~2h30m ago, got %v ago", diff)
		}
	})

	t.Run("invalid duration", func(t *testing.T) {
		_, err := parseSinceDuration("bogus")
		if err == nil {
			t.Fatal("expected error for invalid duration")
		}
		if !strings.Contains(err.Error(), "invalid duration") {
			t.Errorf("expected 'invalid duration' in error, got: %v", err)
		}
	})
}

// --- Integration tests for --type flag ---

func TestLogs_TypeFilter(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "coord", Message: "spawned"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "working on auth"},
		{Timestamp: time.Now(), Type: events.WorkCompleted, Agent: "eng-01", Message: "auth done"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-02", Message: "working on tests"},
	})

	stdout, err := runLogsCmd(t, "logs", "--type", "agent.report")
	if err != nil {
		t.Fatalf("logs --type failed: %v", err)
	}

	if !strings.Contains(stdout, "agent.report") {
		t.Errorf("expected agent.report events, got: %s", stdout)
	}
	if strings.Contains(stdout, "agent.spawned") {
		t.Errorf("should not contain agent.spawned events, got: %s", stdout)
	}
	if strings.Contains(stdout, "work.completed") {
		t.Errorf("should not contain work.completed events, got: %s", stdout)
	}
}

func TestLogs_TypeFilter_NoMatch(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "coord", Message: "spawned"},
	})

	stdout, err := runLogsCmd(t, "logs", "--type", "work.failed")
	if err != nil {
		t.Fatalf("logs --type failed: %v", err)
	}

	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected 'No events found', got: %s", stdout)
	}
}

// --- Integration tests for --since flag ---

func TestLogs_SinceFilter(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-2 * time.Hour), Type: events.AgentSpawned, Agent: "coord", Message: "old event"},
		{Timestamp: time.Now().Add(-10 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "recent event"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.WorkCompleted, Agent: "eng-01", Message: "very recent"},
	})

	stdout, err := runLogsCmd(t, "logs", "--since", "30m")
	if err != nil {
		t.Fatalf("logs --since failed: %v", err)
	}

	if strings.Contains(stdout, "old event") {
		t.Errorf("should not contain old event from 2h ago, got: %s", stdout)
	}
	if !strings.Contains(stdout, "recent event") {
		t.Errorf("should contain recent event from 10m ago, got: %s", stdout)
	}
	if !strings.Contains(stdout, "very recent") {
		t.Errorf("should contain very recent event, got: %s", stdout)
	}
}

func TestLogs_SinceFilter_InvalidDuration(t *testing.T) {
	_, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	_, err := runLogsCmd(t, "logs", "--since", "notaduration")
	if err == nil {
		t.Fatal("expected error for invalid --since duration")
	}
	if !strings.Contains(err.Error(), "invalid duration") {
		t.Errorf("expected 'invalid duration' in error, got: %v", err)
	}
}

// --- Integration tests for message truncation ---

func TestLogs_MessageTruncation(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	longMsg := strings.Repeat("x", 120)
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: longMsg},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Should be truncated - the full 120-char message should not appear
	if strings.Contains(stdout, longMsg) {
		t.Error("long message should be truncated by default")
	}
	if !strings.Contains(stdout, "...") {
		t.Errorf("truncated message should end with '...', got: %s", stdout)
	}
}

func TestLogs_FullFlag_NoTruncation(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	longMsg := strings.Repeat("y", 120)
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: longMsg},
	})

	stdout, err := runLogsCmd(t, "logs", "--full")
	if err != nil {
		t.Fatalf("logs --full failed: %v", err)
	}

	if !strings.Contains(stdout, longMsg) {
		t.Errorf("--full should show complete message, got: %s", stdout)
	}
}

func TestLogs_ShortMessage_NoTruncation(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	shortMsg := "short message"
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: shortMsg},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, shortMsg) {
		t.Errorf("short message should appear in full, got: %s", stdout)
	}
}

// --- Composable filters tests ---

func TestLogs_AgentAndType(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "eng-01 report"},
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "eng-01", Message: "eng-01 spawned"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-02", Message: "eng-02 report"},
	})

	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report")
	if err != nil {
		t.Fatalf("logs --agent --type failed: %v", err)
	}

	if !strings.Contains(stdout, "eng-01 report") {
		t.Errorf("expected eng-01 report, got: %s", stdout)
	}
	if strings.Contains(stdout, "eng-01 spawned") {
		t.Errorf("should not contain eng-01 spawned event, got: %s", stdout)
	}
	if strings.Contains(stdout, "eng-02 report") {
		t.Errorf("should not contain eng-02 events, got: %s", stdout)
	}
}

func TestLogs_AgentAndTypeAndSince(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-2 * time.Hour), Type: events.AgentReport, Agent: "eng-01", Message: "old report"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "recent report"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentSpawned, Agent: "eng-01", Message: "recent spawn"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentReport, Agent: "eng-02", Message: "other agent"},
	})

	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report", "--since", "30m")
	if err != nil {
		t.Fatalf("logs --agent --type --since failed: %v", err)
	}

	if !strings.Contains(stdout, "recent report") {
		t.Errorf("expected recent report, got: %s", stdout)
	}
	if strings.Contains(stdout, "old report") {
		t.Errorf("should not contain old report, got: %s", stdout)
	}
	if strings.Contains(stdout, "recent spawn") {
		t.Errorf("should not contain spawn events, got: %s", stdout)
	}
	if strings.Contains(stdout, "other agent") {
		t.Errorf("should not contain other agent, got: %s", stdout)
	}
}

func TestLogs_AllFiltersComposed(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	// Seed 10 recent agent.report events from eng-01
	for i := 0; i < 10; i++ {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now().Add(-time.Duration(10-i) * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "report " + string(rune('A'+i))},
		})
	}
	// Seed some noise
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-3 * time.Hour), Type: events.AgentReport, Agent: "eng-01", Message: "old report"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentSpawned, Agent: "eng-01", Message: "spawn"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentReport, Agent: "eng-02", Message: "other"},
	})

	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report", "--since", "30m", "--tail", "3")
	if err != nil {
		t.Fatalf("logs with all filters failed: %v", err)
	}

	// Should not contain old, spawn, or other agent events
	if strings.Contains(stdout, "old report") {
		t.Errorf("should not contain old report")
	}
	if strings.Contains(stdout, "spawn") {
		t.Errorf("should not contain spawn events")
	}
	if strings.Contains(stdout, "other") {
		t.Errorf("should not contain other agent")
	}

	// Count the number of event lines
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	eventLines := 0
	for _, l := range lines {
		if strings.Contains(l, "agent.report") {
			eventLines++
		}
	}
	if eventLines != 3 {
		t.Errorf("expected 3 event lines with --tail 3, got %d\noutput: %s", eventLines, stdout)
	}
}

// --- Edge cases ---

func TestLogs_EmptyLog(t *testing.T) {
	_, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected 'No events found', got: %s", stdout)
	}
}

func TestLogs_TailOnly(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now().Add(-time.Duration(10-i) * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "event " + string(rune('A'+i))},
		})
	}

	stdout, err := runLogsCmd(t, "logs", "--tail", "2")
	if err != nil {
		t.Fatalf("logs --tail failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
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

func TestLogs_SinceFilter_AllOld(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-5 * time.Hour), Type: events.AgentReport, Agent: "eng-01", Message: "old"},
		{Timestamp: time.Now().Add(-4 * time.Hour), Type: events.AgentReport, Agent: "eng-02", Message: "also old"},
	})

	stdout, err := runLogsCmd(t, "logs", "--since", "1h")
	if err != nil {
		t.Fatalf("logs --since failed: %v", err)
	}

	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected 'No events found' when all events are old, got: %s", stdout)
	}
}

func TestLogs_TailRejectsNegativeAndZero(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{"zero", "0"},
		{"negative", "-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runLogsCmd(t, "logs", "--tail", tt.val)
			if err == nil {
				t.Fatal("expected error for --tail " + tt.val + ", got nil")
			}
			if !strings.Contains(err.Error(), "tail must be a positive number") {
				t.Errorf("expected 'tail must be a positive number', got: %v", err)
			}
		})
	}
}

// --- Agent filter comprehensive tests ---

func TestLogs_AgentFilter(t *testing.T) {
	tests := []struct {
		name          string
		agents        []string // agents to seed events for
		filterAgent   string
		expectAgents  []string // agents that should appear
		excludeAgents []string // agents that should NOT appear
	}{
		{
			name:          "filter single agent",
			agents:        []string{"eng-01", "eng-02", "eng-03"},
			filterAgent:   "eng-01",
			expectAgents:  []string{"eng-01"},
			excludeAgents: []string{"eng-02", "eng-03"},
		},
		{
			name:          "filter nonexistent agent",
			agents:        []string{"eng-01", "eng-02"},
			filterAgent:   "eng-99",
			expectAgents:  []string{},
			excludeAgents: []string{"eng-01", "eng-02"},
		},
		{
			name:          "case sensitive agent filter",
			agents:        []string{"Eng-01", "eng-01"},
			filterAgent:   "eng-01",
			expectAgents:  []string{"eng-01"},
			excludeAgents: []string{"Eng-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsDir, cleanup := setupLogsWorkspace(t)
			defer cleanup()

			// Seed events for each agent
			for _, agent := range tt.agents {
				seedLogsEvents(t, wsDir, []events.Event{
					{Timestamp: time.Now(), Type: events.AgentReport, Agent: agent, Message: "msg from " + agent},
				})
			}

			stdout, err := runLogsCmd(t, "logs", "--agent", tt.filterAgent)
			if err != nil {
				t.Fatalf("logs --agent failed: %v", err)
			}

			for _, a := range tt.expectAgents {
				if !strings.Contains(stdout, a) {
					t.Errorf("expected agent %q in output, got: %s", a, stdout)
				}
			}
			for _, a := range tt.excludeAgents {
				if strings.Contains(stdout, "msg from "+a) {
					t.Errorf("should not contain agent %q, got: %s", a, stdout)
				}
			}

			if len(tt.expectAgents) == 0 && !strings.Contains(stdout, "No events found") {
				t.Errorf("expected 'No events found' for nonexistent agent")
			}
		})
	}
}

func TestLogs_AgentFilter_SpecialNames(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
	}{
		{"hyphenated", "eng-01-test"},
		{"underscore", "eng_01"},
		{"numbers", "eng123"},
		{"mixed", "Eng-01_Test-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsDir, cleanup := setupLogsWorkspace(t)
			defer cleanup()

			seedLogsEvents(t, wsDir, []events.Event{
				{Timestamp: time.Now(), Type: events.AgentReport, Agent: tt.agentName, Message: "test message"},
			})

			stdout, err := runLogsCmd(t, "logs", "--agent", tt.agentName)
			if err != nil {
				t.Fatalf("logs --agent failed: %v", err)
			}

			if !strings.Contains(stdout, tt.agentName) {
				t.Errorf("expected agent %q in output, got: %s", tt.agentName, stdout)
			}
		})
	}
}

// --- JSON output tests ---

func TestLogs_JSONOutput(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "eng-01", Message: "spawned"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "working"},
	})

	stdout, err := runLogsCmd(t, "logs", "--json")
	if err != nil {
		t.Fatalf("logs --json failed: %v", err)
	}

	// Verify valid JSON
	var evts []events.Event
	if unmarshalErr := json.Unmarshal([]byte(stdout), &evts); unmarshalErr != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", unmarshalErr, stdout)
	}

	if len(evts) != 2 {
		t.Errorf("expected 2 events in JSON, got %d", len(evts))
	}
}

func TestLogs_JSONOutput_Empty(t *testing.T) {
	_, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	stdout, err := runLogsCmd(t, "logs", "--json")
	if err != nil {
		t.Fatalf("logs --json failed: %v", err)
	}

	// Empty should still be valid (either empty array or "No events found")
	if strings.Contains(stdout, "No events found") {
		return // This is acceptable
	}

	var evts []events.Event
	if unmarshalErr := json.Unmarshal([]byte(stdout), &evts); unmarshalErr != nil {
		t.Fatalf("invalid JSON output for empty: %v", unmarshalErr)
	}
}

func TestLogs_JSONOutput_WithFilters(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "eng-01", Message: "spawned"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "report1"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-02", Message: "report2"},
	})

	stdout, err := runLogsCmd(t, "logs", "--json", "--agent", "eng-01", "--type", "agent.report")
	if err != nil {
		t.Fatalf("logs --json with filters failed: %v", err)
	}

	var evts []events.Event
	if unmarshalErr := json.Unmarshal([]byte(stdout), &evts); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v", unmarshalErr)
	}

	if len(evts) != 1 {
		t.Errorf("expected 1 filtered event, got %d", len(evts))
	}
	if len(evts) > 0 && evts[0].Agent != "eng-01" {
		t.Errorf("expected agent eng-01, got %s", evts[0].Agent)
	}
}

// --- Event type tests ---

func TestLogs_TypeFilter_AllTypes(t *testing.T) {
	types := []events.EventType{
		events.AgentSpawned,
		events.AgentStopped,
		events.AgentReport,
		events.WorkAssigned,
		events.WorkCompleted,
		events.WorkFailed,
		events.MessageSent,
	}

	for _, eventType := range types {
		t.Run(string(eventType), func(t *testing.T) {
			wsDir, cleanup := setupLogsWorkspace(t)
			defer cleanup()

			// Seed one event of each type
			for _, et := range types {
				seedLogsEvents(t, wsDir, []events.Event{
					{Timestamp: time.Now(), Type: et, Agent: "eng-01", Message: "test " + string(et)},
				})
			}

			stdout, err := runLogsCmd(t, "logs", "--type", string(eventType))
			if err != nil {
				t.Fatalf("logs --type %s failed: %v", eventType, err)
			}

			if !strings.Contains(stdout, string(eventType)) {
				t.Errorf("expected type %s in output, got: %s", eventType, stdout)
			}

			// Verify other types are not present
			for _, et := range types {
				if et != eventType && strings.Contains(stdout, "test "+string(et)) {
					t.Errorf("should not contain type %s, got: %s", et, stdout)
				}
			}
		})
	}
}

// --- Ordering tests ---

func TestLogs_Ordering_Chronological(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	// Seed events in specific order
	now := time.Now()
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: now.Add(-3 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "first"},
		{Timestamp: now.Add(-2 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "second"},
		{Timestamp: now.Add(-1 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "third"},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Verify order: first should appear before second, second before third
	firstIdx := strings.Index(stdout, "first")
	secondIdx := strings.Index(stdout, "second")
	thirdIdx := strings.Index(stdout, "third")

	if firstIdx == -1 || secondIdx == -1 || thirdIdx == -1 {
		t.Fatalf("missing events in output: %s", stdout)
	}

	if firstIdx >= secondIdx || secondIdx >= thirdIdx {
		t.Errorf("events not in chronological order: first=%d, second=%d, third=%d\noutput: %s",
			firstIdx, secondIdx, thirdIdx, stdout)
	}
}

func TestLogs_Ordering_TailReversed(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	// Seed 10 events
	for i := 0; i < 10; i++ {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now().Add(-time.Duration(10-i) * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "event" + string(rune('A'+i))},
		})
	}

	stdout, err := runLogsCmd(t, "logs", "--tail", "3")
	if err != nil {
		t.Fatalf("logs --tail failed: %v", err)
	}

	// Should have last 3 events: H, I, J (indices 7, 8, 9)
	if !strings.Contains(stdout, "eventH") || !strings.Contains(stdout, "eventI") || !strings.Contains(stdout, "eventJ") {
		t.Errorf("expected last 3 events (H, I, J), got: %s", stdout)
	}

	// Should NOT have earlier events
	if strings.Contains(stdout, "eventA") || strings.Contains(stdout, "eventG") {
		t.Errorf("should not contain early events, got: %s", stdout)
	}
}

// --- Message formatting tests ---

func TestLogs_MessageSent_Format(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{
			Timestamp: time.Now(),
			Type:      events.MessageSent,
			Agent:     "eng-01",
			Message:   "hello there",
			Data:      map[string]interface{}{"recipient": "eng-02"},
		},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Should show sender → recipient format
	if !strings.Contains(stdout, "[eng-01]") || !strings.Contains(stdout, "[eng-02]") {
		t.Errorf("expected [eng-01] → [eng-02] format, got: %s", stdout)
	}
	if !strings.Contains(stdout, "→") {
		t.Errorf("expected arrow in message.sent output, got: %s", stdout)
	}
}

func TestLogs_MessageSent_NoRecipient(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{
			Timestamp: time.Now(),
			Type:      events.MessageSent,
			Agent:     "eng-01",
			Message:   "broadcast",
			Data:      map[string]interface{}{}, // No recipient
		},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Should just show sender without arrow
	if !strings.Contains(stdout, "[eng-01]") {
		t.Errorf("expected [eng-01], got: %s", stdout)
	}
}

// --- Unicode and special characters ---

func TestLogs_UnicodeMessages(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"chinese", "你好世界"},
		{"japanese", "こんにちは"},
		{"emoji", "Working on task 🚀✅"},
		{"arabic", "مرحبا بالعالم"},
		{"mixed", "Hello 世界 🎉 مرحبا"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsDir, cleanup := setupLogsWorkspace(t)
			defer cleanup()

			seedLogsEvents(t, wsDir, []events.Event{
				{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: tt.message},
			})

			stdout, err := runLogsCmd(t, "logs")
			if err != nil {
				t.Fatalf("logs failed: %v", err)
			}

			if !strings.Contains(stdout, tt.message) {
				t.Errorf("expected unicode message %q in output, got: %s", tt.message, stdout)
			}
		})
	}
}

func TestLogs_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"quotes", `He said "hello"`},
		{"backslash", `path\to\file`},
		{"newlines_stripped", "line1"},
		{"tabs", "col1\tcol2"},
		{"brackets", "[INFO] message {data}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsDir, cleanup := setupLogsWorkspace(t)
			defer cleanup()

			seedLogsEvents(t, wsDir, []events.Event{
				{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: tt.message},
			})

			stdout, err := runLogsCmd(t, "logs")
			if err != nil {
				t.Fatalf("logs failed: %v", err)
			}

			// Just verify it doesn't crash and produces output
			if stdout == "" {
				t.Error("expected non-empty output")
			}
		})
	}
}

// --- Boundary tests ---

func TestLogs_TailExceedsEventCount(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	// Seed 3 events
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "event1"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "event2"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "event3"},
	})

	// Request tail 100 (more than available)
	stdout, err := runLogsCmd(t, "logs", "--tail", "100")
	if err != nil {
		t.Fatalf("logs --tail failed: %v", err)
	}

	// Should return all 3 events
	if !strings.Contains(stdout, "event1") || !strings.Contains(stdout, "event2") || !strings.Contains(stdout, "event3") {
		t.Errorf("expected all 3 events, got: %s", stdout)
	}
}

func TestLogs_SingleEvent(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "only event"},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "only event") {
		t.Errorf("expected single event, got: %s", stdout)
	}
}

func TestLogs_ManyEvents(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	// Seed 100 events
	for i := 0; i < 100; i++ {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now().Add(-time.Duration(100-i) * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "event"},
		})
	}

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Count event lines
	lines := strings.Split(stdout, "\n")
	eventCount := 0
	for _, l := range lines {
		if strings.Contains(l, "agent.report") {
			eventCount++
		}
	}

	if eventCount != 100 {
		t.Errorf("expected 100 events, got %d", eventCount)
	}
}

// --- Duration parsing edge cases ---

func TestParseSinceDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"seconds", "30s", false},
		{"days_as_hours", "24h", false},
		{"milliseconds", "100ms", false},
		{"combined_hms", "1h30m45s", false},
		{"empty", "", true},
		{"just_number", "30", true},
		{"negative", "-1h", false}, // time.ParseDuration accepts negative
		{"invalid_unit", "1d", true},
		{"invalid_format", "one hour", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSinceDuration(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("expected error for %q, got nil", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
			}
		})
	}
}

// --- Error path tests ---

func TestLogs_NoWorkspace(t *testing.T) {
	// Run from temp dir without workspace
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	// Ensure BC_WORKSPACE doesn't bypass the workspace check
	t.Setenv("BC_WORKSPACE", "")

	_, err := runLogsCmd(t, "logs")
	if err == nil {
		t.Fatal("expected error for non-workspace dir")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

// --- Combined filter tests ---

func TestLogs_CombinedFilters_AllMatch(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	now := time.Now()
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: now.Add(-5 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "match"},
	})

	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report", "--since", "10m")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "match") {
		t.Errorf("expected match, got: %s", stdout)
	}
}

func TestLogs_CombinedFilters_NoneMatch(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	now := time.Now()
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: now.Add(-2 * time.Hour), Type: events.AgentSpawned, Agent: "eng-02", Message: "event"},
	})

	// All filters mismatch
	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report", "--since", "1h")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected 'No events found', got: %s", stdout)
	}
}

func TestLogs_FilterOrder_AgentThenType(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "report from eng-01"},
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "eng-01", Message: "spawn from eng-01"},
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-02", Message: "report from eng-02"},
	})

	stdout, err := runLogsCmd(t, "logs", "--agent", "eng-01", "--type", "agent.report")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "report from eng-01") {
		t.Errorf("expected 'report from eng-01', got: %s", stdout)
	}
	if strings.Contains(stdout, "spawn from eng-01") || strings.Contains(stdout, "report from eng-02") {
		t.Errorf("should not contain filtered out events, got: %s", stdout)
	}
}

// --- Additional truncation tests ---

func TestTruncateMessage_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		maxLen int
	}{
		{"limit 0", "hello", "", 0},
		{"limit 1", "hello", "h", 1},
		{"limit 2", "hello", "he", 2},
		// Note: truncateMessage uses byte length, not rune length
		// Unicode chars are multi-byte, so results may have partial chars
		{"ascii_short", "abc", "ab", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateMessage(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateMessage(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// --- Timestamp format tests ---

func TestLogs_TimestampFormat(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC), Type: events.AgentReport, Agent: "eng-01", Message: "test"},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	// Should show HH:MM:SS format (converted to local time)
	if !strings.Contains(stdout, ":") {
		t.Errorf("expected timestamp with colons, got: %s", stdout)
	}
}

// --- Additional filter edge cases ---

func TestLogs_TypeFilter_InvalidType(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "test"},
	})

	stdout, err := runLogsCmd(t, "logs", "--type", "invalid.type")
	if err != nil {
		t.Fatalf("logs --type failed: %v", err)
	}

	if !strings.Contains(stdout, "No events found") {
		t.Errorf("expected no events for invalid type, got: %s", stdout)
	}
}

func TestLogs_SinceFilter_VeryShort(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-1 * time.Second), Type: events.AgentReport, Agent: "eng-01", Message: "recent"},
	})

	stdout, err := runLogsCmd(t, "logs", "--since", "5s")
	if err != nil {
		t.Fatalf("logs --since failed: %v", err)
	}

	if !strings.Contains(stdout, "recent") {
		t.Errorf("expected recent event, got: %s", stdout)
	}
}

func TestLogs_SinceFilter_VeryLong(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-100 * time.Hour), Type: events.AgentReport, Agent: "eng-01", Message: "old"},
	})

	stdout, err := runLogsCmd(t, "logs", "--since", "1000h")
	if err != nil {
		t.Fatalf("logs --since failed: %v", err)
	}

	if !strings.Contains(stdout, "old") {
		t.Errorf("expected old event with long since, got: %s", stdout)
	}
}

// --- Tail edge cases ---

func TestLogs_TailOne(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-3 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "first"},
		{Timestamp: time.Now().Add(-2 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "second"},
		{Timestamp: time.Now().Add(-1 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "third"},
	})

	stdout, err := runLogsCmd(t, "logs", "--tail", "1")
	if err != nil {
		t.Fatalf("logs --tail failed: %v", err)
	}

	if !strings.Contains(stdout, "third") {
		t.Errorf("expected last event 'third', got: %s", stdout)
	}
	if strings.Contains(stdout, "first") || strings.Contains(stdout, "second") {
		t.Errorf("should only contain last event, got: %s", stdout)
	}
}

func TestLogs_TailLarge(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: "only"},
	})

	stdout, err := runLogsCmd(t, "logs", "--tail", "9999")
	if err != nil {
		t.Fatalf("logs --tail failed: %v", err)
	}

	if !strings.Contains(stdout, "only") {
		t.Errorf("expected single event, got: %s", stdout)
	}
}

// --- Full flag tests ---

func TestLogs_FullFlag_VeryLongMessage(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	longMsg := strings.Repeat("x", 500)
	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentReport, Agent: "eng-01", Message: longMsg},
	})

	stdout, err := runLogsCmd(t, "logs", "--full")
	if err != nil {
		t.Fatalf("logs --full failed: %v", err)
	}

	if len(stdout) < 500 {
		t.Errorf("expected full 500+ char message, got %d chars", len(stdout))
	}
}

// --- JSON with all flags ---

func TestLogs_JSON_WithTail(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now().Add(-time.Duration(5-i) * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "event"},
		})
	}

	stdout, err := runLogsCmd(t, "logs", "--json", "--tail", "2")
	if err != nil {
		t.Fatalf("logs --json --tail failed: %v", err)
	}

	var evts []events.Event
	if unmarshalErr := json.Unmarshal([]byte(stdout), &evts); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v", unmarshalErr)
	}

	if len(evts) != 2 {
		t.Errorf("expected 2 events, got %d", len(evts))
	}
}

func TestLogs_JSON_WithSince(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now().Add(-2 * time.Hour), Type: events.AgentReport, Agent: "eng-01", Message: "old"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Type: events.AgentReport, Agent: "eng-01", Message: "new"},
	})

	stdout, err := runLogsCmd(t, "logs", "--json", "--since", "1h")
	if err != nil {
		t.Fatalf("logs --json --since failed: %v", err)
	}

	var evts []events.Event
	if unmarshalErr := json.Unmarshal([]byte(stdout), &evts); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v", unmarshalErr)
	}

	if len(evts) != 1 {
		t.Errorf("expected 1 event after since filter, got %d", len(evts))
	}
}

// --- Event with metadata ---

func TestLogs_EventWithData(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{
			Timestamp: time.Now(),
			Type:      events.WorkAssigned,
			Agent:     "eng-01",
			Message:   "task assigned",
			Data:      map[string]any{"task_id": 123, "priority": "high"},
		},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "task assigned") {
		t.Errorf("expected task message, got: %s", stdout)
	}
}

func TestLogs_EventWithData_JSON(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{
			Timestamp: time.Now(),
			Type:      events.WorkAssigned,
			Agent:     "eng-01",
			Message:   "task assigned",
			Data:      map[string]any{"task_id": 123},
		},
	})

	stdout, err := runLogsCmd(t, "logs", "--json")
	if err != nil {
		t.Fatalf("logs --json failed: %v", err)
	}

	if !strings.Contains(stdout, "task_id") {
		t.Errorf("expected data field in JSON, got: %s", stdout)
	}
}

// --- Multiple agents ---

func TestLogs_MultipleAgents(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	agents := []string{"eng-01", "eng-02", "eng-03", "manager-01", "tech-lead-01"}
	for _, agent := range agents {
		seedLogsEvents(t, wsDir, []events.Event{
			{Timestamp: time.Now(), Type: events.AgentReport, Agent: agent, Message: "report from " + agent},
		})
	}

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	for _, agent := range agents {
		if !strings.Contains(stdout, agent) {
			t.Errorf("expected agent %s in output", agent)
		}
	}
}

// --- Empty message ---

func TestLogs_EmptyMessage(t *testing.T) {
	wsDir, cleanup := setupLogsWorkspace(t)
	defer cleanup()

	seedLogsEvents(t, wsDir, []events.Event{
		{Timestamp: time.Now(), Type: events.AgentSpawned, Agent: "eng-01", Message: ""},
	})

	stdout, err := runLogsCmd(t, "logs")
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}

	if !strings.Contains(stdout, "agent.spawned") {
		t.Errorf("expected event type, got: %s", stdout)
	}
}
