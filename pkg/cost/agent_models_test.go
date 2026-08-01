package cost

import (
	"context"
	"testing"
	"time"
)

// TestAgentModelSummaryFiltersByAgent locks the /api/costs/models?agent=
// path: only the requested cost-agent's entries aggregate, grouped by model.
func TestAgentModelSummaryFiltersByAgent(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)

	sums, err := svc.GetAgentModelSummarySince(context.Background(), "a1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 {
		t.Fatalf("got %d model rows for a1, want 2", len(sums))
	}
	if sums[0].Model != "m2" || sums[0].TotalCostUSD != 2.0 {
		t.Errorf("top row = %s $%.2f, want m2 $2.00", sums[0].Model, sums[0].TotalCostUSD)
	}
	if sums[1].Model != "m1" || sums[1].TotalCostUSD != 1.0 {
		t.Errorf("second row = %s $%.2f, want m1 $1.00", sums[1].Model, sums[1].TotalCostUSD)
	}

	all, err := svc.GetAgentModelSummarySince(context.Background(), "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if s.Model == "m1" && s.TotalCostUSD != 5.5 {
			t.Errorf("unfiltered m1 cost = %.2f, want 5.5", s.TotalCostUSD)
		}
	}
}
