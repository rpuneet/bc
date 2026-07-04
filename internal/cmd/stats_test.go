package cmd

import (
	"testing"
)

// resetStatsFlags resets stats command flags between tests.
func resetStatsFlags() {
	statsJSON = false
	statsSave = false
}

// --- Stats Command Unit Tests ---

func TestStats_Basic(t *testing.T) {
	setupTestWorkspace(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats should work in a workspace (even with no data)
	_, err := executeCmd("stats")
	if err != nil {
		t.Fatalf("workspace stats error: %v", err)
	}
}

func TestStats_JSON(t *testing.T) {
	setupTestWorkspace(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats --json should work
	_, err := executeCmd("stats", "--json")
	if err != nil {
		t.Fatalf("workspace stats --json error: %v", err)
	}
}

func TestStats_Save(t *testing.T) {
	setupTestWorkspace(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats --save should work
	_, err := executeCmd("stats", "--save")
	if err != nil {
		t.Fatalf("workspace stats --save error: %v", err)
	}
}

func TestStats_JSONAndSave(t *testing.T) {
	setupTestWorkspace(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Both flags together should work
	_, err := executeCmd("stats", "--json", "--save")
	if err != nil {
		t.Fatalf("workspace stats --json --save error: %v", err)
	}
}

// --- Stats Command Flags Tests ---

func TestStatsCommandFlags(t *testing.T) {
	flags := statsCmd.Flags()

	if flags.Lookup("json") == nil {
		t.Error("expected --json flag on workspace stats")
	}
	if flags.Lookup("save") == nil {
		t.Error("expected --save flag on workspace stats")
	}
}
