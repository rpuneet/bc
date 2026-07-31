package datadog

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// TestHTTPHandlerSharedSecret verifies the honest Datadog auth: a shared
// secret carried in the URL query or the JSON payload is accepted, anything
// else is rejected, and an unconfigured secret means unauthenticated.
func TestHTTPHandlerSharedSecret(t *testing.T) {
	const secret = "topsecret"

	tests := []struct {
		name       string
		secret     string
		target     string
		body       string
		wantStatus int
	}{
		{"secret in query", secret, "/?secret=topsecret", `{"event_type":"alert"}`, http.StatusOK},
		{"secret in payload", secret, "/", `{"event_type":"alert","secret":"topsecret"}`, http.StatusOK},
		{"wrong query secret", secret, "/?secret=nope", `{"event_type":"alert"}`, http.StatusUnauthorized},
		{"no secret supplied", secret, "/", `{"event_type":"alert"}`, http.StatusUnauthorized},
		{"unauthenticated when unconfigured", "", "/", `{"event_type":"alert"}`, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewNamed("datadog", tt.secret)
			if err := a.Start(context.Background(), func(gateway.Notification) {}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, tt.target, bytes.NewReader([]byte(tt.body)))
			rec := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
