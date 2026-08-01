package gmailgw

import (
	"encoding/base64"
	"strings"
	"testing"

	gmail "google.golang.org/api/gmail/v1"
)

func TestExtractEmail(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Ada Lovelace <ada@example.com>", "ada@example.com"},
		{"ada@example.com", "ada@example.com"},
		{"  <bob@example.com>  ", "bob@example.com"},
		{`"Doe, John" <john@x.io>`, "john@x.io"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractEmail(tt.in); got != tt.want {
			t.Errorf("extractEmail(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRecipientAddress(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"gmail:alice@example.com", "alice@example.com"},
		{"alice@example.com", "alice@example.com"},
		{"gmail:Alice <alice@example.com>", "alice@example.com"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := recipientAddress(tt.in); got != tt.want {
			t.Errorf("recipientAddress(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseMessage(t *testing.T) {
	m := &gmail.Message{
		Id:       "m1",
		ThreadId: "t1",
		Snippet:  "hello there",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "From", Value: "Ada <ada@example.com>"},
				{Name: "Subject", Value: "Weekly report"},
				{Name: "Date", Value: "Mon, 1 Jan 2026 10:00:00 +0000"},
			},
		},
	}
	e := parseMessage(m)
	if e.ID != "m1" || e.ThreadID != "t1" {
		t.Errorf("ids = %q/%q, want m1/t1", e.ID, e.ThreadID)
	}
	if e.From != "Ada <ada@example.com>" {
		t.Errorf("From = %q", e.From)
	}
	if e.Subject != "Weekly report" {
		t.Errorf("Subject = %q", e.Subject)
	}
	if e.Snippet != "hello there" {
		t.Errorf("Snippet = %q", e.Snippet)
	}
	if extractEmail(e.From) != "ada@example.com" {
		t.Errorf("extractEmail(From) = %q", extractEmail(e.From))
	}
}

func TestParseMessageEncodedSubject(t *testing.T) {
	m := &gmail.Message{
		Id: "m2",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "=?UTF-8?B?SGVsbG8gd29ybGQ=?="}, // "Hello world"
			},
		},
	}
	if got := parseMessage(m).Subject; got != "Hello world" {
		t.Errorf("decoded subject = %q, want %q", got, "Hello world")
	}
}

func TestBuildMessage(t *testing.T) {
	raw := buildMessage("me@example.com", "you@example.com", "Subject line\nBody paragraph one.\nLine two.")
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	msg := string(decoded)
	for _, want := range []string{
		"From: me@example.com\r\n",
		"To: you@example.com\r\n",
		"Subject: Subject line\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	headerEnd := strings.Index(msg, "\r\n\r\n")
	if headerEnd < 0 {
		t.Fatalf("no header/body separator in %q", msg)
	}
	body := msg[headerEnd+4:]
	if body != "Body paragraph one.\nLine two." {
		t.Errorf("body = %q", body)
	}
}

func TestBuildMessageSingleLine(t *testing.T) {
	raw := buildMessage("", "you@example.com", "just one line")
	decoded, _ := base64.URLEncoding.DecodeString(raw) //nolint:errcheck
	msg := string(decoded)
	if strings.Contains(msg, "From:") {
		t.Error("empty from should omit From header")
	}
	if !strings.Contains(msg, "Subject: just one line\r\n") {
		t.Errorf("single-line subject missing in %q", msg)
	}
	body := msg[strings.Index(msg, "\r\n\r\n")+4:]
	if body != "just one line" {
		t.Errorf("body = %q, want single-line content", body)
	}
}

func TestBuildMessageEmptySubject(t *testing.T) {
	raw := buildMessage("", "you@example.com", "\nbody only")
	decoded, _ := base64.URLEncoding.DecodeString(raw) //nolint:errcheck
	if !strings.Contains(string(decoded), "Subject: (no subject)\r\n") {
		t.Errorf("empty subject fallback missing in %q", string(decoded))
	}
}
