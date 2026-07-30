package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/home"
)

// RolesHandler handles /api/roles routes.
type RolesHandler struct {
	h *home.Home
}

// NewRolesHandler creates a RolesHandler.
func NewRolesHandler(h *home.Home) *RolesHandler {
	return &RolesHandler{h: h}
}

// Register mounts roles routes on mux.
func (h *RolesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/roles", h.list)
	mux.HandleFunc("/api/roles/", h.byName)
}

// roleRequest is the JSON body for creating/updating a role.
type roleRequest struct {
	Rules        map[string]string `json:"rules"`
	Commands     map[string]string `json:"commands"`
	Skills       map[string]string `json:"skills"`
	Agents       map[string]string `json:"agents"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Prompt       string            `json:"prompt"`
	PromptStart  string            `json:"prompt_start"`
	PromptStop   string            `json:"prompt_stop"`
	PromptCreate string            `json:"prompt_create"`
	PromptDelete string            `json:"prompt_delete"`
	Review       string            `json:"review"`
	ParentRoles  []string          `json:"parent_roles"`
	MCPServers   []string          `json:"mcp_servers"`
	Secrets      []string          `json:"secrets"`
	Plugins      []string          `json:"plugins"`
	CLITools     []string          `json:"cli_tools"`
}

func (req *roleRequest) toRole() *home.Role {
	return &home.Role{
		Prompt: req.Prompt,
		Metadata: home.RoleMetadata{
			Name:         req.Name,
			Description:  req.Description,
			ParentRoles:  req.ParentRoles,
			MCPServers:   req.MCPServers,
			Secrets:      req.Secrets,
			Plugins:      req.Plugins,
			Rules:        req.Rules,
			Commands:     req.Commands,
			Skills:       req.Skills,
			Agents:       req.Agents,
			PromptStart:  req.PromptStart,
			PromptStop:   req.PromptStop,
			PromptCreate: req.PromptCreate,
			PromptDelete: req.PromptDelete,
			Review:       req.Review,
			CLITools:     req.CLITools,
		},
	}
}

// list handles GET /api/roles and POST /api/roles.
func (h *RolesHandler) list(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roles, err := h.h.RoleManager.LoadAllRoles()
		if err != nil {
			httpInternalError(w, "list roles", err)
			return
		}

		// Deduplicate roles by normalized name to prevent duplicates
		// like "product_manager" vs "product-manager".
		resolved := make(map[string]*home.ResolvedRole, len(roles))
		for name := range roles {
			normalized := home.NormalizeRoleName(name)
			if _, exists := resolved[normalized]; exists {
				continue // already have this role under normalized name
			}
			if res, resolveErr := h.h.RoleManager.ResolveRole(name); resolveErr == nil {
				resolved[normalized] = res
			}
		}
		writeJSON(w, http.StatusOK, resolved)

	case http.MethodPost:
		var req roleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			httpError(w, "role name is required", http.StatusBadRequest)
			return
		}
		if h.h.RoleManager.HasRole(req.Name) {
			httpError(w, "role already exists: "+req.Name, http.StatusConflict)
			return
		}

		role := req.toRole()
		if err := h.h.RoleManager.WriteRole(role); err != nil {
			httpInternalError(w, "create role", err)
			return
		}

		resolved, err := h.h.RoleManager.ResolveRole(req.Name)
		if err != nil {
			httpInternalError(w, "resolve role", err)
			return
		}
		writeJSON(w, http.StatusCreated, resolved)

	default:
		methodNotAllowed(w)
	}
}

// byName handles GET/PUT/DELETE /api/roles/{name}.
func (h *RolesHandler) byName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/roles/")
	if name == "" {
		httpError(w, "role name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		resolved, err := h.h.RoleManager.ResolveRole(name)
		if err != nil {
			httpError(w, "role not found: "+err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, resolved)

	case http.MethodPut:
		var req roleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// Use the URL name, ignore body name
		req.Name = name

		role := req.toRole()
		if err := h.h.RoleManager.WriteRole(role); err != nil {
			httpInternalError(w, "update role", err)
			return
		}

		resolved, err := h.h.RoleManager.ResolveRole(name)
		if err != nil {
			httpInternalError(w, "resolve role", err)
			return
		}
		writeJSON(w, http.StatusOK, resolved)

	case http.MethodDelete:
		if err := h.h.RoleManager.DeleteRole(name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
