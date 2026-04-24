package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/bc/pkg/gateway"
)

func TestSecretValidation(t *testing.T) {
	secret := "my-webhook-secret" //nolint:gosec // test-only constant, not a real credential

	tests := []struct {
		name       string
		secret     string
		authHeader string
		secretHdr  string
		wantStatus int
	}{
		{
			name:       "valid bearer token",
			secret:     secret,
			authHeader: "Bearer my-webhook-secret",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid X-Webhook-Secret",
			secret:     secret,
			secretHdr:  secret,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid bearer token",
			secret:     secret,
			authHeader: "Bearer wrong-secret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing auth with secret required",
			secret:     secret,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no secret configured",
			secret:     "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewWithSecret("webhook", tt.secret)
			a.handler = func(_ gateway.Notification) {}

			body := `{"message":"hello"}`
			req := httptest.NewRequest(http.MethodPost, "/hooks/webhook", strings.NewReader(body))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.secretHdr != "" {
				req.Header.Set("X-Webhook-Secret", tt.secretHdr)
			}

			rr := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestSenderExtractionFallback(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantSender string
	}{
		{
			name:       "sender as string",
			body:       `{"sender":"deploy-bot"}`,
			wantSender: "deploy-bot",
		},
		{
			name:       "sender.login object",
			body:       `{"sender":{"login":"octocat"}}`,
			wantSender: "octocat",
		},
		{
			name:       "user field",
			body:       `{"user":"ci-bot"}`,
			wantSender: "ci-bot",
		},
		{
			name:       "author.name object",
			body:       `{"author":{"name":"Jane"}}`,
			wantSender: "Jane",
		},
		{
			name:       "from field",
			body:       `{"from":"alertmanager"}`,
			wantSender: "alertmanager",
		},
		{
			name:       "no sender fields",
			body:       `{"message":"hello"}`,
			wantSender: "webhook",
		},
		{
			name:       "invalid json",
			body:       `not json`,
			wantSender: "webhook",
		},
		{
			name:       "user.username object",
			body:       `{"user":{"username":"bot42"}}`,
			wantSender: "bot42",
		},
		{
			name:       "user.email object",
			body:       `{"user":{"email":"bot@example.com"}}`,
			wantSender: "bot@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSender([]byte(tt.body))
			if got != tt.wantSender {
				t.Errorf("extractSender() = %q, want %q", got, tt.wantSender)
			}
		})
	}
}

func TestRawJSONPassthrough(t *testing.T) {
	payload := map[string]any{
		"event":  "deploy",
		"status": "success",
		"env":    "production",
	}
	body, _ := json.Marshal(payload)

	var got gateway.Notification
	a := New()
	a.handler = func(n gateway.Notification) { got = n }

	req := httptest.NewRequest(http.MethodPost, "/hooks/webhook", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rr.Code)
	}

	var parsed map[string]any
	if err := json.Unmarshal(got.Raw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Raw: %v", err)
	}
	if parsed["event"] != "deploy" {
		t.Errorf("Raw event = %v, want %q", parsed["event"], "deploy")
	}
	if parsed["status"] != "success" {
		t.Errorf("Raw status = %v, want %q", parsed["status"], "success")
	}
	if got.Platform != "webhook" {
		t.Errorf("Platform = %q, want %q", got.Platform, "webhook")
	}
	if got.Channel != "webhook" {
		t.Errorf("Channel = %q, want %q", got.Channel, "webhook")
	}
}

func TestAdapterInterface(t *testing.T) {
	a := New()

	if a.Name() != "webhook" {
		t.Errorf("Name() = %q, want %q", a.Name(), "webhook")
	}
	if a.Type() != gateway.AdapterWebhook {
		t.Errorf("Type() = %q, want %q", a.Type(), gateway.AdapterWebhook)
	}

	if err := a.Start(context.TODO(), func(_ gateway.Notification) {}); err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}
	if err := a.Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}

	channels := a.Channels()
	if len(channels) != 1 {
		t.Fatalf("Channels() returned %d, want 1", len(channels))
	}
	if channels[0].Name != "webhook" {
		t.Errorf("channel name = %q, want %q", channels[0].Name, "webhook")
	}
}

