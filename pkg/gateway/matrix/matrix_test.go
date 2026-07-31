package matrix

import (
	"bytes"
	"testing"
)

// TestParseSync verifies that a /sync response yields Content from
// content.body, keeps only m.room.message events, skips the bot's own
// messages, and preserves the full raw event JSON.
func TestParseSync(t *testing.T) {
	body := []byte(`{
		"next_batch":"s2",
		"rooms":{"join":{"!room:host":{"timeline":{"events":[
			{"type":"m.room.message","sender":"@alice:host","content":{"body":"hello world","msgtype":"m.text"}},
			{"type":"m.room.member","sender":"@bob:host","content":{"membership":"join"}},
			{"type":"m.room.message","sender":"@bot:host","content":{"body":"my own echo"}}
		]}}}}
	}`)

	notes, nextBatch, err := parseSync(body, "@bot:host")
	if err != nil {
		t.Fatalf("parseSync: %v", err)
	}
	if nextBatch != "s2" {
		t.Errorf("nextBatch = %q, want s2", nextBatch)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notifications, want 1 (member event and self-echo must be dropped)", len(notes))
	}
	n := notes[0]
	if n.Content != "hello world" {
		t.Errorf("Content = %q, want %q", n.Content, "hello world")
	}
	if n.Sender != "@alice:host" {
		t.Errorf("Sender = %q, want @alice:host", n.Sender)
	}
	if n.Channel != "!room:host" {
		t.Errorf("Channel = %q, want !room:host", n.Channel)
	}
	// Raw must preserve the FULL event, including nested content fields.
	if !bytes.Contains(n.Raw, []byte(`"msgtype":"m.text"`)) {
		t.Errorf("Raw does not preserve full event: %s", n.Raw)
	}
	if !bytes.Contains(n.Raw, []byte(`"type":"m.room.message"`)) {
		t.Errorf("Raw missing event type: %s", n.Raw)
	}
}

// TestParseSyncNoBotID confirms that without a resolved bot id, self-filtering
// is simply skipped (all message events pass through).
func TestParseSyncNoBotID(t *testing.T) {
	body := []byte(`{"next_batch":"s1","rooms":{"join":{"!r:host":{"timeline":{"events":[
		{"type":"m.room.message","sender":"@a:host","content":{"body":"one"}},
		{"type":"m.room.message","sender":"@b:host","content":{"body":"two"}}
	]}}}}}`)
	notes, _, err := parseSync(body, "")
	if err != nil {
		t.Fatalf("parseSync: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notifications, want 2", len(notes))
	}
}
