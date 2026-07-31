package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckDaemonAt_Degraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"degraded","db":"ok","degraded":{` +
			`"notify":"notify store unavailable: no shared database",` +
			`"events":"event log unavailable: disk full"}}`))
	}))
	defer srv.Close()

	cat := CheckDaemonAt(context.Background(), srv.URL)
	if cat.Name != "Daemon" {
		t.Fatalf("category name = %q, want Daemon", cat.Name)
	}
	if len(cat.Items) != 2 {
		t.Fatalf("items = %d, want 2 (one per degraded service): %+v", len(cat.Items), cat.Items)
	}
	// Sorted by service name: events before notify.
	if cat.Items[0].Name != "service: events" || cat.Items[1].Name != "service: notify" {
		t.Errorf("unexpected item names: %q, %q", cat.Items[0].Name, cat.Items[1].Name)
	}
	for _, it := range cat.Items {
		if it.Severity != SeverityWarn {
			t.Errorf("item %q severity = %v, want warn", it.Name, it.Severity)
		}
		if it.Message == "" {
			t.Errorf("item %q has empty message — reason must be surfaced", it.Name)
		}
	}
}

func TestCheckDaemonAt_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","db":"ok","version":"0.3.0","commit":"abc"}`))
	}))
	defer srv.Close()

	cat := CheckDaemonAt(context.Background(), srv.URL)
	if len(cat.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(cat.Items), cat.Items)
	}
	if cat.Items[0].Severity != SeverityOK {
		t.Errorf("severity = %v, want ok", cat.Items[0].Severity)
	}
}

func TestCheckDaemonAt_DaemonDown(t *testing.T) {
	// Grab a port that is guaranteed closed by opening then closing a server.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()

	cat := CheckDaemonAt(context.Background(), addr)
	if len(cat.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(cat.Items), cat.Items)
	}
	if cat.Items[0].Severity != SeverityOK {
		t.Errorf("daemon down must be reported OK (skip gracefully), got %v: %s",
			cat.Items[0].Severity, cat.Items[0].Message)
	}
}
