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
	setupTestHome(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats should work in a repo (even with no data)
	_, err := executeCmd("stats")
	if err != nil {
		t.Fatalf("stats error: %v", err)
	}
}

func TestStats_JSON(t *testing.T) {
	setupTestHome(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats --json should work
	_, err := executeCmd("stats", "--json")
	if err != nil {
		t.Fatalf("stats --json error: %v", err)
	}
}

func TestStats_Save(t *testing.T) {
	setupTestHome(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Stats --save should work
	_, err := executeCmd("stats", "--save")
	if err != nil {
		t.Fatalf("stats --save error: %v", err)
	}
}

func TestStats_JSONAndSave(t *testing.T) {
	setupTestHome(t)
	resetStatsFlags()
	defer resetStatsFlags()

	// Both flags together should work
	_, err := executeCmd("stats", "--json", "--save")
	if err != nil {
		t.Fatalf("stats --json --save error: %v", err)
	}
}

// --- Stats Command Flags Tests ---

func TestStatsCommandFlags(t *testing.T) {
	flags := statsCmd.Flags()

	if flags.Lookup("json") == nil {
		t.Error("expected --json flag on stats")
	}
	if flags.Lookup("save") == nil {
		t.Error("expected --save flag on stats")
	}
}
