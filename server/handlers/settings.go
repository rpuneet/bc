package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// SettingsHandler handles /api/settings routes.
type SettingsHandler struct {
	ws *workspace.Workspace
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(ws *workspace.Workspace) *SettingsHandler {
	return &SettingsHandler{ws: ws}
}

// Register mounts settings routes on mux.
func (h *SettingsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/settings", h.handle)
}

func (h *SettingsHandler) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPatch:
		h.patch(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *SettingsHandler) get(w http.ResponseWriter, r *http.Request) {
	ws := h.ws
	writeJSON(w, http.StatusOK, ws.Config)
}

// patch applies a partial update to the config. The body is a JSON object
// with top-level keys matching Config fields (user, server, runtime, etc.).
func (h *SettingsHandler) patch(w http.ResponseWriter, r *http.Request) {
	ws := h.ws
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var rawPatch map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawPatch); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Copy current config to avoid corrupting on error.
	merged := *ws.Config

	for key, raw := range rawPatch {
		switch key {
		case "user":
			if err := json.Unmarshal(raw, &merged.User); err != nil {
				httpError(w, "invalid user config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "server":
			if err := json.Unmarshal(raw, &merged.Server); err != nil {
				httpError(w, "invalid server config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "runtime":
			if err := json.Unmarshal(raw, &merged.Runtime); err != nil {
				httpError(w, "invalid runtime config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "providers":
			if err := json.Unmarshal(raw, &merged.Providers); err != nil {
				httpError(w, "invalid providers config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "gateways":
			if err := workspace.MergeGatewaysPatch(&merged.Gateways, raw); err != nil {
				httpError(w, "invalid gateways config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "cron":
			if err := json.Unmarshal(raw, &merged.Cron); err != nil {
				httpError(w, "invalid cron config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "storage":
			if err := json.Unmarshal(raw, &merged.Storage); err != nil {
				httpError(w, "invalid storage config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "logs":
			if err := json.Unmarshal(raw, &merged.Logs); err != nil {
				httpError(w, "invalid logs config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "ui":
			if err := json.Unmarshal(raw, &merged.UI); err != nil {
				httpError(w, "invalid ui config: "+err.Error(), http.StatusBadRequest)
				return
			}
		case "version":
			// ignore
		default:
			httpError(w, "unknown section: "+key, http.StatusBadRequest)
			return
		}
	}

	if err := merged.Validate(); err != nil {
		httpError(w, "validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := merged.Save(ws.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}
	*ws.Config = merged

	writeJSON(w, http.StatusOK, ws.Config)
}

// Gateway patches are deep-merged per platform key via
// workspace.MergeGatewaysPatch.
