package whatsapp

import (
	"context"
	"strings"
	"testing"
)

func TestParseSendJID(t *testing.T) {
	tests := []struct {
		name      string
		channelID string
		wantUser  string
		wantSrv   string
		wantErr   string
	}{
		{
			name:      "phone jid",
			channelID: "918051005416@s.whatsapp.net",
			wantUser:  "918051005416",
			wantSrv:   "s.whatsapp.net",
		},
		{
			name:      "hidden lid jid",
			channelID: "30507219845203@lid",
			wantUser:  "30507219845203",
			wantSrv:   "lid",
		},
		{
			name:      "group jid",
			channelID: "120363123456789012@g.us",
			wantUser:  "120363123456789012",
			wantSrv:   "g.us",
		},
		{
			// A bare, all-digit id is treated as a phone number and defaulted
			// to the user server — this is what lets `whatsapp:<number>` reach
			// a 1:1 contact directly.
			name:      "bare phone number → user server",
			channelID: "918051005416",
			wantUser:  "918051005416",
			wantSrv:   "s.whatsapp.net",
		},
		{
			name:      "channel name rejected",
			channelID: "general",
			wantErr:   "no JID server",
		},
		{
			name:      "empty user rejected",
			channelID: "@lid",
			wantErr:   "empty user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jid, err := parseSendJID(tt.channelID)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSendJID(%q) error = %v, want containing %q", tt.channelID, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSendJID(%q) unexpected error: %v", tt.channelID, err)
			}
			if jid.User != tt.wantUser || jid.Server != tt.wantSrv {
				t.Fatalf("parseSendJID(%q) = %s@%s, want %s@%s", tt.channelID, jid.User, jid.Server, tt.wantUser, tt.wantSrv)
			}
		})
	}
}

func TestSendNotConnected(t *testing.T) {
	a := New(t.TempDir())
	err := a.Send(context.Background(), "918051005416@s.whatsapp.net", "zen-zebra", "hello")
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Send on disconnected adapter = %v, want not-connected error", err)
	}
}
