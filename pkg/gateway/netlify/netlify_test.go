package netlify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// makeJWS builds a compact JWS (JWT, HS256) over the given claims JSON, signed
// with secret — mirroring how Netlify signs its outgoing webhooks.
func makeJWS(secret, claimsJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
}

func bodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// TestHTTPHandlerJWS verifies real JWS signature checking: a token correctly
// signed with the secret (and matching body hash) is accepted; a token signed
// with the wrong secret, a tampered body, or a missing token is rejected.
func TestHTTPHandlerJWS(t *testing.T) {
	const secret = "netlify-secret"
	body := []byte(`{"committer":"alice","state":"ready"}`)
	validToken := makeJWS(secret, `{"iss":"netlify","sha256":"`+bodyHash(body)+`"}`)
	wrongSecretToken := makeJWS("other-secret", `{"iss":"netlify","sha256":"`+bodyHash(body)+`"}`)
	wrongHashToken := makeJWS(secret, `{"iss":"netlify","sha256":"`+bodyHash([]byte("different"))+`"}`)

	tests := []struct {
		name       string
		sig        string
		wantStatus int
	}{
		{"valid JWS", validToken, http.StatusOK},
		{"wrong secret", wrongSecretToken, http.StatusUnauthorized},
		{"body hash mismatch (tampered)", wrongHashToken, http.StatusUnauthorized},
		{"missing signature", "", http.StatusUnauthorized},
		{"malformed token", "not.a.jwt.here", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(secret)
			if err := a.Start(context.Background(), func(gateway.Notification) {}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("X-Netlify-Event", "deploy")
			if tt.sig != "" {
				req.Header.Set("X-Webhook-Signature", tt.sig)
			}
			rec := httptest.NewRecorder()
			a.HTTPHandler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestHTTPHandlerNoSecret confirms that when no secret is configured the
// webhook is accepted unauthenticated (honest optional-secret behavior).
func TestHTTPHandlerNoSecret(t *testing.T) {
	a := New("")
	if err := a.Start(context.Background(), func(gateway.Notification) {}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	a.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
