package bitbucket

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// TestHTTPHandlerSignature verifies that a correctly-signed sha256=-prefixed
// request (as real Bitbucket sends) is accepted, while a wrong signature is
// rejected with 401.
func TestHTTPHandlerSignature(t *testing.T) {
	const secret = "shh"
	body := []byte(`{"actor":{"display_name":"alice"}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name       string
		sig        string
		wantStatus int
	}{
		{"valid sha256-prefixed signature", validSig, http.StatusOK},
		{"valid bare hex (no prefix)", hex.EncodeToString(mac.Sum(nil)), http.StatusOK},
		{"wrong signature", "sha256=deadbeef", http.StatusUnauthorized},
		{"missing signature", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(secret)
			if err := a.Start(context.Background(), func(gateway.Notification) {}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("X-Event-Key", "push")
			if tt.sig != "" {
				req.Header.Set("X-Hub-Signature", tt.sig)
			}
			rec := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
