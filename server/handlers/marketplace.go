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

// installResponse is returned on success.
type installResponse struct {
	Dispatched int `json:"dispatched"`
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

	msg := composeInstallMessage(req)

	dispatched := 0
	for _, agentName := range req.Agents {
		if err := h.sender.Send(r.Context(), agentName, msg); err != nil {
			// Log and continue — other agents should still receive the message.
			httpError(w, fmt.Sprintf("send to agent %q: %s", agentName, err.Error()), http.StatusBadRequest)
			return
		}
		dispatched++
	}

	writeJSON(w, http.StatusOK, installResponse{Dispatched: dispatched})
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
		spec := req.ItemSourceURL
		if spec == "" {
			spec = req.ItemID
		}
		sb.WriteString(fmt.Sprintf("  claude mcp add %q %s\n", req.ItemName, spec))
	case marketplace.TypeSkill:
		switch marketplace.Source(req.ItemSource) {
		case marketplace.SourceOpenclaw:
			sb.WriteString(fmt.Sprintf("  clawhub install %s\n", req.ItemID))
		default:
			spec := req.ItemSourceURL
			if spec == "" {
				spec = req.ItemID
			}
			sb.WriteString(fmt.Sprintf("  claude skill install %s\n", spec))
		}
	case marketplace.TypeTemplate:
		sb.WriteString(fmt.Sprintf("  bc template import %s\n", req.ItemName))
	default:
		sb.WriteString("  Consult the item URL above for installation instructions.\n")
	}

	return sb.String()
}
