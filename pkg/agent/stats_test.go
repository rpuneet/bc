package agent

import (
	"context"
	"testing"
	"time"
)

func TestSaveAndQueryStats(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	rec := &AgentStatsRecord{
		AgentName:   "eng-01",
		CollectedAt: time.Now().UTC().Truncate(time.Second),
		CPUPct:      12.5,
		MemUsedMB:   256.0,
		MemLimitMB:  1024.0,
		NetRxMB:     0.1,
		NetTxMB:     0.05,
	}
	if saveErr := store.SaveStats(context.Background(), rec); saveErr != nil {
		t.Fatalf("SaveStats: %v", saveErr)
	}

	records, err := store.QueryStats(context.Background(), "eng-01", 10)
	if err != nil {
		t.Fatalf("QueryStats: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	got := records[0]
	if got.AgentName != rec.AgentName {
		t.Errorf("AgentName = %q, want %q", got.AgentName, rec.AgentName)
	}
	if got.CPUPct != rec.CPUPct {
		t.Errorf("CPUPct = %v, want %v", got.CPUPct, rec.CPUPct)
	}
	if got.MemUsedMB != rec.MemUsedMB {
		t.Errorf("MemUsedMB = %v, want %v", got.MemUsedMB, rec.MemUsedMB)
	}
}

func TestQueryStats_Empty(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	records, err := store.QueryStats(context.Background(), "nobody", 10)
	if err != nil {
		t.Fatalf("QueryStats: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestQueryStats_LimitRespected(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	for i := range 5 {
		rec := &AgentStatsRecord{
			AgentName:   "worker",
			CollectedAt: time.Now().Add(time.Duration(i) * time.Second),
			CPUPct:      float64(i),
		}
		if saveErr := store.SaveStats(context.Background(), rec); saveErr != nil {
			t.Fatalf("SaveStats #%d: %v", i, saveErr)
		}
	}
	records, err := store.QueryStats(context.Background(), "worker", 3)
	if err != nil {
		t.Fatalf("QueryStats: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("len(records) = %d, want 3", len(records))
	}
}
