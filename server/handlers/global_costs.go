package handlers

import (
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
)

// GlobalCostsHandler serves per-repo cost rollups from the user-global
// ledger.
//
// GET /api/global/costs?start=<RFC3339|YYYY-MM-DD>&groupBy=workspace|project
//
//   - `start` defaults to 30 days ago when omitted.
//   - `end` is not honored because the underlying store only accepts a
//     lower bound (`since`); callers that need a window should narrow on
//     the client. TODO(#250): widen pkg/cost to accept a full range.
type GlobalCostsHandler struct {
	store *cost.Store
}

// NewGlobalCostsHandler builds a handler. If store is nil the endpoint
// returns 503, keeping production test harnesses that don't wire a
// global ledger from panicking.
func NewGlobalCostsHandler(store *cost.Store) *GlobalCostsHandler {
	return &GlobalCostsHandler{store: store}
}

// CostRow is one row of the /api/global/costs response.
type CostRow struct {
	Key   string  `json:"key"`   // workspace id or project label
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
	if h.store == nil {
		httpError(w, "global cost ledger not configured", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	groupBy := q.Get("groupBy")
	if groupBy == "" {
		groupBy = "workspace"
	}
	if groupBy != "workspace" && groupBy != "project" {
		httpError(w, "groupBy must be 'workspace' or 'project'", http.StatusBadRequest)
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

// rollup returns rows keyed either by workspace id or project name.
func (h *GlobalCostsHandler) rollup(r *http.Request, groupBy string, since time.Time) ([]CostRow, error) {
	sinceArg := sinceFormatter{t: since}

	if groupBy == "workspace" {
		byRepo, err := h.store.SumByRepo(r.Context(), sinceArg)
		if err != nil {
			return nil, err
		}
		out := make([]CostRow, 0, len(byRepo))
		for repo, total := range byRepo {
			key := repo
			label := h.resolveLabel(repo)
			if repo == "" {
				key = "unattributed"
				label = "Unattributed"
			}
			out = append(out, CostRow{Key: key, Label: label, Total: total})
		}
		return out, nil
	}

	// groupBy=project: resolver collapses by repo name.
	resolve := func(repo string) string { return h.resolveLabel(repo) }
	byProj, err := h.store.SumByProject(r.Context(), sinceArg, resolve)
	if err != nil {
		return nil, err
	}
	out := make([]CostRow, 0, len(byProj))
	for key, total := range byProj {
		out = append(out, CostRow{Key: key, Label: key, Total: total})
	}
	return out, nil
}

// resolveLabel maps a repo path to a human-readable label: the repo
// directory basename, or the path itself when it has no useful base.
func (h *GlobalCostsHandler) resolveLabel(repo string) string {
	if repo == "" {
		return repo
	}
	if base := filepath.Base(repo); base != "." && base != string(filepath.Separator) {
		return base
	}
	return repo
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

// sinceFormatter adapts time.Time to the interface{Format(string) string}
// parameter required by (*cost.Store).SumByRepo / SumByProject.
type sinceFormatter struct{ t time.Time }

func (s sinceFormatter) Format(layout string) string { return s.t.Format(layout) }
