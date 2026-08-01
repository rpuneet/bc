package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getOnboardingState(t *testing.T, h *OnboardingHandler) onboardingState {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding/state", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var st onboardingState
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return st
}

func TestOnboardingState_FreshInstall(t *testing.T) {
	h := newTestHome(t) // valid prefs, no onboarding progress
	oh := NewOnboardingHandler(h, func() int { return 0 })

	st := getOnboardingState(t, oh)
	if !st.PrefsValid {
		t.Error("prefsValid = false, want true for a valid config")
	}
	if st.HasAgents {
		t.Error("hasAgents = true, want false with zero agents")
	}
	if !st.FirstRun {
		t.Error("firstRun = false, want true (no agents, wizard not completed)")
	}
}

func TestOnboardingState_HasAgents(t *testing.T) {
	h := newTestHome(t)
	oh := NewOnboardingHandler(h, func() int { return 3 })

	st := getOnboardingState(t, oh)
	if !st.HasAgents {
		t.Error("hasAgents = false, want true")
	}
	if st.FirstRun {
		t.Error("firstRun = true, want false once agents exist")
	}
}

func TestOnboardingState_CompletedSuppressesFirstRun(t *testing.T) {
	h := newTestHome(t)
	h.Config.Onboarding.Completed = []string{"welcome", "done"}
	h.Config.Onboarding.Step = "done"
	oh := NewOnboardingHandler(h, func() int { return 0 })

	st := getOnboardingState(t, oh)
	if st.FirstRun {
		t.Error("firstRun = true, want false after the wizard was completed")
	}
	if st.Step != "done" {
		t.Errorf("step = %q, want done", st.Step)
	}
	if len(st.Completed) != 2 {
		t.Errorf("completed = %v, want 2 entries", st.Completed)
	}
}

func TestOnboardingState_InvalidPrefsIsFirstRun(t *testing.T) {
	h := newTestHome(t)
	h.Config.Version = 0 // invalidates the config (Validate requires v2)
	oh := NewOnboardingHandler(h, func() int { return 5 })

	st := getOnboardingState(t, oh)
	if st.PrefsValid {
		t.Error("prefsValid = true, want false for an invalid config")
	}
	if !st.FirstRun {
		t.Error("firstRun = false, want true when prefs are invalid even with agents")
	}
}

func TestOnboardingState_MethodGuard(t *testing.T) {
	h := newTestHome(t)
	oh := NewOnboardingHandler(h, func() int { return 0 })
	mux := http.NewServeMux()
	oh.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/state", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
