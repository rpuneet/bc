package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/template"
)

// sourceTimeout is the per-source HTTP request deadline.
const sourceTimeout = 10 * time.Second

// cacheTTL controls how long a successful aggregate is reused without
// re-fetching from the upstream sources.
const cacheTTL = 1 * time.Hour

// Fetcher is the HTTP transport abstraction used by the aggregator.
// The default is http.DefaultClient; tests inject a fake.
type Fetcher interface {
	Do(req *http.Request) (*http.Response, error)
}

// Aggregator fetches catalog items from all registered sources and
// caches the result for cacheTTL.
type Aggregator struct { //nolint:govet // readability over fieldalignment here
	mu         sync.Mutex
	cached     []Item
	cachedAt   time.Time
	tmplStore  *template.Store // may be nil
	httpClient Fetcher
	hasCache   bool // true once a successful aggregate has been stored
}

// NewAggregator constructs an Aggregator. tmplStore may be nil (mycel
// source is skipped). httpClient may be nil (http.DefaultClient used).
func NewAggregator(tmplStore *template.Store, httpClient Fetcher) *Aggregator {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Aggregator{
		tmplStore:  tmplStore,
		httpClient: httpClient,
	}
}

// List returns the aggregated catalog, refreshing from upstream when
// the cache is stale. Filters are applied after aggregation.
func (a *Aggregator) List(ctx context.Context, typeFilter, sourceFilter, query string) ([]Item, error) {
	items, err := a.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return filter(items, typeFilter, sourceFilter, query), nil
}

// catalog returns the cached catalog, refreshing it if stale.
func (a *Aggregator) catalog(ctx context.Context) ([]Item, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.hasCache && time.Since(a.cachedAt) < cacheTTL {
		return a.cached, nil
	}

	items, err := a.aggregate(ctx)
	if err != nil {
		// Return stale data rather than an error when we have a prior result.
		if a.hasCache {
			log.Warn("marketplace: refresh failed, serving stale cache", "error", err)
			return a.cached, nil
		}
		return nil, err
	}
	a.cached = items
	a.cachedAt = time.Now()
	a.hasCache = true
	return items, nil
}

// aggregate collects items from all sources concurrently, logging which
// sources were included or skipped.
func (a *Aggregator) aggregate(ctx context.Context) ([]Item, error) {
	type result struct { //nolint:govet // readability over fieldalignment
		source Source
		items  []Item
		err    error
	}

	sources := []struct { //nolint:govet // readability over fieldalignment
		name Source
		fn   func(context.Context) ([]Item, error)
	}{
		{SourceMCPRegistry, a.fetchMCPRegistry},
		{SourceGitHub, a.fetchGitHub},
		{SourceMycel, a.fetchMycel},
		{SourceClaude, a.fetchClaude},
		{SourceOpenclaw, a.fetchOpenclaw},
		{SourceGemini, a.fetchGemini},
		{SourceGlama, a.fetchGlama},
		{SourceSmithery, a.fetchSmithery},
	}

	ch := make(chan result, len(sources))
	for _, s := range sources {
		s := s
		go func() {
			items, err := s.fn(ctx)
			ch <- result{source: s.name, items: items, err: err}
		}()
	}

	var all []Item
	for range sources {
		r := <-ch
		if r.err != nil {
			log.Warn("marketplace: source skipped", "source", r.source, "error", r.err)
			continue
		}
		log.Info("marketplace: source loaded", "source", r.source, "count", len(r.items))
		all = append(all, r.items...)
	}
	return all, nil
}

// ── MCP registry ──────────────────────────────────────────────────────────────

// mcpRegistryURL is the official MCP server registry.
const mcpRegistryURL = "https://registry.modelcontextprotocol.io/v0.1/servers"

// mcpRegistryPage is the paginated response from the MCP registry.
type mcpRegistryPage struct {
	Servers  []mcpRegistryEntry `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor"`
		Count      int    `json:"count"`
	} `json:"metadata"`
}

type mcpRegistryEntry struct {
	Server struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Repository  *struct {
			URL string `json:"url"`
		} `json:"repository,omitempty"`
		Packages []struct {
			RegistryName string `json:"registry_name"`
			Name         string `json:"name"`
		} `json:"packages,omitempty"`
	} `json:"server"`
}

