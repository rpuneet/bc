package marketplace

import (
	"context"
	"fmt"

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

// ── openclaw / ClawHub ────────────────────────────────────────────────────────

// clawHubURL is the ClawHub public skills registry (no auth required for reads).
// Docs: https://docs.openclaw.ai/clawhub/http-api
const clawHubURL = "https://clawhub.ai/api/v1/skills"

// clawHubPage is the paginated response from ClawHub.
type clawHubPage struct {
	NextCursor interface{}   `json:"nextCursor"` // null or cursor string; interface{} allows JSON null
	Items      []clawHubItem `json:"items"`
}

// clawHubItem is a single skill entry from ClawHub.
type clawHubItem struct { //nolint:govet // field order matches JSON/API contract
	Stats struct {
		Stars int `json:"stars"`
	} `json:"stats"`
	LatestVersion struct {
		Version string `json:"version"`
	} `json:"latestVersion"`
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
}

// fetchOpenclaw pages through the ClawHub registry (up to maxItems) and returns
// openclaw skills as marketplace items. Pagination uses the cursor field.
func (a *Aggregator) fetchOpenclaw(ctx context.Context) ([]Item, error) {
	const maxItems = 300
	var items []Item

	u := clawHubURL + "?limit=100&sort=stars"
	for len(items) < maxItems {
		var page clawHubPage
		pageCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		err := a.getJSON(pageCtx, u, &page)
		cancel()
		if err != nil {
			if len(items) > 0 {
				break // partial result is still useful
			}
			return nil, fmt.Errorf("clawhub fetch: %w", err)
		}

		for _, s := range page.Items {
			name := s.DisplayName
			if name == "" {
				name = s.Slug
			}
			items = append(items, Item{
				ID:          "openclaw:" + s.Slug,
				Name:        name,
				Description: s.Summary,
				URL:         "https://clawhub.ai/skills/" + s.Slug,
				Stars:       s.Stats.Stars,
				Source:      SourceOpenclaw,
				Type:        TypeSkill,
				InstallSpec: s.Slug,
			})
		}

		// nextCursor is null (nil) when there are no more pages.
		if page.NextCursor == nil {
			break
		}
		cursor, ok := page.NextCursor.(string)
		if !ok || cursor == "" {
			break
		}
		u = clawHubURL + "?limit=100&sort=stars&cursor=" + cursor
	}
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

// ── OpenAI ────────────────────────────────────────────────────────────────────
//
// OpenAI deprecated its ChatGPT plugin store on 2024-04-09 and has not
// published a replacement public catalog API. The chatgpt.com/apps directory
// launched in late 2025 is browse-only with no machine-readable feed.
//
// fetchOpenAI is intentionally absent; SourceOpenAI is not wired into the
// aggregator. This is documented in the PR body.
