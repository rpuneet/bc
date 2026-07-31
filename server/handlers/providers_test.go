package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/home"
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

// newProvidersMuxWithWS builds a provider mux with a home attached so
// handlers that pass the repo root through can be verified.
func newProvidersMuxWithWS(t *testing.T, reg *provider.Registry, h *home.Home) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	handlers.NewProviderHandler(reg, nil, nil, h).Register(mux)
	return mux
}

func TestProvidersCommandsCurated(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/claude/commands", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var cmds []struct {
		Name        string `json:"name"`
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cmds); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Claude implements CommandLister; the curated list must come through.
	if len(cmds) != 7 {
		t.Fatalf("want 7 curated commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0].Name != "mcp add" {
		t.Errorf("first command = %q, want %q", cmds[0].Name, "mcp add")
	}
}

func TestProvidersCommandsGenericFallback(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&fakeSlowProvider{})
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/fake-slow/commands", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var cmds []struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cmds); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No CommandLister → generic run/version/help default keyed by name.
	if len(cmds) != 3 {
		t.Fatalf("want 3 generic commands, got %d: %v", len(cmds), cmds)
	}
	want := map[string]string{"run": "fake-slow", "version": "fake-slow --version", "help": "fake-slow --help"}
	for _, c := range cmds {
		if want[c.Name] != c.Command {
			t.Errorf("command %q = %q, want %q", c.Name, c.Command, want[c.Name])
		}
	}
}

// stubMCPProvider implements Provider + MCPConfigReader and records the
// rootDir the handler passes through.
type stubMCPProvider struct {
	gotRootDir atomic.Value
}

func (s *stubMCPProvider) Name() string                               { return "stub-mcp" }
func (s *stubMCPProvider) Description() string                        { return "test" }
func (s *stubMCPProvider) Command() string                            { return "stub" }
func (s *stubMCPProvider) Binary() string                             { return "stub" }
func (s *stubMCPProvider) InstallHint() string                        { return "" }
func (s *stubMCPProvider) BuildCommand(_ provider.CommandOpts) string { return "stub" }
func (s *stubMCPProvider) IsInstalled(_ context.Context) bool         { return false }
func (s *stubMCPProvider) Version(_ context.Context) string           { return "" }

func (s *stubMCPProvider) ReadMCPs(_ context.Context, rootDir string) []provider.MCPServerInfo {
	s.gotRootDir.Store(rootDir)
	return []provider.MCPServerInfo{
		{Name: "the daemon", Transport: "sse", URL: "http://localhost:9374/mcp/sse", Enabled: true},
	}
}

func TestProvidersMCPsCapability(t *testing.T) {
	stub := &stubMCPProvider{}
	reg := provider.NewRegistry()
	reg.Register(stub)
	mux := newProvidersMuxWithWS(t, reg, &home.Home{RootDir: "/h/root"})

	req := httptest.NewRequest(http.MethodGet, "/api/providers/stub-mcp/mcps", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var servers []struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		URL       string `json:"url"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &servers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("want 1 server, got %d: %v", len(servers), servers)
	}
	s := servers[0]
	if s.Name != "the daemon" || s.Transport != "sse" || s.URL != "http://localhost:9374/mcp/sse" || !s.Enabled {
		t.Errorf("server = %+v", s)
	}
	if got := stub.gotRootDir.Load(); got != "/h/root" {
		t.Errorf("rootDir passed to ReadMCPs = %v, want /h/root", got)
	}
}

func TestProvidersMCPsNoCapability(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&fakeSlowProvider{})
	mux := newProvidersMux(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/fake-slow/mcps", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// No MCPConfigReader → empty JSON array, never null.
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("body = %q, want empty array", body)
	}
}
