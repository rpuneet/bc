package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/template"
)

// ── fake HTTP client ──────────────────────────────────────────────────────────

// fakeFetcher routes requests to registered handlers by URL prefix.
type fakeFetcher struct {
	handlers map[string]http.Handler
}

func (f *fakeFetcher) Do(req *http.Request) (*http.Response, error) {
	for prefix, h := range f.handlers {
		if strings.HasPrefix(req.URL.String(), prefix) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec.Result(), nil
		}
	}
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusNotFound)
	return rec.Result(), nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonHandler(v interface{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	})
}

func makeTemplateStore(t *testing.T, names []string) *template.Store {
	t.Helper()
	dir := t.TempDir()
	s := template.NewStore(dir)
	for _, n := range names {
		tmpl := template.Template{Name: n, Description: "desc " + n}
		if err := s.Create(tmpl, "", template.ScopeGlobal); err != nil {
			t.Fatalf("seed template %q: %v", n, err)
		}
	}
	return s
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestAggregator_MCPRegistry(t *testing.T) {
	page := mcpRegistryPage{}
	page.Servers = []mcpRegistryEntry{
		{Server: struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Repository  *struct {
				URL string `json:"url"`
			} `json:"repository,omitempty"`
			Packages []struct {
				RegistryName string `json:"registry_name"`
				Name         string `json:"name"`
			} `json:"packages,omitempty"`
		}{Name: "fetch", Description: "HTTP fetch tool"}},
		{Server: struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Repository  *struct {
				URL string `json:"url"`
			} `json:"repository,omitempty"`
			Packages []struct {
				RegistryName string `json:"registry_name"`
				Name         string `json:"name"`
			} `json:"packages,omitempty"`
		}{Name: "brave-search", Description: "Brave search"}},
	}

	client := &fakeFetcher{handlers: map[string]http.Handler{
		mcpRegistryURL: jsonHandler(page),
	}}

	agg := NewAggregator(nil, client)
	items, err := agg.fetchMCPRegistry(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "fetch" {
		t.Errorf("want first item name 'fetch', got %q", items[0].Name)
	}
	if items[0].Source != SourceMCPRegistry {
		t.Errorf("want source %q, got %q", SourceMCPRegistry, items[0].Source)
	}
	if items[0].Type != TypeMCP {
		t.Errorf("want type %q, got %q", TypeMCP, items[0].Type)
	}
}

func TestAggregator_GitHub_StarsThreshold(t *testing.T) {
	resp := githubSearchResponse{
		Items: []githubRepo{
			{FullName: "owner/high-stars", Name: "high-stars", StarCount: 5000, HTMLURL: "https://github.com/owner/high-stars"},
			{FullName: "owner/low-stars", Name: "low-stars", StarCount: 10, HTMLURL: "https://github.com/owner/low-stars"},
		},
	}

	client := &fakeFetcher{handlers: map[string]http.Handler{
		githubSearchURL: jsonHandler(resp),
	}}

	agg := NewAggregator(nil, client)
	items, err := agg.fetchGitHub(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// low-stars should be filtered; high-stars kept.
	// Two queries (mcp-server + claude-skill) → high-stars appears twice (deduped).
	for _, it := range items {
		if it.Stars < GitHubStarsThreshold {
			t.Errorf("item %q has stars %d below threshold %d", it.Name, it.Stars, GitHubStarsThreshold)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected at least one item above threshold")
	}
}

func TestAggregator_Mycel(t *testing.T) {
	store := makeTemplateStore(t, []string{"eng", "reviewer"})
	agg := NewAggregator(store, nil)
	items, err := agg.fetchMycel(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	for _, it := range items {
		if it.Source != SourceMycel {
			t.Errorf("want source %q, got %q", SourceMycel, it.Source)
		}
		if it.Type != TypeTemplate {
			t.Errorf("want type %q, got %q", TypeTemplate, it.Type)
		}
	}
}

func TestAggregator_Cache(t *testing.T) {
	callCount := 0
	srv := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		_ = json.NewEncoder(w).Encode(mcpRegistryPage{})
	})
	client := &fakeFetcher{handlers: map[string]http.Handler{
		mcpRegistryURL: srv,
		githubSearchURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(githubSearchResponse{})
		}),
	}}

	agg := NewAggregator(nil, client)
	ctx := context.Background()

	if _, err := agg.List(ctx, "", "", ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	firstCount := callCount
	if firstCount == 0 {
		t.Fatal("expected HTTP calls on first List()")
	}

	// Second call should hit cache.
	if _, err := agg.List(ctx, "", "", ""); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if callCount != firstCount {
		t.Errorf("expected no new HTTP calls (cache hit), but callCount went %d → %d", firstCount, callCount)
	}
}

