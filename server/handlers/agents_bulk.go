package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/rpuneet/bc/pkg/agent"
)

// bulkResult is the per-agent result of a bulk operation.
type bulkResult struct {
	Agent  string `json:"agent"`
	Status string `json:"status"` // "ok" | "error"
	Error  string `json:"error,omitempty"`
}

// bulkRequest is the common request shape for bulk start/stop/delete.
type bulkRequest struct {
	Agents []string `json:"agents"`
}

// bulkMessageRequest extends bulkRequest with a broadcast message.
type bulkMessageRequest struct {
	Message string   `json:"message"`
	Agents  []string `json:"agents"`
}

// registerBulkRoutes mounts /api/agents/bulk/* routes.
// Called from Register() in agents.go.
func (h *AgentHandler) registerBulkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agents/bulk/start", h.bulkStart)
	mux.HandleFunc("/api/agents/bulk/stop", h.bulkStop)
	mux.HandleFunc("/api/agents/bulk/delete", h.bulkDelete)
	mux.HandleFunc("/api/agents/bulk/message", h.bulkMessage)
}

// runBulk executes fn against each agent in parallel and collects results.
// Uses a bounded goroutine pool to avoid spawning hundreds at once.
func runBulk(ctx context.Context, agents []string, fn func(ctx context.Context, name string) error) []bulkResult {
	const maxConcurrent = 10
	results := make([]bulkResult, len(agents))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, name := range agents {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, n string) {
			defer wg.Done()
			defer func() { <-sem }()
			err := fn(ctx, n)
			if err != nil {
				results[idx] = bulkResult{Agent: n, Status: "error", Error: err.Error()}
			} else {
				results[idx] = bulkResult{Agent: n, Status: "ok"}
			}
		}(i, name)
	}
	wg.Wait()
	return results
}

// bulkStart starts many agents in parallel.
// POST /api/agents/bulk/start
func (h *AgentHandler) bulkStart(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Agents) == 0 {
		httpError(w, "agents array is required", http.StatusBadRequest)
		return
	}

	results := runBulk(r.Context(), req.Agents, func(ctx context.Context, name string) error {
		_, err := h.svc.Start(ctx, name, agent.StartOptions{})
		return err
	})
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// bulkStop stops many agents in parallel.
// POST /api/agents/bulk/stop
func (h *AgentHandler) bulkStop(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Agents) == 0 {
		httpError(w, "agents array is required", http.StatusBadRequest)
		return
	}

	results := runBulk(r.Context(), req.Agents, h.svc.Stop)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// bulkDelete deletes many agents in parallel.
// POST /api/agents/bulk/delete  {agents: [], force: bool}
func (h *AgentHandler) bulkDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Agents []string `json:"agents"`
		Force  bool     `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Agents) == 0 {
		httpError(w, "agents array is required", http.StatusBadRequest)
		return
	}

	force := req.Force
	results := runBulk(r.Context(), req.Agents, func(ctx context.Context, name string) error {
		return h.svc.Delete(ctx, name, force)
	})
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// bulkMessage sends a message to many agents in parallel.
// POST /api/agents/bulk/message  {agents: [], message: ""}
func (h *AgentHandler) bulkMessage(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req bulkMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Agents) == 0 || req.Message == "" {
		httpError(w, "agents and message are required", http.StatusBadRequest)
		return
	}

	msg := req.Message
	results := runBulk(r.Context(), req.Agents, func(ctx context.Context, name string) error {
		return h.svc.Send(ctx, name, msg)
	})
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
