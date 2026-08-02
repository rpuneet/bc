package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

func computeHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHMACValidation(t *testing.T) {
	secret := "test-secret-123"
	body := []byte(`{"action":"opened","sender":{"login":"octocat"}}`)

	tests := []struct {
		name       string
		secret     string
		signature  string
		wantStatus int
	}{
		{
			name:       "valid signature",
			secret:     secret,
			signature:  computeHMAC(secret, body),
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid signature",
			secret:     secret,
			signature:  "sha256=deadbeef",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing signature with secret",
			secret:     secret,
			signature:  "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no secret configured",
			secret:     "",
			signature:  "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.secret)
			a.handler = func(_ gateway.Notification) {} // no-op handler

			handler := a.HTTPHandler()
			req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(string(body)))
			req.Header.Set("X-GitHub-Event", "issues")
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestEventExtraction(t *testing.T) {
	body := []byte(`{"action":"opened","sender":{"login":"octocat"}}`)

	var got gateway.Notification
	a := New("")
	a.handler = func(n gateway.Notification) { got = n }

	handler := a.HTTPHandler()

	tests := []struct {
		name      string
		event     string
		wantEvent string
	}{
		{"push event", "push", "push"},
		{"pull_request event", "pull_request", "pull_request"},
		{"missing event header", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(string(body)))
			if tt.event != "" {
				req.Header.Set("X-GitHub-Event", tt.event)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", rr.Code)
			}
			if got.Channel != tt.wantEvent {
				t.Errorf("got channel %q, want %q", got.Channel, tt.wantEvent)
			}
		})
	}
}

func TestSenderExtraction(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantSender string
	}{
		{
			name:       "sender.login present",
			body:       `{"sender":{"login":"octocat"},"action":"opened"}`,
			wantSender: "octocat",
		},
		{
			name:       "no sender field",
			body:       `{"action":"opened"}`,
			wantSender: "github",
		},
		{
			name:       "empty sender login",
			body:       `{"sender":{"login":""},"action":"opened"}`,
			wantSender: "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSender([]byte(tt.body))
			if got != tt.wantSender {
				t.Errorf("got sender %q, want %q", got, tt.wantSender)
			}
		})
	}
}

func TestAdapterInterface(t *testing.T) {
	a := New("secret")

	if a.Name() != "github" {
		t.Errorf("Name() = %q, want %q", a.Name(), "github")
	}
	if a.Type() != gateway.AdapterWebhook {
		t.Errorf("Type() = %q, want %q", a.Type(), gateway.AdapterWebhook)
	}

	// Start should be a no-op and succeed.
	if err := a.Start(context.TODO(), func(_ gateway.Notification) {}); err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}

	// Stop should be a no-op and succeed.
	if err := a.Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}

	channels := a.Channels()
	if len(channels) != len(commonEventTypes) {
		t.Errorf("Channels() returned %d, want %d", len(channels), len(commonEventTypes))
	}

	status := a.Status()
	if status.Connected {
		t.Error("Status().Connected = true before any webhook received")
	}
}

func TestNamedAdapter(t *testing.T) {
	a := NewNamed("github:mycel", "secret", "")
	if a.Name() != "github:mycel" {
		t.Errorf("Name() = %q, want %q", a.Name(), "github:mycel")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	a := New("")
	handler := a.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/hooks/github", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET request: got status %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestStatusUpdatesAfterWebhook(t *testing.T) {
	a := New("")
	a.handler = func(_ gateway.Notification) {}

	body := `{"sender":{"login":"octocat"}}`
	req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rr, req)

	status := a.Status()
	if !status.Connected {
		t.Error("Status().Connected should be true after first webhook")
	}
	if status.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", status.MessageCount)
	}
	if status.LastMessageAt.IsZero() {
		t.Error("LastMessageAt should be set after webhook")
	}
}

func TestRawPayloadPassthrough(t *testing.T) {
	payload := map[string]any{
		"action": "opened",
		"sender": map[string]any{"login": "octocat"},
		"number": 42,
	}
	body, _ := json.Marshal(payload)

	var got gateway.Notification
	a := New("")
	a.handler = func(n gateway.Notification) { got = n }

	req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "pull_request")

	rr := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rr, req)

	respBody, _ := io.ReadAll(rr.Result().Body)
	if string(respBody) != "ok" {
		t.Errorf("response body = %q, want %q", string(respBody), "ok")
	}

	// Verify raw JSON is passed through.
	var parsed map[string]any
	if err := json.Unmarshal(got.Raw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Raw: %v", err)
	}
	if parsed["action"] != "opened" {
		t.Errorf("Raw action = %v, want %q", parsed["action"], "opened")
	}
}
