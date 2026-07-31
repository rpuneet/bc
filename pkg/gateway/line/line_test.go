package line

import "testing"

// TestExtractEventContent verifies that a LINE message event yields the message
// text as content (previously only type+userId were extracted, dropping the
// body), while non-message events carry empty content.
func TestExtractEventContent(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantType    string
		wantSender  string
		wantContent string
	}{
		{
			name:        "message event with text",
			body:        `{"events":[{"type":"message","source":{"userId":"U123"},"message":{"type":"text","text":"hello line"}}]}`,
			wantType:    "message",
			wantSender:  "U123",
			wantContent: "hello line",
		},
		{
			name:        "follow event has no text",
			body:        `{"events":[{"type":"follow","source":{"userId":"U999"}}]}`,
			wantType:    "follow",
			wantSender:  "U999",
			wantContent: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et, sender, content := extractEvent([]byte(tt.body))
			if et != tt.wantType {
				t.Errorf("type = %q, want %q", et, tt.wantType)
			}
			if sender != tt.wantSender {
				t.Errorf("sender = %q, want %q", sender, tt.wantSender)
			}
			if content != tt.wantContent {
				t.Errorf("content = %q, want %q", content, tt.wantContent)
			}
		})
	}
}
