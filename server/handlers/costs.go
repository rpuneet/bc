package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
)

// CostHandler handles /api/costs routes.
type CostHandler struct {
	store    *cost.Store
	importer *cost.Importer
}

// NewCostHandler creates a CostHandler.
func NewCostHandler(store *cost.Store, importer *cost.Importer) *CostHandler {
	return &CostHandler{store: store, importer: importer}
}

// Register mounts cost routes on mux.
func (h *CostHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/costs", h.summary)
	mux.HandleFunc("/api/costs/", h.byResource)
}

func (h *CostHandler) summary(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	store := h.store
	s, err := store.WorkspaceSummary(r.Context())
	if err != nil {
		httpInternalError(w, "workspace summary", err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *CostHandler) byResource(w http.ResponseWriter, r *http.Request) {
	store := h.store
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/costs/"), "/", 3)
	resource := parts[0]

	switch resource {
	case "agents":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		summaries, err := store.SummaryByAgent(r.Context())
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(summaries) {
			summaries = []*cost.Summary{}
		} else {
			summaries = summaries[offset:]
			if len(summaries) > limit {
				summaries = summaries[:limit]
			}
		}
		writeJSON(w, http.StatusOK, summaries)

	case "teams":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		summaries, err := store.SummaryByTeam(r.Context())
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(summaries) {
			summaries = []*cost.Summary{}
		} else {
			summaries = summaries[offset:]
			if len(summaries) > limit {
				summaries = summaries[:limit]
			}
		}
		writeJSON(w, http.StatusOK, summaries)

	case "models":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		summaries, err := store.SummaryByModel(r.Context())
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(summaries) {
			summaries = []*cost.Summary{}
		} else {
			summaries = summaries[offset:]
			if len(summaries) > limit {
				summaries = summaries[:limit]
			}
		}
		writeJSON(w, http.StatusOK, summaries)

	case "daily":
		h.daily(w, r)

	case "sync":
		h.sync(w, r)

	case "budgets":
		h.budgets(w, r, parts)

	case "project":
		h.project(w, r)

	case "agent":
		h.agentDetail(w, r, parts)

	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

// daily handles GET /api/costs/daily?days=30
func (h *CostHandler) daily(w http.ResponseWriter, r *http.Request) {
	store := h.store
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	days := 30
	if s := r.URL.Query().Get("days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			days = n
		}
	}
	days = clampInt(days, 1, 365)
	since := time.Now().AddDate(0, 0, -days)
	costs, err := store.GetDailyCosts(r.Context(), since)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	if costs == nil {
		costs = []*cost.DailyCost{}
	}
	writeJSON(w, http.StatusOK, costs)
}

// sync handles POST /api/costs/sync — triggers a fresh import from JSONL files.
func (h *CostHandler) sync(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	importer := h.importer
	if importer == nil {
		httpError(w, "importer not configured", http.StatusServiceUnavailable)
		return
	}
	n, err := importer.ImportAll(r.Context())
	if err != nil {
		httpInternalError(w, "import failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": n})
}

// budgets handles /api/costs/budgets and /api/costs/budgets/{scope}.
func (h *CostHandler) budgets(w http.ResponseWriter, r *http.Request, parts []string) {
	store := h.store
	// Determine scope from path: /api/costs/budgets or /api/costs/budgets/{scope}
	scope := ""
	if len(parts) >= 2 && parts[1] != "" {
		scope = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		if scope == "" {
			// GET /api/costs/budgets — list all budgets
			budgets, err := store.GetAllBudgets(r.Context())
			if err != nil {
				httpInternalError(w, "operation failed", err)
				return
			}
			if budgets == nil {
				budgets = []*cost.Budget{}
			}
			writeJSON(w, http.StatusOK, budgets)
		} else {
			// GET /api/costs/budgets/{scope} — get budget + check status
			status, err := store.CheckBudget(r.Context(), scope)
			if err != nil {
				httpInternalError(w, "operation failed", err)
				return
			}
			if status == nil {
				httpError(w, "no budget configured for "+scope, http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, status)
		}

	case http.MethodPost:
		// POST /api/costs/budgets — set budget
		var req struct {
			Scope    string  `json:"scope"`
			Period   string  `json:"period"`
			LimitUSD float64 `json:"limit_usd"`
			AlertAt  float64 `json:"alert_at"`
			HardStop bool    `json:"hard_stop"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Scope == "" {
			httpError(w, "scope is required", http.StatusBadRequest)
			return
		}
		if req.LimitUSD <= 0 {
			httpError(w, "limit_usd must be positive", http.StatusBadRequest)
			return
		}
		period := cost.BudgetPeriod(req.Period)
		switch period {
		case cost.BudgetPeriodDaily, cost.BudgetPeriodWeekly, cost.BudgetPeriodMonthly:
			// valid
		default:
			httpError(w, "invalid period: must be daily, weekly, or monthly", http.StatusBadRequest)
			return
		}
		budget, err := store.SetBudget(r.Context(), req.Scope, period, req.LimitUSD, req.AlertAt, req.HardStop)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusOK, budget)

	case http.MethodDelete:
		// DELETE /api/costs/budgets/{scope}
		if scope == "" {
			httpError(w, "scope is required in path", http.StatusBadRequest)
			return
		}
		if err := store.DeleteBudget(r.Context(), scope); err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

// project handles GET /api/costs/project?lookback_days=30&project_days=30
func (h *CostHandler) project(w http.ResponseWriter, r *http.Request) {
	store := h.store
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	lookbackDays := 30
	if s := r.URL.Query().Get("lookback_days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			lookbackDays = n
		}
	}
	projectDays := 30
	if s := r.URL.Query().Get("project_days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			projectDays = n
		}
	}
	proj, err := store.ProjectCost(r.Context(), lookbackDays, time.Duration(projectDays)*24*time.Hour)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

// agentDetail handles GET /api/costs/agent/{name}
func (h *CostHandler) agentDetail(w http.ResponseWriter, r *http.Request, parts []string) {
	store := h.store
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if len(parts) < 2 || parts[1] == "" {
		httpError(w, "agent name required", http.StatusBadRequest)
		return
	}
	agentName := parts[1]

	summary, err := store.AgentSummary(r.Context(), agentName)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}

	// Get daily breakdown for the last 30 days
	since := time.Now().AddDate(0, 0, -30)
	allAgentDaily, err := store.GetAgentDailyCosts(r.Context(), since)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}

	// Filter to the requested agent
	var daily []*cost.AgentDailyCost
	for _, d := range allAgentDaily {
		if d.AgentID == agentName {
			daily = append(daily, d)
		}
	}

	response := struct {
		Summary *cost.Summary          `json:"summary"`
		Daily   []*cost.AgentDailyCost `json:"daily"`
	}{
		Summary: summary,
		Daily:   daily,
	}
	writeJSON(w, http.StatusOK, response)
}