func TestFilter_TypeAndQuery(t *testing.T) {
	items := []Item{
		{ID: "1", Name: "brave-search", Type: TypeMCP, Source: SourceMCPRegistry},
		{ID: "2", Name: "reviewer", Type: TypeTemplate, Source: SourceMycel},
		{ID: "3", Name: "fetch-tool", Type: TypeMCP, Source: SourceGitHub},
	}

	got := filter(items, "mcp", "", "")
	if len(got) != 2 {
		t.Errorf("type=mcp want 2, got %d", len(got))
	}

	got = filter(items, "", "", "brave")
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("query=brave want [1], got %v", got)
	}

	got = filter(items, "mcp", "", "fetch")
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("type=mcp query=fetch want [3], got %v", got)
	}
}

func TestAggregator_NilTemplateStore(t *testing.T) {
	// fetchMycel with nil store returns no items and no error.
	agg := NewAggregator(nil, nil)
	items, err := agg.fetchMycel(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items with nil store, got %d", len(items))
	}
}

func TestAggregator_HTTPError(t *testing.T) {
	// Registry returns 500 — fetchMCPRegistry should return an error.
	client := &fakeFetcher{handlers: map[string]http.Handler{
		mcpRegistryURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchMCPRegistry(context.Background())
	if err == nil {
		t.Fatal("expected error from 500 response, got nil")
	}
}

// TestAggregator_StaleOnRefreshFailure verifies that List returns stale data
// when a refresh fails but a previous successful result is cached.
func TestAggregator_StaleOnRefreshFailure(t *testing.T) {
	// Build a store with one template and prime the cache with it.
	store := makeTemplateStore(t, []string{"cached-item"})

	// First client always succeeds (MCP registry + GitHub return empty, mycel works).
	good := &fakeFetcher{handlers: map[string]http.Handler{
		mcpRegistryURL:  jsonHandler(mcpRegistryPage{}),
		githubSearchURL: jsonHandler(githubSearchResponse{}),
	}}
	agg := NewAggregator(store, good)
	ctx := context.Background()

	items, err := agg.List(ctx, "", "", "")
	if err != nil {
		t.Fatalf("prime: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("prime: expected at least one item from mycel store")
	}

	// Swap in a broken client; cache should be served.
	agg.httpClient = &fakeFetcher{} // no handlers → 404 for every URL
	// Force cache expiry so aggregate() is called again.
	agg.cachedAt = agg.cachedAt.Add(-2 * cacheTTL)
	// hasCache was set by the first successful List(); leave it true.

	items2, err2 := agg.List(ctx, "", "", "")
	if err2 != nil {
		t.Fatalf("stale: expected no error, got %v", err2)
	}
	if len(items2) == 0 {
		t.Fatal("stale: expected stale items to be returned")
	}
}

// Ensure the package compiles correctly by exercising the directory path.
func TestItemIDFormat(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(filepath.Join(dir, "templates"))
	if err := os.MkdirAll(store.GlobalDir(), 0750); err != nil {
		t.Fatal(err)
	}
	agg := NewAggregator(store, &fakeFetcher{})
	items, err := agg.fetchMycel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("empty store should yield 0 items, got %d", len(items))
	}
}

func TestDedupeByID(t *testing.T) {
	in := []Item{
		{ID: "mcp-registry:ai.bowmark/bowmark", Name: "bowmark"},
		{ID: "mcp-registry:ai.bowmark/bowmark", Name: "bowmark"}, // dup version
		{ID: "mcp-registry:ai.bowmark/bowmark", Name: "bowmark"}, // dup version
		{ID: "github:foo/bar", Name: "bar"},
		{ID: "", Name: "no-id-a"}, // empty IDs are kept as-is
		{ID: "", Name: "no-id-b"},
	}
	out := dedupeByID(in)
	if len(out) != 4 {
		t.Fatalf("expected 4 items after dedup, got %d", len(out))
	}
	var bowmark int
	for _, it := range out {
		if it.ID == "mcp-registry:ai.bowmark/bowmark" {
			bowmark++
		}
	}
	if bowmark != 1 {
		t.Errorf("bowmark should collapse to 1 entry, got %d", bowmark)
	}
}
