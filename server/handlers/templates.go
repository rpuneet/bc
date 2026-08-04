package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/template"
)

// TemplateHandler handles /api/templates routes.
type TemplateHandler struct {
	store *template.Store
}

// NewTemplateHandler creates a TemplateHandler backed by the given store.
func NewTemplateHandler(store *template.Store) *TemplateHandler {
	return &TemplateHandler{store: store}
}

// Register mounts template routes on mux.
func (h *TemplateHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/templates", h.list)
	mux.HandleFunc("/api/templates/", h.byName)
}

// templateRequest is the JSON body for creating/updating a template.
// SystemPrompt is a pointer so that PUT can distinguish between
// "clear the prompt" (explicit empty string) and "don't change" (field absent).
type templateRequest struct { //nolint:govet // field order matches JSON/API contract
	ToolPolicies     *template.ToolPolicies `json:"tool_policies,omitempty"`
	MCPs             []string               `json:"mcps,omitempty"`
	Secrets          []string               `json:"secrets,omitempty"`
	Plugins          []string               `json:"plugins,omitempty"`
	ContextFiles     []string               `json:"context_files,omitempty"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description,omitempty"`
	Label            string                 `json:"label,omitempty"`
	SystemPrompt     *string                `json:"system_prompt,omitempty"`
	SystemPromptFile string                 `json:"system_prompt_file,omitempty"`
	MaxCostUSD       float64                `json:"max_cost_usd,omitempty"`
	StuckTimeoutMin  int                    `json:"stuck_timeout_min,omitempty"`
}

func (req *templateRequest) toTemplate() template.Template {
	return template.Template{
		Name:             req.Name,
		Description:      req.Description,
		Label:            req.Label,
		SystemPromptFile: req.SystemPromptFile,
		MCPs:             req.MCPs,
		Secrets:          req.Secrets,
		Plugins:          req.Plugins,
		ContextFiles:     req.ContextFiles,
		ToolPolicies:     req.ToolPolicies,
		MaxCostUSD:       req.MaxCostUSD,
		StuckTimeoutMin:  req.StuckTimeoutMin,
	}
}

// templateResponse extends Template with the rendered system prompt.
type templateResponse struct {
	SystemPrompt string `json:"system_prompt,omitempty"`
	template.Template
}

// list handles GET /api/templates and POST /api/templates.
func (h *TemplateHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		templates, err := store.List()
		if err != nil {
			httpInternalError(w, "list templates", err)
			return
		}
		writeJSON(w, http.StatusOK, templates)

	case http.MethodPost:
		var req templateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			httpError(w, "template name is required", http.StatusBadRequest)
			return
		}

		t := req.toTemplate()
		prompt := ""
		if req.SystemPrompt != nil {
			prompt = *req.SystemPrompt
		}
		if err := store.Create(t, prompt, ""); err != nil {
			// Distinguish conflict from internal error
			if strings.Contains(err.Error(), "already exists") {
				httpError(w, err.Error(), http.StatusConflict)
				return
			}
			httpInternalError(w, "create template", err)
			return
		}

		created, prompt, err := store.Get(req.Name)
		if err != nil {
			httpInternalError(w, "fetch created template", err)
			return
		}
		writeJSON(w, http.StatusCreated, templateResponse{Template: *created, SystemPrompt: prompt})

	default:
		methodNotAllowed(w)
	}
}

// byName handles GET/PUT/DELETE /api/templates/{name}.
func (h *TemplateHandler) byName(w http.ResponseWriter, r *http.Request) {
	store := h.store
	name := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	if name == "" {
		httpError(w, "template name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, prompt, err := store.Get(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httpError(w, err.Error(), http.StatusNotFound)
				return
			}
			httpInternalError(w, "get template", err)
			return
		}
		writeJSON(w, http.StatusOK, templateResponse{Template: *t, SystemPrompt: prompt})

	case http.MethodPut:
		var req templateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Name = name // URL name takes precedence

		// Determine the prompt to write:
		// - explicit string (including "") → use as-is (allows clearing the prompt)
		// - field absent (nil) → preserve the existing prompt
		var prompt string
		if req.SystemPrompt != nil {
			prompt = *req.SystemPrompt
		} else {
			if _, existing, err := store.Get(name); err == nil {
				prompt = existing
			}
		}

		t := req.toTemplate()
		if err := store.Update(name, t, prompt); err != nil {
			if strings.Contains(err.Error(), "not found") {
				httpError(w, err.Error(), http.StatusNotFound)
				return
			}
			httpInternalError(w, "update template", err)
			return
		}

		updated, prompt, err := store.Get(name)
		if err != nil {
			httpInternalError(w, "fetch updated template", err)
			return
		}
		writeJSON(w, http.StatusOK, templateResponse{Template: *updated, SystemPrompt: prompt})

	case http.MethodDelete:
		if err := store.Delete(name, ""); err != nil {
			if strings.Contains(err.Error(), "not found") {
				httpError(w, err.Error(), http.StatusNotFound)
				return
			}
			httpInternalError(w, "delete template", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