// fetchMCPRegistry pages through the official MCP registry and returns
// all servers as catalog items. A single page that errors is skipped;
// we stop when there is no nextCursor or we hit 500 items.
func (a *Aggregator) fetchMCPRegistry(ctx context.Context) ([]Item, error) {
	const maxItems = 500
	var items []Item
	cursor := ""

	for len(items) < maxItems {
		u := mcpRegistryURL + "?limit=100"
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}

		var page mcpRegistryPage
		pageCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		err := a.getJSON(pageCtx, u, &page)
		cancel()
		if err != nil {
			if len(items) > 0 {
				break // partial result is still useful
			}
			return nil, fmt.Errorf("mcp-registry fetch: %w", err)
		}

		for _, e := range page.Servers {
			srvURL := ""
			if e.Server.Repository != nil {
				srvURL = e.Server.Repository.URL
			}
			name := e.Server.Name
			id := "mcp-registry:" + name
			items = append(items, Item{
				ID:          id,
				Name:        name,
				Description: e.Server.Description,
				URL:         srvURL,
				Source:      SourceMCPRegistry,
				Type:        TypeMCP,
				InstallSpec: name,
			})
		}

		cursor = page.Metadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return items, nil
}

// ── GitHub ────────────────────────────────────────────────────────────────────

// githubSearchURL is the GitHub code-search API endpoint for repositories.
const githubSearchURL = "https://api.github.com/search/repositories"

// githubSearchResponse is the relevant subset of the GitHub search API.
type githubSearchResponse struct {
	Items []githubRepo `json:"items"`
}

type githubRepo struct { //nolint:govet // field order matches JSON/API contract
	FullName    string   `json:"full_name"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	HTMLURL     string   `json:"html_url"`
	Topics      []string `json:"topics"`
	StarCount   int      `json:"stargazers_count"`
}

// fetchGitHub queries GitHub for MCP-server and Claude-skill repos that
// meet the GitHubStarsThreshold. Two queries are issued (one per topic)
// to maximize coverage; duplicates are deduplicated by full_name.
func (a *Aggregator) fetchGitHub(ctx context.Context) ([]Item, error) {
	queries := []struct {
		q        string
		itemType ItemType
	}{
		{
			fmt.Sprintf("topic:mcp-server stars:>=%d", GitHubStarsThreshold),
			TypeMCP,
		},
		{
			fmt.Sprintf("topic:claude-skill stars:>=%d", GitHubStarsThreshold),
			TypeSkill,
		},
	}

	seen := map[string]bool{}
	var items []Item

	for _, q := range queries {
		u := fmt.Sprintf("%s?q=%s&sort=stars&order=desc&per_page=100",
			githubSearchURL, url.QueryEscape(q.q))

		var resp githubSearchResponse
		qCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		err := a.getJSON(qCtx, u, &resp)
		cancel()
		if err != nil {
			log.Warn("marketplace: github query skipped", "query", q.q, "error", err)
			continue
		}

		for _, r := range resp.Items {
			if seen[r.FullName] {
				continue
			}
			seen[r.FullName] = true
			if r.StarCount < GitHubStarsThreshold {
				continue
			}
			items = append(items, Item{
				ID:          "github:" + r.FullName,
				Name:        r.Name,
				Description: r.Description,
				URL:         r.HTMLURL,
				Stars:       r.StarCount,
				Source:      SourceGitHub,
				Type:        q.itemType,
				InstallSpec: r.HTMLURL,
			})
		}
	}
	return items, nil
}

// ── Mycel templates ───────────────────────────────────────────────────────────

// fetchMycel returns the local mycel template store as catalog items.
func (a *Aggregator) fetchMycel(ctx context.Context) ([]Item, error) {
	_ = ctx // no I/O; context kept for interface consistency
	if a.tmplStore == nil {
		return nil, nil
	}
	templates, err := a.tmplStore.List()
	if err != nil {
		return nil, fmt.Errorf("mycel templates: %w", err)
	}
	items := make([]Item, 0, len(templates))
	for _, t := range templates {
		items = append(items, Item{
			ID:          "mycel:" + t.Name,
			Name:        t.Name,
			Description: t.Description,
			Source:      SourceMycel,
			Type:        TypeTemplate,
			InstallSpec: t.Name,
		})
	}
	return items, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// getJSON performs a GET to rawURL and unmarshals the response into out.
// It respects ctx for cancellation and timeout. out must be a non-nil pointer.
func (a *Aggregator) getJSON(ctx context.Context, rawURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mycel-marketplace/1 (+https://bc-infra.com)")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

// filter applies optional type, source, and text filters to items.
func filter(items []Item, typeStr, sourceStr, query string) []Item {
	if typeStr == "" && sourceStr == "" && query == "" {
		return items
	}
	q := strings.ToLower(query)
	out := items[:0:0] // share backing array but start empty
	for _, it := range items {
		if typeStr != "" && string(it.Type) != typeStr {
			continue
		}
		if sourceStr != "" && string(it.Source) != sourceStr {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(it.Name), q) &&
				!strings.Contains(strings.ToLower(it.Description), q) {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}
