package cmd

import (
	"testing"
)

// --- Status Command Unit Tests ---

func TestStatus_Basic(t *testing.T) {
	setupTestHome(t)

	// Status should work in a repo (even with no agents)
	_, err := executeCmd("status")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
}

func TestStatus_JSON(t *testing.T) {
	setupTestHome(t)

	// Status --json should work
	_, err := executeCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json error: %v", err)
	}
}

func TestStatus_CommandFlags(t *testing.T) {
	flags := statusCmd.Flags()

	if flags.Lookup("json") == nil {
		t.Error("expected --json flag on status")
	}
}
