package cost

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func mkGlobalStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenGlobalStore(filepath.Join(dir, "costs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenGlobalStoreCreatesSchema(t *testing.T) {
	s := mkGlobalStore(t)
	if s.db == nil {
		t.Fatal("store db nil")
	}

	// The repo column must exist.
	rows, err := s.db.QueryContext(context.Background(), `PRAGMA table_info(cost_records)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); scanErr != nil {
			t.Fatal(scanErr)
		}
		if name == "repo" {
			found = true
		}
	}
	if !found {
		t.Error("repo column not created")
	}
}

func TestScopedRecordTagsRepo(t *testing.T) {
	s := mkGlobalStore(t)
	ctx := context.Background()
	scoped := s.ScopedTo("/repos/alpha")

	if _, err := scoped.Record(ctx, "agent1", "", "claude-3", 100, 50, 0.01); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Record(ctx, "agent1", "", "claude-3", 100, 50, 0.02); err != nil {
		t.Fatal(err)
	}

	scopedB := s.ScopedTo("/repos/beta")
	if _, err := scopedB.Record(ctx, "agent2", "", "claude-3", 200, 100, 0.05); err != nil {
		t.Fatal(err)
	}

	byRepo, err := s.SumByRepo(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := byRepo["/repos/alpha"]; got < 0.0299 || got > 0.0301 {
		t.Errorf("/repos/alpha total = %f, want ~0.03", got)
	}
	if got := byRepo["/repos/beta"]; got < 0.0499 || got > 0.0501 {
		t.Errorf("/repos/beta total = %f, want ~0.05", got)
	}
}

func TestSumByRepoIncludesUnattributed(t *testing.T) {
	s := mkGlobalStore(t)
	ctx := context.Background()

	scoped := s.ScopedTo("")
	if _, err := scoped.Record(ctx, "a", "", "m", 1, 1, 0.1); err != nil {
		t.Fatal(err)
	}
	scopedA := s.ScopedTo("/repos/a")
	if _, err := scopedA.Record(ctx, "a", "", "m", 1, 1, 0.2); err != nil {
		t.Fatal(err)
	}

	byRepo, err := s.SumByRepo(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := byRepo[""]; got < 0.099 || got > 0.101 {
		t.Errorf("unattributed total = %f", got)
	}
	if got := byRepo["/repos/a"]; got < 0.199 || got > 0.201 {
		t.Errorf("/repos/a total = %f", got)
	}
}

func TestSumByProjectUsesResolver(t *testing.T) {
	s := mkGlobalStore(t)
	ctx := context.Background()

	sA := s.ScopedTo("/repos/trade-prod")
	sB := s.ScopedTo("/repos/trade-paper")
	if _, err := sA.Record(ctx, "x", "", "m", 1, 1, 1.0); err != nil {
		t.Fatal(err)
	}
	if _, err := sB.Record(ctx, "x", "", "m", 1, 1, 2.0); err != nil {
		t.Fatal(err)
	}

	names := map[string]string{"/repos/trade-prod": "trade-prod", "/repos/trade-paper": "trade-paper"}
	resolve := func(repo string) string { return names[repo] }

	got, err := s.SumByProject(ctx, time.Time{}, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if got["trade-prod"] != 1.0 {
		t.Errorf("trade-prod = %f", got["trade-prod"])
	}
	if got["trade-paper"] != 2.0 {
		t.Errorf("trade-paper = %f", got["trade-paper"])
	}
}

func TestSumByProjectFallbackUnattributed(t *testing.T) {
	s := mkGlobalStore(t)
	ctx := context.Background()

	sc := s.ScopedTo("")
	if _, err := sc.Record(ctx, "x", "", "m", 1, 1, 3.0); err != nil {
		t.Fatal(err)
	}

	got, err := s.SumByProject(ctx, time.Time{}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if got["unattributed"] != 3.0 {
		t.Errorf("got %+v", got)
	}
}
