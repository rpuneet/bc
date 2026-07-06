package handlers

import (
	"net/http"

	"github.com/rpuneet/mycel/pkg/marketplace"
)

// MarketplaceHandler handles /api/marketplace routes.
type MarketplaceHandler struct {
	agg *marketplace.Aggregator
}

// NewMarketplaceHandler creates a MarketplaceHandler backed by the given aggregator.
func NewMarketplaceHandler(agg *marketplace.Aggregator) *MarketplaceHandler {
	return &MarketplaceHandler{agg: agg}
}

// Register mounts marketplace routes on mux.
func (h *MarketplaceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/marketplace", h.list)
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
