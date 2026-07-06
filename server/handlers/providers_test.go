package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
