package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/provider"
)

// The Live tab decides between "waiting for events" and "capture unavailable"
// from activity_mode. It used to decide from a hardcoded provider list in the
// frontend, which went stale and told cursor users their agents could never be
// captured while events were in fact being ingested. Serving the provider's own
// declaration is what keeps the two from drifting, so the field must be present
// and correct on the wire.

// activityModes fetches /api/providers and returns name → activity_mode.
func activityModes(t *testing.T, reg *provider.Registry) map[string]string {
	t.Helper()
	rec := httptest.NewRecorder()
	newProvidersMux(t, reg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []struct {
		Name         string `json:"name"`
		ActivityMode string `json:"activity_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	modes := make(map[string]string, len(body))
	for _, p := range body {
		modes[p.Name] = p.ActivityMode
	}
	return modes
}

func TestProvidersListReportsActivityMode(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())   // hooks
	reg.Register(provider.NewCursorProvider())   // hooks
	reg.Register(provider.NewCodexProvider())    // transcript
	reg.Register(provider.NewOpenclawProvider()) // none

	modes := activityModes(t, reg)

	want := map[string]string{
		"claude":   provider.ActivityModeHooks,
		"cursor":   provider.ActivityModeHooks,
		"codex":    provider.ActivityModeTranscript,
		"openclaw": provider.ActivityModeNone,
	}
	for name, wantMode := range want {
		if got := modes[name]; got != wantMode {
			t.Errorf("%s activity_mode = %q, want %q", name, got, wantMode)
		}
	}
}

// Every registered provider must serve a non-empty mode. An empty string would
// read as "unknown" in the UI and quietly reintroduce the guessing this replaced.
func TestProvidersListNeverServesAnEmptyActivityMode(t *testing.T) {
	for name, mode := range activityModes(t, provider.DefaultRegistry) {
		switch mode {
		case provider.ActivityModeHooks, provider.ActivityModeTranscript, provider.ActivityModeNone:
		default:
			t.Errorf("%s activity_mode = %q, want one of hooks/transcript/none", name, mode)
		}
	}
}
