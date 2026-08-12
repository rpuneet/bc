package notify

import (
	"context"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
)

// TestStore_IndexesPresent asserts the schema self-creates the indexes the
// hot notify read/aggregation paths depend on. Missing any of these turns a
// SEARCH into a SCAN (message history, delivery activity) or reintroduces a
// GROUP BY temp b-tree (ChannelStats top-senders).
func TestStore_IndexesPresent(t *testing.T) {
	store := setupTestStore(t)
	want := []string{
		"idx_notify_subs_channel",
		"idx_notify_subs_agent",
		"idx_notify_delivery_channel",
		"idx_notify_messages_channel",
		"idx_notify_messages_chan_sender",
	}
	for _, name := range want {
		var got string
		err := store.db.QueryRowContext(context.Background(),
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", name).Scan(&got)
		if err != nil {
			t.Errorf("expected index %q to exist: %v", name, err)
		}
	}
}

func TestStore_MigratesSkippedDeliveryStatus(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	_, err = d.ExecContext(context.Background(), `
CREATE TABLE notify_delivery_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT
)`)
	if err != nil {
		t.Fatalf("seed old table: %v", err)
	}

	store, err := OpenStore(d, "sqlite")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if err := store.LogDelivery(context.Background(), DeliveryEntry{
		Channel: "slack:eng",
		Agent:   "bot",
		Status:  StatusSkipped,
		Error:   "agent bot is stopped",
		Preview: "hi",
	}); err != nil {
		t.Fatalf("LogDelivery skipped: %v", err)
	}
}

// TestGetMessages_CursorPagination verifies message history returns a bounded,
// newest-first page and that before=<id> walks strictly older pages.
func TestGetMessages_CursorPagination(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "slack:eng"
	for i := 0; i < 30; i++ {
		if err := store.SaveMessage(ctx, channel, "alice", "", "m"); err != nil {
			t.Fatalf("SaveMessage %d: %v", i, err)
		}
	}

	page, err := store.GetMessages(ctx, channel, 10, 0)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(page) != 10 {
		t.Fatalf("first page = %d, want 10", len(page))
	}
	// Newest first: IDs descending.
	if page[0].ID <= page[len(page)-1].ID {
		t.Error("GetMessages should return newest first (descending IDs)")
	}

	cursor := page[len(page)-1].ID
	older, err := store.GetMessages(ctx, channel, 10, cursor)
	if err != nil {
		t.Fatalf("GetMessages older: %v", err)
	}
	if len(older) != 10 {
		t.Fatalf("older page = %d, want 10", len(older))
	}
	for _, m := range older {
		if m.ID >= cursor {
			t.Errorf("older page leaked id %d >= cursor %d", m.ID, cursor)
		}
	}
}

// TestRecentActivity_CursorPagination verifies the delivery-log activity feed
// is bounded, newest-first, and pages older entries via before=<id>.
func TestRecentActivity_CursorPagination(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "slack:eng"
	for i := 0; i < 20; i++ {
		if err := store.LogDelivery(ctx, DeliveryEntry{
			Channel: channel,
			Agent:   "eng-01",
			Status:  StatusDelivered,
			Preview: "p",
		}); err != nil {
			t.Fatalf("LogDelivery %d: %v", i, err)
		}
	}

	page, err := store.RecentActivity(ctx, channel, 5, 0)
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(page) != 5 {
		t.Fatalf("first page = %d, want 5", len(page))
	}
	if page[0].ID <= page[len(page)-1].ID {
		t.Error("RecentActivity should return newest first")
	}

	cursor := page[len(page)-1].ID
	older, err := store.RecentActivity(ctx, channel, 5, cursor)
	if err != nil {
		t.Fatalf("RecentActivity older: %v", err)
	}
	for _, e := range older {
		if e.ID >= cursor {
			t.Errorf("older page leaked id %d >= cursor %d", e.ID, cursor)
		}
	}
}
