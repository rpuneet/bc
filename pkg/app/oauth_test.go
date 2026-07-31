package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fakeOAuthPlugin proves OAuthFlow is assertable the way the server
// dispatches it (a type assertion on the registered plugin value).
type fakeOAuthPlugin struct{}

func (fakeOAuthPlugin) BeginAuth(_ context.Context, _ Instance) (AuthSession, error) {
	return AuthSession{
		ID:              "s1",
		Kind:            AuthKindDevice,
		VerificationURL: "https://example.com/device",
		UserCode:        "ABCD-1234",
		ExpiresAt:       time.Now().Add(15 * time.Minute),
		Interval:        5 * time.Second,
	}, nil
}

func (fakeOAuthPlugin) PollAuth(_ context.Context, s AuthSession) (AuthResult, error) {
	if s.ID != "s1" {
		return AuthResult{State: AuthStateError, Error: "unknown session"}, nil
	}
	return AuthResult{State: AuthStateComplete, Secrets: map[string]string{"api_token": "tok"}}, nil
}

func TestOAuthFlowCapabilityDispatch(t *testing.T) {
	var p any = fakeOAuthPlugin{}
	flow, ok := p.(OAuthFlow)
	if !ok {
		t.Fatal("fakeOAuthPlugin does not assert as OAuthFlow")
	}

	sess, err := flow.BeginAuth(context.Background(), Instance{Name: "oauthfake"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	if sess.Kind != AuthKindDevice || sess.UserCode == "" || sess.VerificationURL == "" {
		t.Errorf("session = %+v, want device kind with user code and URL", sess)
	}

	res, err := flow.PollAuth(context.Background(), AuthSession{ID: sess.ID})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != AuthStateComplete || res.Secrets["api_token"] != "tok" {
		t.Errorf("result = %+v, want complete with api_token", res)
	}
}

// TestAuthResultNeverSerializesSecrets locks the wire contract: only the
// state and error cross the API boundary; secrets go to the vault.
func TestAuthResultNeverSerializesSecrets(t *testing.T) {
	res := AuthResult{
		State:   AuthStateComplete,
		Secrets: map[string]string{"api_token": "super-sekret-value"},
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "super-sekret-value") || strings.Contains(string(data), "api_token") {
		t.Errorf("secrets leaked into JSON: %s", data)
	}
	if !strings.Contains(string(data), `"state":"complete"`) {
		t.Errorf("state missing from JSON: %s", data)
	}
}
