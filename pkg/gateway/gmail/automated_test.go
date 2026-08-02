package gmailgw

import (
	"strings"
	"testing"

	gmail "google.golang.org/api/gmail/v1"
)

// TestClassifyAutomated covers the senders actually seen in a live mailbox.
// The false cases matter as much as the true ones: a human writing from a
// role address must still reach agents.
func TestClassifyAutomated(t *testing.T) {
	tests := []struct {
		headers map[string]string
		name    string
		from    string
		want    bool
	}{
		{
			name:    "github notification",
			from:    `"coderabbitai[bot]" <notifications@github.com>`,
			headers: map[string]string{"Auto-Submitted": "auto-generated", "Precedence": "list"},
			want:    true,
		},
		{
			name:    "github notification without auto-submitted still caught by sender",
			from:    "notifications@github.com",
			headers: map[string]string{},
			want:    true,
		},
		{
			name:    "newsletter",
			from:    "Market Wrap <wrap@news.example.com>",
			headers: map[string]string{"Precedence": "bulk", "List-Unsubscribe": "<https://news.example.com/u/1>"},
			want:    true,
		},
		{
			name:    "bank alert from no-reply",
			from:    "Example Bank <no-reply@examplebank.com>",
			headers: map[string]string{},
			want:    true,
		},
		{
			name:    "vendor-prefixed noreply",
			from:    "github-noreply@example.com",
			headers: map[string]string{},
			want:    true,
		},
		{
			name:    "bounce",
			from:    "MAILER-DAEMON@mx.example.com",
			headers: map[string]string{},
			want:    true,
		},
		{
			name:    "out of office autoreply",
			from:    "Colleague <colleague@example.com>",
			headers: map[string]string{"Auto-Submitted": "auto-replied"},
			want:    true,
		},
		{
			name:    "corporate suppression hint",
			from:    "hr-system@example.com",
			headers: map[string]string{"X-Auto-Response-Suppress": "All"},
			want:    true,
		},
		{
			name:    "header lookup is case-insensitive",
			from:    "someone@example.com",
			headers: map[string]string{"list-unsubscribe": "<mailto:u@example.com>"},
			want:    true,
		},
		{
			name:    "plain human mail",
			from:    "Puneet <puneet@example.com>",
			headers: map[string]string{},
			want:    false,
		},
		{
			name:    "human mail explicitly marked not auto-submitted",
			from:    "Puneet <puneet@example.com>",
			headers: map[string]string{"Auto-Submitted": "no"},
			want:    false,
		},
		{
			name:    "human-staffed role address is not filtered",
			from:    "Support <support@vendor.com>",
			headers: map[string]string{},
			want:    false,
		},
		{
			name:    "team discussion list still reaches agents",
			from:    "Colleague <colleague@example.com>",
			headers: map[string]string{"Precedence": "list", "List-Id": "team <team.example.com>"},
			want:    false,
		},
		{
			name:    "no headers and no address",
			from:    "",
			headers: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := classifyAutomated(tt.from, tt.headers)
			if got != tt.want {
				t.Errorf("classifyAutomated(%q, %v) = %v (%q), want %v", tt.from, tt.headers, got, reason, tt.want)
			}
			// A positive verdict must explain itself — the reason is what an
			// operator reads when mail unexpectedly skips agent delivery.
			if got && strings.TrimSpace(reason) == "" {
				t.Error("automated verdict returned an empty reason")
			}
			if !got && reason != "" {
				t.Errorf("non-automated verdict returned reason %q", reason)
			}
		})
	}
}

// TestParseMessageClassifiesAutomated proves the classification is wired into
// the poll path, not just available as a helper.
func TestParseMessageClassifiesAutomated(t *testing.T) {
	m := &gmail.Message{
		Id: "m1",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "From", Value: `"coderabbitai[bot]" <notifications@github.com>`},
				{Name: "Subject", Value: "Re: approved this pull request"},
				{Name: "Auto-Submitted", Value: "auto-generated"},
			},
		},
	}
	e := parseMessage(m)
	if !e.Automated {
		t.Error("GitHub notification mail should be marked automated")
	}
	if e.AutomatedReason == "" {
		t.Error("automated mail should carry a reason for the logs")
	}
}

func TestParseMessageHumanMailNotAutomated(t *testing.T) {
	m := &gmail.Message{
		Id: "m2",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "From", Value: "Puneet <puneet@example.com>"},
				{Name: "Subject", Value: "ready for release"},
			},
		},
	}
	e := parseMessage(m)
	if e.Automated {
		t.Errorf("human mail marked automated: %q", e.AutomatedReason)
	}
}

// TestAutomatedHeadersAreFetched guards the coupling between the classifier
// and the metadata headers the poll requests: a signal the classifier reads
// but the API call never asks for is dead code.
func TestAutomatedHeadersAreFetched(t *testing.T) {
	for _, h := range []string{"Auto-Submitted", "Precedence", "List-Unsubscribe", "X-Auto-Response-Suppress"} {
		found := false
		for _, got := range automatedHeaders {
			if strings.EqualFold(got, h) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("classifier reads %q but it is not in automatedHeaders", h)
		}
	}
}
