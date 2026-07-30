package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
)

// GlobalCostsHandler serves per-repo cost rollups computed directly
// from provider session files.
//
// GET /api/global/costs?start=<RFC3339|YYYY-MM-DD>&groupBy=repo|project
//
//   - `start` defaults to 30 days ago when omitted.
//   - Rows are keyed by the session working directory ("repo"); sources
//     without a recorded working dir roll up under "unattributed".
type GlobalCostsHandler struct {
	svc *cost.Service
}

// NewGlobalCostsHandler builds a handler. If svc is nil the endpoint
// returns 503, keeping test harnesses that don't wire costs from
// panicking.
func NewGlobalCostsHandler(svc *cost.Service) *GlobalCostsHandler {
	return &GlobalCostsHandler{svc: svc}
}

// CostRow is one row of the /api/global/costs response.
type CostRow struct {
	Key   string  `json:"key"`   // repo path or project label
	Label string  `json:"label"` // human-readable name
	Total float64 `json:"total"` // USD
}

// CostReport is the envelope returned by /api/global/costs.
type CostReport struct {
	Range struct {
		Start string `json:"start"`
	} `json:"range"`
	GroupBy string    `json:"groupBy"`
	Rows    []CostRow `json:"rows"`
}

// ServeHTTP implements http.Handler.
func (h *GlobalCostsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		httpError(w, "cost service not configured", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	groupBy := q.Get("groupBy")
	if groupBy == "" {
		groupBy = "repo"
	}
	if groupBy != "repo" && groupBy != "project" {
		httpError(w, "groupBy must be 'repo' or 'project'", http.StatusBadRequest)
		return
	}

	startStr := q.Get("start")
	since, err := parseCostStart(startStr)
	if err != nil {
		httpError(w, "invalid start: "+err.Error(), http.StatusBadRequest)
		return
	}

	rows, err := h.rollup(r, groupBy, since)
	if err != nil {
		httpInternalError(w, "roll up costs", err)
		return
	}

	// Stable sort: highest total first, ties broken by label.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return rows[i].Label < rows[j].Label
	})

	report := CostReport{GroupBy: groupBy, Rows: rows}
	report.Range.Start = since.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, report)
}

// rollup returns rows keyed either by repo path or project name.
func (h *GlobalCostsHandler) rollup(r *http.Request, groupBy string, since time.Time) ([]CostRow, error) {
	if groupBy == "repo" {
		byRepo, err := h.svc.SumByRepo(r.Context(), since)
		if err != nil {
			return nil, err
		}
		out := make([]CostRow, 0, len(byRepo))
		for repo, total := range byRepo {
			key := repo
			label := cost.RepoLabel(repo)
			if repo == "" {
				key = "unattributed"
				label = "Unattributed"
			}
			out = append(out, CostRow{Key: key, Label: label, Total: total})
		}
		return out, nil
	}

	// groupBy=project: resolver collapses by repo name.
	byProj, err := h.svc.SumByProject(r.Context(), since, cost.RepoLabel)
	if err != nil {
		return nil, err
	}
	out := make([]CostRow, 0, len(byProj))
	for key, total := range byProj {
		out = append(out, CostRow{Key: key, Label: key, Total: total})
	}
	return out, nil
}

// parseCostStart parses start=... into a time. Accepts RFC3339 or
// YYYY-MM-DD. Empty input defaults to 30 days ago.
func parseCostStart(s string) (time.Time, error) {
	if s == "" {
		return time.Now().Add(-30 * 24 * time.Hour), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errBadTime
}

var errBadTime = &httpErr{Msg: "expected RFC3339 or YYYY-MM-DD"}

type httpErr struct{ Msg string }

func (e *httpErr) Error() string { return e.Msg }
