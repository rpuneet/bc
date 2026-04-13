package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/bc/pkg/template"
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
type templateRequest struct { //nolint:govet // field order matches JSON/API contract
	ToolPolicies     *template.ToolPolicies `json:"tool_policies,omitempty"`
	MCPs             []string               `json:"mcps,omitempty"`
	Secrets          []string               `json:"secrets,omitempty"`
	Plugins          []string               `json:"plugins,omitempty"`
	ContextFiles     []string               `json:"context_files,omitempty"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description,omitempty"`
	SystemPrompt     string                 `json:"system_prompt,omitempty"`
	SystemPromptFile string                 `json:"system_prompt_file,omitempty"`
	MaxCostUSD       float64                `json:"max_cost_usd,omitempty"`
	StuckTimeoutMin  int                    `json:"stuck_timeout_min,omitempty"`
}

func (req *templateRequest) toTemplate() template.Template {
	return template.Template{
		Name:             req.Name,
		Description:      req.Description,
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
	switch r.Method {
	case http.MethodGet:
		templates, err := h.store.List()
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
		if err := h.store.Create(t, req.SystemPrompt); err != nil {
			// Distinguish conflict from internal error
			if strings.Contains(err.Error(), "already exists") {
				httpError(w, err.Error(), http.StatusConflict)
				return
			}
			httpInternalError(w, "create template", err)
			return
		}

		created, prompt, err := h.store.Get(req.Name)
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
	name := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	if name == "" {
		httpError(w, "template name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, prompt, err := h.store.Get(name)
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

		t := req.toTemplate()
		if err := h.store.Update(name, t, req.SystemPrompt); err != nil {
			if strings.Contains(err.Error(), "not found") {
				httpError(w, err.Error(), http.StatusNotFound)
				return
			}
			httpInternalError(w, "update template", err)
			return
		}

		updated, prompt, err := h.store.Get(name)
		if err != nil {
			httpInternalError(w, "fetch updated template", err)
			return
		}
		writeJSON(w, http.StatusOK, templateResponse{Template: *updated, SystemPrompt: prompt})

	case http.MethodDelete:
		if err := h.store.Delete(name); err != nil {
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
