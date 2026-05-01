package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/stats"
)

// statsQuery holds parsed query parameters common to all stats endpoints.
type statsQuery struct {
	Filters   map[string][]string // param name → comma-split values
	TimeRange stats.TimeRange
}

// parseStatsQuery extracts from/to as time.Time (default: last 1 hour),
// interval as string (default: "5m"), and any comma-separated filter params.
func parseStatsQuery(r *http.Request, filterKeys ...string) statsQuery {
	now := time.Now()
	sq := statsQuery{
		TimeRange: stats.TimeRange{
			From:     now.Add(-1 * time.Hour),
			To:       now,
			Interval: "5m",
		},
		Filters: make(map[string][]string, len(filterKeys)),
	}

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sq.TimeRange.From = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sq.TimeRange.To = t
		}
	}
	if v := r.URL.Query().Get("interval"); v != "" {
		sq.TimeRange.Interval = v
	}

	for _, key := range filterKeys {
		if v := r.URL.Query().Get(key); v != "" {
			sq.Filters[key] = splitCSV(v)
		}
	}

	return sq
}

// splitCSV splits a comma-separated string into a trimmed slice.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
