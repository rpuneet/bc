package marketplace

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rpuneet/mycel/pkg/log"
)

// ── Claude / Anthropic skills ─────────────────────────────────────────────────

// claudePluginsURL is the official Anthropic Claude-plugins-official catalog.
// It is a static JSON file hosted on GitHub, not a paginated REST API.
// Source: github.com/anthropics/claude-plugins-official
const claudePluginsURL = "https://raw.githubusercontent.com/anthropics/claude-plugins-official/main/.claude-plugin/marketplace.json"

// claudeMarketplace is the top-level shape of the Anthropic marketplace.json.
type claudeMarketplace struct {
	Plugins []claudePlugin `json:"plugins"`
}

// claudePlugin is a single entry in the Anthropic official plugins catalog.
type claudePlugin struct { //nolint:govet // field order matches JSON
	Author struct {
		Name string `json:"name"`
	} `json:"author,omitempty"`
	Source struct {
		URL string `json:"url"`
	} `json:"source,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	Category    string `json:"category,omitempty"`
}

// fetchClaude fetches the official Anthropic/Claude plugins catalog and returns
// them as marketplace items tagged with SourceClaude and TypeSkill.
func (a *Aggregator) fetchClaude(ctx context.Context) ([]Item, error) {
	pluginCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()

	var page claudeMarketplace
	if err := a.getJSON(pluginCtx, claudePluginsURL, &page); err != nil {
		return nil, fmt.Errorf("claude plugins fetch: %w", err)
	}

	items := make([]Item, 0, len(page.Plugins))
	for _, p := range page.Plugins {
		link := p.Homepage
		if link == "" {
			link = p.Source.URL
		}
		items = append(items, Item{
			ID:          "claude:" + p.Name,
			Name:        p.Name,
			Description: p.Description,
			URL:         link,
			Source:      SourceClaude,
			Type:        TypeSkill,
			InstallSpec: p.Name,
		})
	}

	log.Info("marketplace: claude source loaded", "count", len(items))
	return items, nil
}

// ── Google Gemini CLI extensions ──────────────────────────────────────────────

// geminiExtOrgURL is the GitHub API endpoint for the gemini-cli-extensions org.
// Google maintains this org with officially blessed Gemini CLI extensions.
// Source: github.com/gemini-cli-extensions
const geminiExtOrgURL = "https://api.github.com/orgs/gemini-cli-extensions/repos?per_page=100&sort=stargazers"

// fetchGemini fetches the gemini-cli-extensions GitHub org and returns each
// repo as a marketplace item tagged SourceGemini. Items with zero stars are
// still included (the org is small and curated by Google).
func (a *Aggregator) fetchGemini(ctx context.Context) ([]Item, error) {
	gemCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()

	var repos []githubRepo
	if err := a.getJSON(gemCtx, geminiExtOrgURL, &repos); err != nil {
		return nil, fmt.Errorf("gemini extensions fetch: %w", err)
	}

	items := make([]Item, 0, len(repos))
	for _, r := range repos {
		items = append(items, Item{
			ID:          "gemini:" + r.FullName,
			Name:        r.Name,
			Description: r.Description,
			URL:         r.HTMLURL,
			Stars:       r.StarCount,
			Source:      SourceGemini,
			Type:        TypeSkill,
			InstallSpec: r.HTMLURL,
		})
	}
	return items, nil
}

// ── Glama ─────────────────────────────────────────────────────────────────────

// glamaURL is the Glama MCP server registry (cursor-based pagination).
// Docs: https://glama.ai/mcp/servers
const glamaURL = "https://glama.ai/api/mcp/v1/servers"

// glamaPage is the top-level paginated response from the Glama registry.
type glamaPage struct {
	PageInfo struct {
		EndCursor   string `json:"endCursor"`
		HasNextPage bool   `json:"hasNextPage"`
	} `json:"pageInfo"`
	Servers []glamaServer `json:"servers"`
}

// glamaServer is a single entry from the Glama registry.
type glamaServer struct { //nolint:govet // field order matches JSON/API contract
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// fetchGlama pages through the Glama MCP registry (cursor-based) and returns
// up to maxItems servers as catalog items. A page that errors after we already
// have results is silently truncated; a first-page failure is returned as an
// error so the aggregator can log the skip.
func (a *Aggregator) fetchGlama(ctx context.Context) ([]Item, error) {
	const maxItems = 500
	var items []Item
	cursor := ""

	for len(items) < maxItems {
		u := glamaURL + "?first=100"
		if cursor != "" {
			u += "&after=" + url.QueryEscape(cursor)
		}

		var page glamaPage
		pageCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		err := a.getJSON(pageCtx, u, &page)
		cancel()
		if err != nil {
			if len(items) > 0 {
				break // partial result is still useful
			}
			return nil, fmt.Errorf("glama fetch: %w", err)
		}

		for _, s := range page.Servers {
			id := "glama:" + s.Namespace + "/" + s.Slug
			items = append(items, Item{
				ID:          id,
				Name:        s.Name,
				Description: s.Description,
				URL:         s.URL,
				Source:      SourceGlama,
				Type:        TypeMCP,
				InstallSpec: s.Slug,
			})
		}

		if !page.PageInfo.HasNextPage {
			break
		}
		cursor = page.PageInfo.EndCursor
		if cursor == "" {
			break
		}
	}
	return items, nil
}

// ── Smithery ──────────────────────────────────────────────────────────────────

// smitheryURL is the Smithery MCP server registry (page-based pagination).
// Docs: https://smithery.ai
const smitheryURL = "https://registry.smithery.ai/servers"

// smitheryPage is the paginated response from the Smithery registry.
type smitheryPage struct { //nolint:govet // field order matches JSON/API contract
	Pagination struct {
		CurrentPage int `json:"currentPage"`
		TotalPages  int `json:"totalPages"`
	} `json:"pagination"`
	Servers []smitheryServer `json:"servers"`
}

// smitheryServer is a single entry from the Smithery registry.
type smitheryServer struct { //nolint:govet // field order matches JSON/API contract
	QualifiedName string `json:"qualifiedName"`
	DisplayName   string `json:"displayName"`
	Description   string `json:"description"`
	Homepage      string `json:"homepage"`
}

// fetchSmithery pages through the Smithery MCP registry and returns up to
// maxPages × pageSize servers as catalog items. The page count is capped at
// maxPages so a very large registry does not stall the aggregator.
func (a *Aggregator) fetchSmithery(ctx context.Context) ([]Item, error) {
	const (
		pageSize = 100
		maxPages = 5 // cap at 500 items
	)
	var items []Item

	for page := 1; page <= maxPages; page++ {
		u := fmt.Sprintf("%s?pageSize=%d&page=%d", smitheryURL, pageSize, page)

		var resp smitheryPage
		pageCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		err := a.getJSON(pageCtx, u, &resp)
		cancel()
		if err != nil {
			if len(items) > 0 {
				break // partial result is still useful
			}
			return nil, fmt.Errorf("smithery fetch: %w", err)
		}

		for _, s := range resp.Servers {
			name := s.DisplayName
			if name == "" {
				name = s.QualifiedName
			}
			link := s.Homepage
			if link == "" {
				link = "https://smithery.ai/servers/" + s.QualifiedName
			}
			items = append(items, Item{
				ID:          "smithery:" + s.QualifiedName,
				Name:        name,
				Description: s.Description,
				URL:         link,
				Source:      SourceSmithery,
				Type:        TypeMCP,
				InstallSpec: s.QualifiedName,
			})
		}

		// Stop if the page was empty (covers TotalPages==0 / omitted).
		if len(resp.Servers) == 0 {
			break
		}
		// Stop when TotalPages is known and we've reached the last page.
		if resp.Pagination.TotalPages > 0 && page >= resp.Pagination.TotalPages {
			break
		}
	}
	return items, nil
}

// ── PulseMCP ──────────────────────────────────────────────────────────────────
//
// PulseMCP (pulsemcp.com) provides a curated newsletter/directory for MCP
// servers but has no public machine-readable API as of 2026-07.
// Checked endpoints: api.pulsemcp.com/v0/servers, pulsemcp.com/api/servers —
// all return HTTP 404.
//
// fetchPulseMCP is intentionally absent; SourcePulseMCP is not wired into the
// aggregator. This is documented in the PR body.

// ── OpenAI ────────────────────────────────────────────────────────────────────
//
// OpenAI deprecated its ChatGPT plugin store on 2024-04-09 and has not
// published a replacement public catalog API. The chatgpt.com/apps directory
// launched in late 2025 is browse-only with no machine-readable feed.
//
// fetchOpenAI is intentionally absent; SourceOpenAI is not wired into the
// aggregator. This is documented in the PR body.
