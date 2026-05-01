package notify

import (
	"context"
	"testing"
)

// TestSaveChannel_PreservesPlatformID asserts that an existing non-empty
// platform_id is not clobbered by a later SaveChannel call carrying a
// fallback (e.g., the channel name when the real platform-specific ID
// could not be extracted from the inbound payload).
func TestSaveChannel_PreservesPlatformID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const bcChannel = "telegram:general"
	const platform = "telegram"
	const goodID = "123456789" // numeric chat_id
	const fallbackID = "general"

	// 1. First write: real platform_id stored.
	if err := store.SaveChannel(ctx, bcChannel, platform, goodID); err != nil {
		t.Fatalf("SaveChannel (initial): %v", err)
	}

	// 2. Second write with a fallback platform_id. Must NOT overwrite.
	if err := store.SaveChannel(ctx, bcChannel, platform, fallbackID); err != nil {
		t.Fatalf("SaveChannel (fallback): %v", err)
	}

	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if got := channels[0].PlatformID; got != goodID {
		t.Fatalf("platform_id was clobbered: got %q, want %q", got, goodID)
	}
}

// TestSaveChannel_FillsEmptyPlatformID asserts that if the existing row has
// an empty platform_id, the next write does populate it.
func TestSaveChannel_FillsEmptyPlatformID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const bcChannel = "slack:general"
	const platform = "slack"

	// 1. First write: empty platform_id.
	if err := store.SaveChannel(ctx, bcChannel, platform, ""); err != nil {
		t.Fatalf("SaveChannel (empty): %v", err)
	}

	// 2. Second write with a real ID. Must populate the previously-empty value.
	if err := store.SaveChannel(ctx, bcChannel, platform, "C0123ABC"); err != nil {
		t.Fatalf("SaveChannel (real): %v", err)
	}

	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if got := channels[0].PlatformID; got != "C0123ABC" {
		t.Fatalf("platform_id not filled: got %q, want %q", got, "C0123ABC")
	}
}
