package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// uniqueChannelName returns a channel name unique to the current test.
// This prevents 409 "channel already exists" errors when tests run against
// a shared daemon with persistent state.
func uniqueChannelName(t *testing.T, suffix string) string {
	t.Helper()
	// Use a short hash of t.Name() to keep names short but unique
	name := strings.ReplaceAll(t.Name(), "/", "-")
	// Channel names must be lowercase alphanumeric with hyphens
	name = strings.ToLower(name)
	if suffix != "" {
		name = fmt.Sprintf("%s-%s", name, suffix)
	}
	// Truncate to reasonable length
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

// --- Channel Lifecycle E2E Tests ---

func TestChannelLifecycle_ListEmpty(t *testing.T) {
	setupTestHome(t)

	// Channel list should succeed even with no channels
	_, err := executeCmd("channel", "list")
	if err != nil {
		t.Fatalf("channel list error: %v", err)
	}
}

func TestChannelLifecycle_ListJSON(t *testing.T) {
	setupTestHome(t)

	// Channel list --json should succeed even with no channels
	_, err := executeCmd("channel", "list", "--json")
	if err != nil {
		t.Fatalf("channel list --json error: %v", err)
	}
}

func TestChannelLifecycle_CreateAndList(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// List channels should now show the new channel
	_, err = executeCmd("channel", "list")
	if err != nil {
		t.Fatalf("channel list error: %v", err)
	}
}

func TestChannelLifecycle_CreateDuplicate(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Create duplicate should fail
	_, err = executeCmd("channel", "create", ch)
	if err == nil {
		t.Error("expected error for duplicate channel")
	}
	if err != nil && !strings.Contains(err.Error(), "exists") {
		t.Errorf("expected 'exists' error, got: %v", err)
	}
}

func TestChannelLifecycle_CreateEmptyName(t *testing.T) {
	setupTestHome(t)

	// Create with empty name should fail
	_, err := executeCmd("channel", "create", "")
	if err == nil {
		t.Error("expected error for empty channel name")
	}
}

func TestChannelLifecycle_AddMember(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Add a member
	_, err = executeCmd("channel", "add", ch, "agent-01")
	if err != nil {
		t.Fatalf("channel add error: %v", err)
	}
}

func TestChannelLifecycle_AddMemberToNonexistent(t *testing.T) {
	setupTestHome(t)

	// Add member to non-existent channel prints a warning but doesn't error
	// Command returns success but with 0 members added
	_, err := executeCmd("channel", "add", "nonexistent-channel-e2e-xyz", "agent-01")
	if err != nil {
		t.Fatalf("channel add should not error (shows warning instead): %v", err)
	}
}

func TestChannelLifecycle_RemoveMember(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel and add a member
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	_, err = executeCmd("channel", "add", ch, "agent-01")
	if err != nil {
		t.Fatalf("channel add error: %v", err)
	}

	// Remove the member
	_, err = executeCmd("channel", "remove", ch, "agent-01")
	if err != nil {
		t.Fatalf("channel remove error: %v", err)
	}
}

func TestChannelLifecycle_RemoveMemberNotInChannel(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Remove a member that's not in the channel should fail
	_, err = executeCmd("channel", "remove", ch, "agent-01")
	if err == nil {
		t.Error("expected error for removing non-member")
	}
}

func TestChannelLifecycle_Delete(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}

	// Delete the channel
	_, err = executeCmd("channel", "delete", ch)
	if err != nil {
		t.Fatalf("channel delete error: %v", err)
	}
}

func TestChannelLifecycle_DeleteNonexistent(t *testing.T) {
	setupTestHome(t)

	// Delete non-existent channel should fail
	_, err := executeCmd("channel", "delete", "nonexistent-e2e-xyz")
	if err == nil {
		t.Error("expected error for deleting non-existent channel")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestChannelLifecycle_History(t *testing.T) {
	setupTestHome(t)

	ch := uniqueChannelName(t, "")
	// Create a channel
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// History should work (even if empty)
	_, err = executeCmd("channel", "history", ch)
	if err != nil {
		t.Fatalf("channel history error: %v", err)
	}
}

func TestChannelLifecycle_HistoryNonexistent(t *testing.T) {
	setupTestHome(t)

	// History of non-existent channel should fail
	_, err := executeCmd("channel", "history", "nonexistent-e2e-xyz")
	if err == nil {
		t.Error("expected error for history of non-existent channel")
	}
}

// --- Channel Command Structure Tests ---

func TestChannelCommandStructure(t *testing.T) {
	// Verify channelCmd has expected subcommands
	subcommands := channelCmd.Commands()

	expectedCmds := map[string]bool{
		"create":  false,
		"list":    false,
		"add":     false,
		"remove":  false,
		"send":    false,
		"delete":  false,
		"join":    false,
		"leave":   false,
		"history": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestChannelListFlags(t *testing.T) {
	flags := channelListCmd.Flags()

	if flags.Lookup("json") == nil {
		t.Error("expected --json flag on channel list")
	}
}
