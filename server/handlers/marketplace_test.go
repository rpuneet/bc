package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/marketplace"
	"github.com/rpuneet/mycel/pkg/template"
)

// fakeFetcherHandler is a minimal Fetcher that always returns 404 so tests
// exercise the mycel-template source without any real HTTP calls.
type fakeFetcherHandler struct{}

func (f *fakeFetcherHandler) Do(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusNotFound)
	return rec.Result(), nil
}

func newTestAggregator(t *testing.T, tmplNames []string) *marketplace.Aggregator {
	t.Helper()
	dir := t.TempDir()
	store := template.NewStore(dir)
	for _, n := range tmplNames {
		tmpl := template.Template{Name: n, Description: "desc " + n}
		if err := store.Create(tmpl, "", template.ScopeGlobal); err != nil {
			t.Fatalf("create template %q: %v", n, err)
		}
	}
	return marketplace.NewAggregator(store, &fakeFetcherHandler{})
}

func TestMarketplaceHandler_List(t *testing.T) {
	agg := newTestAggregator(t, []string{"reviewer", "feature-dev"})
	h := NewMarketplaceHandler(agg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []marketplace.Item
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 mycel items, got %d", len(items))
	}
}

func TestMarketplaceHandler_FilterByType(t *testing.T) {
	agg := newTestAggregator(t, []string{"reviewer"})
	h := NewMarketplaceHandler(agg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace?type=template", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var items []marketplace.Item
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, it := range items {
		if it.Type != marketplace.TypeTemplate {
			t.Errorf("unexpected item type %q (want template)", it.Type)
		}
	}
}

func TestMarketplaceHandler_MethodNotAllowed(t *testing.T) {
	agg := newTestAggregator(t, nil)
	h := NewMarketplaceHandler(agg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/marketplace", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestMarketplaceHandler_EmptyResultIsArray(t *testing.T) {
	// type=mcp with no MCP sources → should return [] not null.
	agg := newTestAggregator(t, []string{"reviewer"})
	h := NewMarketplaceHandler(agg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace?type=mcp", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || body[0] != '[' {
		t.Errorf("want JSON array, got %q", body)
	}
}
