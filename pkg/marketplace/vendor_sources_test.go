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

// ── openclaw / ClawHub ────────────────────────────────────────────────────────

func TestFetchOpenclaw_ParsesItems(t *testing.T) {
	page := clawHubPage{
		Items: []clawHubItem{
			{
				Slug:        "couponclaw",
				DisplayName: "CouponClaw",
				Summary:     "Coupon skill",
				Stats: struct {
					Stars int `json:"stars"`
				}{Stars: 42},
			},
			{
				Slug:    "codegen",
				Summary: "Code generation",
			},
		},
	}
	client := &fakeFetcher{handlers: map[string]http.Handler{
		clawHubURL: jsonHandler(page),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchOpenclaw(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "CouponClaw" {
		t.Errorf("want DisplayName as Name, got %q", items[0].Name)
	}
	if items[0].Source != SourceOpenclaw {
		t.Errorf("want source %q, got %q", SourceOpenclaw, items[0].Source)
	}
	if items[0].Type != TypeSkill {
		t.Errorf("want type %q, got %q", TypeSkill, items[0].Type)
	}
	if items[0].Stars != 42 {
		t.Errorf("want stars=42, got %d", items[0].Stars)
	}
	// Fallback: no DisplayName → use slug.
	if items[1].Name != "codegen" {
		t.Errorf("want slug as Name fallback, got %q", items[1].Name)
	}
}

func TestFetchOpenclaw_HTTPError(t *testing.T) {
	client := &fakeFetcher{handlers: map[string]http.Handler{
		clawHubURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}),
	}}
	agg := NewAggregator(nil, client)
	_, err := agg.fetchOpenclaw(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 503, got nil")
	}
}

func TestFetchOpenclaw_NullCursorStopsPages(t *testing.T) {
	// NextCursor null → single page only.
	calls := 0
	client := &fakeFetcher{handlers: map[string]http.Handler{
		clawHubURL: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			page := clawHubPage{Items: []clawHubItem{{Slug: "skill1"}}}
			jsonHandler(page).ServeHTTP(w, nil)
		}),
	}}
	agg := NewAggregator(nil, client)
	items, err := agg.fetchOpenclaw(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 HTTP call (null cursor stops paging), got %d", calls)
	}
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
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
