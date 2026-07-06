package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestExtractWAMentions_ExtendedText(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("hey check this out"),
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{
					"911234567890@s.whatsapp.net",
					"19876543210@s.whatsapp.net",
				},
			},
		},
	}

	got := extractWAMentions(msg)
	if len(got) != 2 {
		t.Fatalf("extractWAMentions got %d mentions, want 2: %v", len(got), got)
	}
	wantSet := map[string]bool{"911234567890": true, "19876543210": true}
	for _, m := range got {
		if !wantSet[m] {
			t.Errorf("unexpected mention %q", m)
		}
	}
}

func TestExtractWAMentions_ImageCaption(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption: proto.String("check this photo"),
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{"4912345@s.whatsapp.net"},
			},
		},
	}

	got := extractWAMentions(msg)
	if len(got) != 1 || got[0] != "4912345" {
		t.Fatalf("extractWAMentions = %v, want [4912345]", got)
	}
}

func TestExtractWAMentions_NoMentions(t *testing.T) {
	// Plain conversation message — no ContextInfo
	msg := &waE2E.Message{
		Conversation: proto.String("hello everyone"),
	}
	if got := extractWAMentions(msg); got != nil {
		t.Fatalf("extractWAMentions got %v, want nil", got)
	}
}

func TestExtractWAMentions_Nil(t *testing.T) {
	if got := extractWAMentions(nil); got != nil {
		t.Fatalf("extractWAMentions(nil) got %v, want nil", got)
	}
}

func TestExtractWAMentions_EmptyContext(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("hi"),
			ContextInfo: &waE2E.ContextInfo{},
		},
	}
	if got := extractWAMentions(msg); got != nil {
		t.Fatalf("extractWAMentions with empty ContextInfo got %v, want nil", got)
	}
}

func TestExtractWAMentions_NilContext(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("hi"),
			ContextInfo: nil,
		},
	}
	if got := extractWAMentions(msg); got != nil {
		t.Fatalf("extractWAMentions with nil ContextInfo got %v, want nil", got)
	}
}