func TestNamedAdapter(t *testing.T) {
	a := NewNamed("webhook:deploy")
	if a.Name() != "webhook:deploy" {
		t.Errorf("Name() = %q, want %q", a.Name(), "webhook:deploy")
	}

	channels := a.Channels()
	if len(channels) != 1 {
		t.Fatalf("Channels() returned %d, want 1", len(channels))
	}
	if channels[0].Name != "webhook:deploy" {
		t.Errorf("channel name = %q, want %q", channels[0].Name, "webhook:deploy")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	a := New()
	a.handler = func(_ gateway.Notification) {}

	req := httptest.NewRequest(http.MethodGet, "/hooks/webhook", nil)
	rr := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: got status %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestWebhookBodySizeLimit(t *testing.T) {
	tests := []struct {
		name     string
		bodySize int
		wantOK   bool
	}{
		{
			name:     "small payload accepted",
			bodySize: 100,
			wantOK:   true,
		},
		{
			name:     "exactly 1MB accepted",
			bodySize: 1 << 20, // 1MB
			wantOK:   true,
		},
		{
			name:     "over 1MB truncated silently",
			bodySize: (1 << 20) + 1000, // 1MB + 1000 bytes
			wantOK:   true,             // handler reads up to 1MB, rest is ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received []byte
			a := New()
			a.handler = func(n gateway.Notification) {
				received = n.Raw
			}

			// Build a JSON body of the desired size.
			// Use a repeated character payload embedded in JSON.
			payload := `{"data":"` + strings.Repeat("x", tt.bodySize-12) + `"}`
			req := httptest.NewRequest(http.MethodPost, "/hooks/webhook", strings.NewReader(payload))
			rr := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rr, req)

			if tt.wantOK && rr.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", rr.Code)
			}

			// For oversized payloads, verify the raw data is capped at 1MB
			if tt.bodySize > 1<<20 && len(received) > 1<<20 {
				t.Errorf("received %d bytes, expected at most %d (1MB limit)", len(received), 1<<20)
			}
		})
	}
}

func TestWebhookConstantTimeSecretComparison(t *testing.T) {
	// Verify that valid and invalid secrets produce correct results.
	// This tests that the auth validation works correctly for both paths,
	// which is the observable behavior of constant-time comparison.
	secret := "s3cr3t-t0k3n" //nolint:gosec // test-only constant

	tests := []struct {
		name       string
		authHeader string
		secretHdr  string
		wantStatus int
	}{
		{
			name:       "correct bearer token accepted",
			authHeader: "Bearer s3cr3t-t0k3n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "correct X-Webhook-Secret accepted",
			secretHdr:  "s3cr3t-t0k3n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong bearer token rejected",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong X-Webhook-Secret rejected",
			secretHdr:  "wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty secret header rejected",
			secretHdr:  "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "similar-length wrong secret rejected",
			authHeader: "Bearer s3cr3t-t0k3o", // off by one char
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer prefix without token rejected",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "basic auth scheme rejected",
			authHeader: "Basic czNjcjN0LXQwazNu",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewWithSecret("webhook", secret)
			a.handler = func(_ gateway.Notification) {}

			req := httptest.NewRequest(http.MethodPost, "/hooks/webhook", strings.NewReader(`{"test":true}`))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.secretHdr != "" {
				req.Header.Set("X-Webhook-Secret", tt.secretHdr)
			}

			rr := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestStatusUpdatesAfterWebhook(t *testing.T) {
	a := New()
	a.handler = func(_ gateway.Notification) {}

	req := httptest.NewRequest(http.MethodPost, "/hooks/webhook", strings.NewReader(`{"msg":"hi"}`))
	rr := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rr, req)

	status := a.Status()
	if !status.Connected {
		t.Error("Status().Connected should be true after first webhook")
	}
	if status.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", status.MessageCount)
	}
}
