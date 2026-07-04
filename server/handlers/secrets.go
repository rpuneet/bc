package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/secret"
)

// SecretHandler handles /api/secrets routes.
// Values are never returned — only metadata.
type SecretHandler struct {
	store *secret.Store
}

// NewSecretHandler creates a SecretHandler.
func NewSecretHandler(store *secret.Store) *SecretHandler {
	return &SecretHandler{store: store}
}

// Register mounts secret routes on mux.
func (h *SecretHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/secrets", h.list)
	mux.HandleFunc("/api/secrets/", h.byName)
}

func (h *SecretHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		secrets, err := store.List()
		if err != nil {
			httpInternalError(w, "list secrets", err)
			return
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(secrets) {
			secrets = []*secret.SecretMeta{}
		} else {
			secrets = secrets[offset:]
			if len(secrets) > limit {
				secrets = secrets[:limit]
			}
		}
		writeJSON(w, http.StatusOK, secrets)

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Value       string `json:"value"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := store.Set(req.Name, req.Value, req.Description); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := store.GetMeta(req.Name)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, meta)

	default:
		methodNotAllowed(w)
	}
}

func (h *SecretHandler) byName(w http.ResponseWriter, r *http.Request) {
	store := h.store
	name := strings.TrimPrefix(r.URL.Path, "/api/secrets/")
	if name == "" {
		httpError(w, "secret name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		meta, err := store.GetMeta(name)
		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		if meta == nil {
			httpError(w, "secret not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, meta)

	case http.MethodPut:
		var req struct {
			Value       string `json:"value"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := store.Set(name, req.Value, req.Description); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := store.GetMeta(name)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusOK, meta)

	case http.MethodDelete:
		if err := store.Delete(name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
