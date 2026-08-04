package marketplace

import (
	"context"
	"net/http"
	"testing"
)

// ── Claude / Anthropic skills ─────────────────────────────────────────────────

func TestFetchClaude_ParsesPlugins(t *testing.T) {
	page := claudeMarketplace{
		Plugins: []claudePlugin{
			{Name: "pdf", Description: "PDF processing skill", Homepage: "https://github.com/anthropics/skills/tree/main/skills/pdf"},
			{Name: "xlsx", Description: "Excel skill"},
		},
	}
	client := &fakeFetcher{handlers: map[string]http.Handler{
		claudePluginsURL: jsonHandler(page),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchClaude(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "pdf" {
		t.Errorf("want name 'pdf', got %q", items[0].Name)
	}
	if items[0].Source != SourceClaude {
		t.Errorf("want source %q, got %q", SourceClaude, items[0].Source)
	}
	if items[0].Type != TypeSkill {
		t.Errorf("want type %q, got %q", TypeSkill, items[0].Type)
	}
	if items[0].URL != "https://github.com/anthropics/skills/tree/main/skills/pdf" {
		t.Errorf("want homepage as URL, got %q", items[0].URL)
	}
	// Item with no homepage falls back to source.url (which is empty → empty URL).
	if items[1].URL != "" {
		t.Errorf("want empty URL for item with no homepage/source, got %q", items[1].URL)
	}
}

func TestFetchClaude_HTTPError(t *testing.T) {
	client := &fakeFetcher{handlers: map[string]http.Handler{
		claudePluginsURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchClaude(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

// ── Google Gemini extensions ──────────────────────────────────────────────────

func TestFetchGemini_ParsesRepos(t *testing.T) {
	repos := []githubRepo{
		{FullName: "gemini-cli-extensions/conductor", Name: "conductor", Description: "Plan features", HTMLURL: "https://github.com/gemini-cli-extensions/conductor", StarCount: 3600},
		{FullName: "gemini-cli-extensions/nanobanana", Name: "nanobanana", HTMLURL: "https://github.com/gemini-cli-extensions/nanobanana", StarCount: 1100},
	}
	client := &fakeFetcher{handlers: map[string]http.Handler{
		geminiExtOrgURL: jsonHandler(repos),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchGemini(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Source != SourceGemini {
		t.Errorf("want source %q, got %q", SourceGemini, items[0].Source)
	}
	if items[0].Type != TypeSkill {
		t.Errorf("want type %q, got %q", TypeSkill, items[0].Type)
	}
	if items[0].Stars != 3600 {
		t.Errorf("want stars=3600, got %d", items[0].Stars)
	}
}

func TestFetchGemini_HTTPError(t *testing.T) {
	client := &fakeFetcher{handlers: map[string]http.Handler{
		geminiExtOrgURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchGemini(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// ── Glama ─────────────────────────────────────────────────────────────────────

func TestFetchGlama_ParsesServers(t *testing.T) {
	page := glamaPage{}
	page.PageInfo.HasNextPage = false
	page.Servers = []glamaServer{
		{Name: "fetch", Namespace: "acme", Slug: "fetch", Description: "HTTP fetch tool", URL: "https://glama.ai/mcp/servers/abc123"},
		{Name: "brave-search", Namespace: "brave", Slug: "brave-search", Description: "Brave search MCP"},
	}
	client := &fakeFetcher{handlers: map[string]http.Handler{
		glamaURL: jsonHandler(page),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchGlama(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "fetch" {
		t.Errorf("want name 'fetch', got %q", items[0].Name)
	}
	if items[0].Source != SourceGlama {
		t.Errorf("want source %q, got %q", SourceGlama, items[0].Source)
	}
	if items[0].Type != TypeMCP {
		t.Errorf("want type %q, got %q", TypeMCP, items[0].Type)
	}
	if items[0].ID != "glama:acme/fetch" {
		t.Errorf("want id 'glama:acme/fetch', got %q", items[0].ID)
	}
	if items[0].URL != "https://glama.ai/mcp/servers/abc123" {
		t.Errorf("want url from registry, got %q", items[0].URL)
	}
}

func TestFetchGlama_HTTPError(t *testing.T) {
	client := &fakeFetcher{handlers: map[string]http.Handler{
		glamaURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchGlama(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

func TestFetchGlama_PaginationStopsOnNoNextPage(t *testing.T) {
	calls := 0
	client := &fakeFetcher{handlers: map[string]http.Handler{
		glamaURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			page := glamaPage{}
			page.PageInfo.HasNextPage = false
			page.Servers = []glamaServer{{Name: "srv1", Namespace: "ns", Slug: "srv1"}}
			jsonHandler(page).ServeHTTP(w, nil)
		}),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchGlama(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 HTTP call (hasNextPage=false stops paging), got %d", calls)
	}
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
	}
}

// ── Smithery ──────────────────────────────────────────────────────────────────

func TestFetchSmithery_ParsesServers(t *testing.T) {
	resp := smitheryPage{}
	resp.Pagination.CurrentPage = 1
	resp.Pagination.TotalPages = 1
	resp.Servers = []smitheryServer{
		{QualifiedName: "gmail", DisplayName: "Gmail", Description: "Manage Gmail", Homepage: "https://smithery.ai/servers/gmail"},
		{QualifiedName: "jina", DisplayName: "Jina AI", Description: "AI search", Homepage: "https://jina.ai"},
		{QualifiedName: "noname"}, // fallback: no DisplayName → use QualifiedName
	}
	client := &fakeFetcher{handlers: map[string]http.Handler{
		smitheryURL: jsonHandler(resp),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchSmithery(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	if items[0].Name != "Gmail" {
		t.Errorf("want DisplayName as Name, got %q", items[0].Name)
	}
	if items[0].Source != SourceSmithery {
		t.Errorf("want source %q, got %q", SourceSmithery, items[0].Source)
	}
	if items[0].Type != TypeMCP {
		t.Errorf("want type %q, got %q", TypeMCP, items[0].Type)
	}
	if items[0].ID != "smithery:gmail" {
		t.Errorf("want id 'smithery:gmail', got %q", items[0].ID)
	}
	// Fallback: no DisplayName → use QualifiedName.
	if items[2].Name != "noname" {
		t.Errorf("want QualifiedName as Name fallback, got %q", items[2].Name)
	}
	// Fallback URL when homepage is empty.
	if items[2].URL != "https://smithery.ai/servers/noname" {
		t.Errorf("want fallback URL for no-homepage item, got %q", items[2].URL)
	}
}

func TestFetchSmithery_HTTPError(t *testing.T) {
	client := &fakeFetcher{handlers: map[string]http.Handler{
		smitheryURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchSmithery(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 503, got nil")
	}
}

func TestFetchSmithery_StopsAtTotalPages(t *testing.T) {
	calls := 0
	client := &fakeFetcher{handlers: map[string]http.Handler{
		smitheryURL: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			resp := smitheryPage{}
			resp.Pagination.CurrentPage = calls
			resp.Pagination.TotalPages = 2 // only 2 pages
			resp.Servers = []smitheryServer{{QualifiedName: "srv", DisplayName: "Srv"}}
			jsonHandler(resp).ServeHTTP(w, r)
		}),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchSmithery(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("want exactly 2 HTTP calls (stops at totalPages=2), got %d", calls)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items (1 per page), got %d", len(items))
	}
}
