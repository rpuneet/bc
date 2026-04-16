package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/rpuneet/bc/pkg/cost"
	"github.com/rpuneet/bc/pkg/workspace"
)

// GlobalCostStore is the minimal interface used by GlobalCostHandler.
// A real *cost.GlobalStore satisfies this; tests provide an in-memory fake.
type GlobalCostStore interface {
	SumByWorkspace(ctx context.Context, start, end time.Time) ([]*cost.GlobalSummary, error)
	SumByProject(ctx context.Context, start, end time.Time) ([]*cost.GlobalSummary, error)
}

// WorkspaceRegistryLister is the minimal interface used to resolve workspace
// names from their paths. *workspace.Registry satisfies it; tests can stub it.
type WorkspaceRegistryLister interface {
	List() []workspace.RegistryEntry
}

// GlobalCostHandler serves cross-workspace cost reports at /api/global/costs.
type GlobalCostHandler struct {
	store    GlobalCostStore
	registry WorkspaceRegistryLister
}

// NewGlobalCostHandler constructs a handler. registry may be nil.
func NewGlobalCostHandler(store GlobalCostStore, registry WorkspaceRegistryLister) *GlobalCostHandler {
	return &GlobalCostHandler{store: store, registry: registry}
}

// Register mounts the handler on mux at /api/global/costs.
//
// The route intentionally lives OUTSIDE any per-workspace scope middleware —
// it reports across workspaces and must not require a workspace context.
func (h *GlobalCostHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/global/costs", h.serve)
}

// Row is one table row in the response.
type Row struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Total      float64 `json:"total"`
	AgentCount int64   `json:"agentCount"`
}

// Response is the JSON payload for GET /api/global/costs.
type Response struct {
	Range   RangeBlock `json:"range"`
	GroupBy string     `json:"groupBy"`
	Rows    []Row      `json:"rows"`
}

// RangeBlock reports the resolved time range (RFC3339 UTC).
type RangeBlock struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// parseDate accepts either RFC3339 or YYYY-MM-DD. Empty string returns zero.
func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errInvalidDate
}

var errInvalidDate = &dateFormatError{}

type dateFormatError struct{}

func (e *dateFormatError) Error() string {
	return "invalid date: expected RFC3339 or YYYY-MM-DD"
}

func (h *GlobalCostHandler) serve(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.store == nil {
		httpError(w, "global cost store not configured", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	groupBy := strings.ToLower(strings.TrimSpace(q.Get("groupBy")))
	if groupBy == "" {
		groupBy = "workspace"
	}
	if groupBy != "workspace" && groupBy != "project" {
		httpError(w, "groupBy must be 'workspace' or 'project'", http.StatusBadRequest)
		return
	}

	start, err := parseDate(q.Get("start"))
	if err != nil {
		httpError(w, "invalid 'start' date: "+err.Error(), http.StatusBadRequest)
		return
	}
	end, err := parseDate(q.Get("end"))
	if err != nil {
		httpError(w, "invalid 'end' date: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Default to last 30 days when unspecified.
	if start.IsZero() && end.IsZero() {
		end = time.Now().UTC()
		start = end.AddDate(0, 0, -30)
	}

	var (
		summaries []*cost.GlobalSummary
		sumErr    error
	)
	if groupBy == "workspace" {
		summaries, sumErr = h.store.SumByWorkspace(r.Context(), start, end)
	} else {
		summaries, sumErr = h.store.SumByProject(r.Context(), start, end)
	}
	if sumErr != nil {
		httpInternalError(w, "sum cost records", sumErr)
		return
	}

	labels := h.buildLabels(groupBy)

	rows := make([]Row, 0, len(summaries))
	for _, s := range summaries {
		if s == nil {
			continue
		}
		label := labels[s.Key]
		if label == "" {
			// Fallback: for workspace groupBy use the key; for project
			// groupBy the key IS the project path, which is already a
			// reasonable label.
			label = s.Key
		}
		rows = append(rows, Row{
			Key:        s.Key,
			Label:      label,
			Total:      s.TotalUSD,
			AgentCount: s.AgentCount,
		})
	}

	writeJSON(w, http.StatusOK, Response{
		Range: RangeBlock{
			Start: formatRange(start),
			End:   formatRange(end),
		},
		GroupBy: groupBy,
		Rows:    rows,
	})
}

func formatRange(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// buildLabels produces key→label maps for the given groupBy. For workspaces
// the key is the workspace path/name (whichever the global store uses); we
// resolve to the registry's Name/Alias. For projects the key is the path and
// no lookup is needed.
func (h *GlobalCostHandler) buildLabels(groupBy string) map[string]string {
	labels := map[string]string{}
	if h.registry == nil {
		return labels
	}
	entries := h.registry.List()
	for _, e := range entries {
		// Index by name, alias, and path so we can resolve regardless of
		// which identifier callers mirrored into the global store.
		if e.Name != "" {
			labels[e.Name] = displayName(e)
		}
		if e.Alias != "" {
			labels[e.Alias] = displayName(e)
		}
		if e.Path != "" {
			if groupBy == "workspace" {
				labels[e.Path] = displayName(e)
			}
		}
	}
	return labels
}

func displayName(e workspace.RegistryEntry) string {
	if e.Name != "" {
		return e.Name
	}
	if e.Alias != "" {
		return e.Alias
	}
	return e.Path
}
