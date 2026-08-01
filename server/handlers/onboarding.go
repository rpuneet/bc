package handlers

import (
	"net/http"

	"github.com/rpuneet/mycel/pkg/home"
)

// OnboardingHandler serves first-run setup state for the web wizard.
//
// The wizard is config-only: it reads whether this is a fresh install and
// where the user left off, and never inspects or mutates agents, secrets,
// or the database here.
type OnboardingHandler struct {
	home       *home.Home
	agentCount func() int
}

// NewOnboardingHandler builds an OnboardingHandler. agentCount reports the
// number of agents known to the daemon; pass a nil-safe closure over the
// agent manager. It may be nil, in which case the count is treated as zero.
func NewOnboardingHandler(h *home.Home, agentCount func() int) *OnboardingHandler {
	return &OnboardingHandler{home: h, agentCount: agentCount}
}

// Register mounts the onboarding routes on mux.
func (h *OnboardingHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/onboarding/state", h.state)
}

// onboardingState is the JSON shape returned by GET /api/onboarding/state.
type onboardingState struct {
	Step       string   `json:"step"`
	Completed  []string `json:"completed"`
	FirstRun   bool     `json:"firstRun"`
	HasAgents  bool     `json:"hasAgents"`
	PrefsValid bool     `json:"prefsValid"`
}

// state handles GET /api/onboarding/state.
//
// firstRun is true when the home is not yet usable as a set-up install:
// preferences are missing/invalid, or there are no agents and the wizard
// has never been completed. The web app routes to /welcome on firstRun.
func (h *OnboardingHandler) state(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	count := 0
	if h.agentCount != nil {
		count = h.agentCount()
	}
	hasAgents := count > 0

	prefsValid := false
	var step string
	var completed []string
	completedWizard := false
	if h.home != nil && h.home.Config != nil {
		prefsValid = h.home.Config.Validate() == nil
		step = h.home.Config.Onboarding.Step
		completed = h.home.Config.Onboarding.Completed
		completedWizard = h.home.Config.Onboarding.OnboardingComplete()
	}
	if completed == nil {
		completed = []string{}
	}

	firstRun := !prefsValid || (!hasAgents && !completedWizard)

	writeJSON(w, http.StatusOK, onboardingState{
		Step:       step,
		Completed:  completed,
		FirstRun:   firstRun,
		HasAgents:  hasAgents,
		PrefsValid: prefsValid,
	})
}
