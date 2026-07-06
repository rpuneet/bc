package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/server/handlers"
)

func newProvidersMux(t *testing.T, reg *provider.Registry) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	handlers.NewProviderHandler(reg, nil, nil, nil).Register(mux)
	return mux
}

func TestProvidersList(t *testing.T) {
	reg := provider.NewRegistry()
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("want empty list, got %d entries", len(body))
	}
}

func TestProvidersListModelShape(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var providers []struct {
		Name   string `json:"name"`
		Models []struct {
			ID        string `json:"id"`
			Available bool   `json:"available"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &providers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("want 1 provider, got %d", len(providers))
	}
	// claude has a static model list; all entries should have Available=false (static fallback).
	for _, m := range providers[0].Models {
		if m.ID == "" {
			t.Error("model ID must not be empty")
		}
		if m.Available {
			t.Error("static model should have available=false")
		}
	}
}

func TestProvidersModelsSubroute(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/claude/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var models []struct {
		ID        string `json:"id"`
		Available bool   `json:"available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected at least one model for claude")
	}
	for _, m := range models {
		if m.ID == "" {
			t.Error("model ID must not be empty")
		}
	}
}

func TestProvidersModelsUnknownProvider(t *testing.T) {
	reg := provider.NewRegistry()
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/nope/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestProvidersListParallelCorrectness verifies that the parallel list handler
// returns all registered providers sorted by name without dropping or duplicating
// entries. This is the regression gate for the parallelisation introduced to fix
// the cold-cache serial hang (bugs #1).
func TestProvidersListParallelCorrectness(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())
	reg.Register(provider.NewAgyProvider())
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var provs []struct {
		Name   string `json:"name"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &provs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(provs) != 2 {
		t.Fatalf("want 2 providers, got %d: %v", len(provs), provs)
	}
	// Response must be alphabetically sorted regardless of dispatch order.
	if provs[0].Name > provs[1].Name {
		t.Errorf("providers not sorted: %q > %q", provs[0].Name, provs[1].Name)
	}
	// Each provider must include its model list (even if empty).
	for _, p := range provs {
		if p.Models == nil {
			t.Errorf("provider %q: models must not be null", p.Name)
		}
	}
}

// fakeSlowProvider is a minimal Provider + DynamicModelLister that records how
// many times ListModels is invoked and returns after a short delay. It is used
// to verify singleflight deduplication (bug #3).
type fakeSlowProvider struct {
	calls atomic.Int32
	delay time.Duration
}

func (f *fakeSlowProvider) Name() string                               { return "fake-slow" }
func (f *fakeSlowProvider) Description() string                        { return "test" }
func (f *fakeSlowProvider) Command() string                            { return "fake" }
func (f *fakeSlowProvider) Binary() string                             { return "fake" }
func (f *fakeSlowProvider) InstallHint() string                        { return "" }
func (f *fakeSlowProvider) BuildCommand(_ provider.CommandOpts) string { return "fake" }
func (f *fakeSlowProvider) IsInstalled(_ context.Context) bool         { return false }
func (f *fakeSlowProvider) Version(_ context.Context) string           { return "" }
func (f *fakeSlowProvider) ListModels(_ context.Context) ([]string, error) {
	f.calls.Add(1)
	time.Sleep(f.delay)
	return []string{"model-a"}, nil
}

// TestProvidersModelsFetchSingleflight verifies that concurrent requests for the
// same provider's model list result in only one CLI invocation (singleflight, bug #3).
func TestProvidersModelsFetchSingleflight(t *testing.T) {
	fake := &fakeSlowProvider{delay: 30 * time.Millisecond}

	reg := provider.NewRegistry()
	reg.Register(fake)
	mux := newProvidersMux(t, reg)

	const concurrency = 5
	done := make(chan int, concurrency)
	for range concurrency {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/providers/fake-slow/models", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			done <- rec.Code
		}()
	}
	for range concurrency {
		if code := <-done; code != http.StatusOK {
			t.Errorf("concurrent request: status = %d, want 200", code)
		}
	}

	// With singleflight the in-flight requests share one shell-out; allow up to 2
	// (one real + one possible cache miss race) but never 5.
	if got := fake.calls.Load(); got > 2 {
		t.Errorf("ListModels called %d times for %d concurrent requests; want ≤2 (singleflight)", got, concurrency)
	}
}
