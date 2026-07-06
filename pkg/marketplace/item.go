// Package marketplace provides a live aggregator for MCP servers, skills,
// and prompt templates from public registries and the local mycel template store.
package marketplace

// ItemType classifies what kind of catalog entry an item represents.
type ItemType string

const (
	TypeMCP      ItemType = "mcp"
	TypeSkill    ItemType = "skill"
	TypeTemplate ItemType = "template"
)

// Source identifies which upstream registry an item came from.
type Source string

const (
	SourceMCPRegistry Source = "mcp-registry" // registry.modelcontextprotocol.io
	SourceGitHub      Source = "github"       // GitHub search (stars-gated)
	SourceMycel       Source = "mycel"        // local mycel template store
	SourceClaude      Source = "claude"       // github.com/anthropics/skills
	SourceOpenclaw    Source = "openclaw"     // clawhub.ai/api/v1/skills
	SourceGemini      Source = "gemini"       // github.com/orgs/gemini-cli-extensions
	SourceGlama       Source = "glama"        // glama.ai/api/mcp/v1/servers
	SourceSmithery    Source = "smithery"     // registry.smithery.ai/servers
	// SourcePulseMCP is intentionally absent: pulsemcp.com has no public machine-readable API.
)

// GitHubStarsThreshold is the minimum star count for a GitHub repo to
// appear in the catalog. Keeps the listing focused on quality repos.
const GitHubStarsThreshold = 1000

// Item is a single entry in the marketplace catalog.
//
// Field order follows the JSON/API contract; govet fieldalignment
// is satisfied (pointers and strings after ints).
type Item struct { //nolint:govet // field order matches JSON/API contract
	Stars       int      `json:"stars,omitempty"`
	Name        string   `json:"name"`
	ID          string   `json:"id"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url,omitempty"`
	Source      Source   `json:"source"`
	Type        ItemType `json:"type"`
	// InstallSpec carries the minimal spec needed to wire the item.
	// For MCP items this is the server name; for templates it is the
	// mycel template name. Deep install wiring is a follow-up.
	InstallSpec string `json:"install_spec,omitempty"`
}
