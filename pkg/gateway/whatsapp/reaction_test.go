package whatsapp

import (
	"context"
	"strings"
	"testing"
)

// TestSendReactionNotConnected verifies the adapter rejects reactions when not paired.
func TestSendReactionNotConnected(t *testing.T) {
	a := New(t.TempDir())
	err := a.SendReaction(context.Background(),
		"1234@g.us",
		"9876543210@s.whatsapp.net",
		"msg-abc",
		"👍",
	)
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("SendReaction on disconnected adapter = %v, want not-connected error", err)
	}
}
