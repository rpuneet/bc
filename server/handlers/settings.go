package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/home"
)

// SettingsHandler handles /api/settings routes.
type SettingsHandler struct {
	home *home.Home
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(h *home.Home) *SettingsHandler {
	return &SettingsHandler{home: h}
}

// Register mounts settings routes on mux.
func (h *SettingsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/settings", h.handle)
	mux.HandleFunc("/api/settings/injected-instructions", h.handleInjected)
}

// injectedInstructionsBody is the request/response shape for the
// injected-instructions endpoint.
type injectedInstructionsBody struct {
	InjectedInstructions string `json:"injected_instructions"`
}

func (h *SettingsHandler) handleInjected(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, injectedInstructionsBody{
			InjectedInstructions: h.home.Config.InjectedInstructions,
		})
	case http.MethodPut:
		h.putInjected(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *SettingsHandler) putInjected(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req injectedInstructionsBody
	if err := json.Unmarshal(body, &req); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	merged := *h.home.Config
	merged.InjectedInstructions = req.InjectedInstructions
	if err := merged.Validate(); err != nil {
		httpError(w, "validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := merged.Save(h.home.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}
	*h.home.Config = merged

	writeJSON(w, http.StatusOK, injectedInstructionsBody{
		InjectedInstructions: h.home.Config.InjectedInstructions,
	})
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
	hm := h.home
	writeJSON(w, http.StatusOK, hm.Config)
}

// patch applies a partial update to the config. The body is a JSON object
// with top-level keys matching Config fields (user, server, runtime, etc.).
func (h *SettingsHandler) patch(w http.ResponseWriter, r *http.Request) {
	hm := h.home
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
	merged := *hm.Config

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
		case "apps":
			if err := mergeAppsPatch(&merged, raw); err != nil {
				httpError(w, "invalid apps config: "+err.Error(), http.StatusBadRequest)
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
		case "injected_instructions":
			if err := json.Unmarshal(raw, &merged.InjectedInstructions); err != nil {
				httpError(w, "invalid injected_instructions: "+err.Error(), http.StatusBadRequest)
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

	if err := merged.Save(hm.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}
	*hm.Config = merged

	writeJSON(w, http.StatusOK, hm.Config)
}

// mergeAppsPatch merges an "apps" settings patch per instance key so a
// patch containing only {"slack": {...}} never wipes other instances.
// Every submitted instance is validated against its app descriptor —
// unknown apps, unknown config keys, and secret-typed fields are all
// rejected: secrets never travel through /api/settings, they go through
// POST /api/apps/{name} into the vault.
func mergeAppsPatch(merged *home.Config, raw json.RawMessage) error {
	var patch map[string]app.InstanceConfig
	if err := json.Unmarshal(raw, &patch); err != nil {
		return err
	}

	// Copy-on-write: merged shares the Apps map with the live config
	// until the patch is fully validated.
	apps := make(map[string]app.InstanceConfig, len(merged.Apps)+len(patch))
	for k, v := range merged.Apps {
		apps[k] = v
	}
	for name, ic := range patch {
		if !app.ValidInstanceName(name) {
			return fmt.Errorf("invalid instance name %q", name)
		}
		if got := strings.SplitN(name, ":", 2)[0]; ic.App != "" && ic.App != got {
			return fmt.Errorf("instance %q cannot belong to app %q", name, ic.App)
		}
		plugin, ok := app.Get(ic.App)
		if !ok {
			return fmt.Errorf("unknown app %q for instance %q", ic.App, name)
		}
		if err := app.ValidateConfig(plugin.Describe(), ic.Config); err != nil {
			return err
		}
		apps[name] = ic
	}
	merged.Apps = apps
	return nil
}
