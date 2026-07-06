package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/marketplace"
)

// AgentSender is the minimal interface needed to dispatch a message to a named
// agent. *agent.AgentService satisfies this interface.
type AgentSender interface {
	Send(ctx context.Context, name, message string) error
}

// MarketplaceHandler handles /api/marketplace routes.
type MarketplaceHandler struct { //nolint:govet // readability over fieldalignment
	agg    *marketplace.Aggregator
	sender AgentSender // may be nil; install endpoint returns 503 when nil
}

// NewMarketplaceHandler creates a MarketplaceHandler backed by the given
// aggregator. sender may be nil; the install endpoint will return 503 when
// no sender is wired (e.g. during integration tests that omit the agent service).
func NewMarketplaceHandler(agg *marketplace.Aggregator, sender AgentSender) *MarketplaceHandler {
	return &MarketplaceHandler{agg: agg, sender: sender}
}

// Register mounts marketplace routes on mux.
func (h *MarketplaceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/marketplace", h.list)
	mux.HandleFunc("/api/marketplace/install", h.install)
}

// list handles GET /api/marketplace with optional ?type=, ?source=, ?q= filters.
func (h *MarketplaceHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	typeFilter := r.URL.Query().Get("type")
	sourceFilter := r.URL.Query().Get("source")
	query := r.URL.Query().Get("q")

	items, err := h.agg.List(r.Context(), typeFilter, sourceFilter, query)
	if err != nil {
		httpInternalError(w, "aggregate marketplace", err)
		return
	}

	// Return an empty array rather than null when there are no results.
	if items == nil {
		items = []marketplace.Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

// installRequest is the body accepted by POST /api/marketplace/install.
type installRequest struct { //nolint:govet // field order matches JSON/API contract
	Agents        []string `json:"agents"`
	ItemID        string   `json:"item_id"`
	ItemName      string   `json:"item_name"`
	ItemSourceURL string   `json:"item_source_url"`
	ItemType      string   `json:"item_type"`
	ItemSource    string   `json:"item_source"`
}

// installResponse is returned on success (HTTP 200).
// Errors lists per-agent failures so the caller can surface which agents were
// not reached; a non-empty Errors slice alongside Dispatched>0 indicates a
// partial success.
type installResponse struct { //nolint:govet // field order matches API contract
	Dispatched int      `json:"dispatched"`
	Errors     []string `json:"errors,omitempty"`
}

// install handles POST /api/marketplace/install.
//
// It composes a clear install-instruction message for the item and dispatches
// it to each named agent via the existing AgentSender (same code path as
// POST /api/agents/{name}/send). The agent then runs the install using its
// own native command.
func (h *MarketplaceHandler) install(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	if h.sender == nil {
		httpError(w, "agent service unavailable", http.StatusServiceUnavailable)
		return
	}

	var req installRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.ItemName) == "" {
		httpError(w, "item_name must not be empty", http.StatusBadRequest)
		return
	}
	if len(req.Agents) == 0 {
		httpError(w, "agents must not be empty", http.StatusBadRequest)
		return
	}

	// Strip newline characters to prevent injection into the structured message.
	newlineReplacer := strings.NewReplacer("\n", "", "\r", "")
	req.ItemName = newlineReplacer.Replace(req.ItemName)
	req.ItemSourceURL = newlineReplacer.Replace(req.ItemSourceURL)
	req.ItemID = newlineReplacer.Replace(req.ItemID)

	msg := composeInstallMessage(req)

	dispatched := 0
	var sendErrs []string
	for _, agentName := range req.Agents {
		if err := h.sender.Send(r.Context(), agentName, msg); err != nil {
			// Collect the error and continue — other agents should still receive the message.
			sendErrs = append(sendErrs, fmt.Sprintf("agent %q: %s", agentName, err.Error()))
			continue
		}
		dispatched++
	}

	// If no agents were reached at all, surface it as a server-side error (the
	// agents are the remote resource; caller request was valid).
	if dispatched == 0 && len(sendErrs) > 0 {
		httpError(w, "no agents reached: "+strings.Join(sendErrs, "; "), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, installResponse{Dispatched: dispatched, Errors: sendErrs})
}

// composeInstallMessage builds the human-readable install instruction sent to
// each target agent. The message tells the agent what to install and how.
func composeInstallMessage(req installRequest) string {
	var sb strings.Builder
	sb.WriteString("[mycel marketplace] Install request\n\n")
	sb.WriteString("Item:   " + req.ItemName + "\n")
	if req.ItemSource != "" {
		sb.WriteString("Source: " + req.ItemSource + "\n")
	}
	if req.ItemType != "" {
		sb.WriteString("Type:   " + req.ItemType + "\n")
	}
	if req.ItemSourceURL != "" {
		sb.WriteString("URL:    " + req.ItemSourceURL + "\n")
	}
	sb.WriteString("\nPlease install this using your native command:\n")

	switch marketplace.ItemType(req.ItemType) {
	case marketplace.TypeMCP:
		if marketplace.Source(req.ItemSource) == marketplace.SourceGlama {
			// Glama's listing API does not expose the server's runnable endpoint or
			// command — only the listing-page URL. Emitting "claude mcp add <listing-url>"
			// would create a broken stdio entry pointing at an HTML page, not a server.
			// Emit an honest instruction so the agent is not handed a command that fails.
			listingURL := req.ItemSourceURL
			if listingURL == "" {
				listingURL = "https://glama.ai/mcp/servers"
			}
			sb.WriteString("  Glama's catalog API does not expose the server's runnable\n")
			sb.WriteString("  endpoint, so a ready-made install command cannot be composed.\n")
			sb.WriteString(fmt.Sprintf("  1. Open %s\n", listingURL))
			sb.WriteString("  2. Find the server's install command (npx/uvx invocation or HTTP/SSE URL).\n")
			sb.WriteString(fmt.Sprintf("  3. Run: claude mcp add %q <command-or-url>\n", req.ItemName))
		} else {
			spec := req.ItemSourceURL
			if spec == "" {
				spec = req.ItemID
			}
			sb.WriteString(fmt.Sprintf("  claude mcp add %q %q\n", req.ItemName, spec))
		}
	case marketplace.TypeSkill:
		switch marketplace.Source(req.ItemSource) {
		case marketplace.SourceOpenclaw:
			// The correct CLI is the openclaw CLI, not the non-existent "clawhub" binary.
			// Command: openclaw skills install <slug>
			// The item ID may carry an "openclaw:" prefix; strip it to get the bare slug.
			slug := strings.TrimPrefix(req.ItemID, "openclaw:")
			sb.WriteString(fmt.Sprintf("  openclaw skills install %q\n", slug))
		default:
			repoURL := req.ItemSourceURL
			if repoURL == "" {
				repoURL = req.ItemID
			}
			marketplaceName := marketplaceNameFromURL(repoURL)
			sb.WriteString("Step 1 — register the marketplace (first time only):\n")
			sb.WriteString(fmt.Sprintf("  claude plugin marketplace add %q\n", repoURL))
			sb.WriteString("Step 2 — install the plugin:\n")
			sb.WriteString(fmt.Sprintf("  claude plugin install %q\n", req.ItemName+"@"+marketplaceName))
		}
	case marketplace.TypeTemplate:
		sb.WriteString(fmt.Sprintf("  mycel template import %q\n", req.ItemName))
	default:
		sb.WriteString("  Consult the item URL above for installation instructions.\n")
	}

	return sb.String()
}

// marketplaceNameFromURL extracts the GitHub repository name from a URL such as
// https://github.com/owner/repo[/tree/…] → "repo".
// Falls back to the full rawURL when the pattern is not recognized.
func marketplaceNameFromURL(rawURL string) string {
	// Strip scheme + host, then split on "/" to get path segments.
	u := strings.TrimPrefix(rawURL, "https://")
	u = strings.TrimPrefix(u, "http://")
	// Segments: ["github.com", "owner", "repo", …]
	parts := strings.SplitN(u, "/", 4)
	if len(parts) >= 3 && parts[2] != "" {
		return parts[2]
	}
	return rawURL
}
