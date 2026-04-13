package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/bc/pkg/agent"
	"github.com/rpuneet/bc/pkg/cost"
	"github.com/rpuneet/bc/pkg/events"
	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/stats"
	"github.com/rpuneet/bc/pkg/token"
	"github.com/rpuneet/bc/pkg/workspace"
	"github.com/rpuneet/bc/server/ws"
)

// HookEventRequest is the rich payload accepted by POST /api/agents/{name}/hook.
// All fields except Event are optional; callers may send any subset.
// The handler accepts the full agent.HookPayload; this struct documents the
// additional fields introduced in Phase 1c (TaskID, TaskTitle, Metadata).
type HookEventRequest struct {
	ToolInput any            `json:"tool_input,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	TaskTitle string         `json:"task_title,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Error     string         `json:"error,omitempty"`
	Event     string         `json:"event"`
}

// agentEventSub is a single SSE subscriber for per-agent hook events.
type agentEventSub struct {
	ch   chan []byte
	done <-chan struct{}
}

// agentEventBroker fans out hook events to per-agent SSE subscribers.
type agentEventBroker struct {
	subs map[string][]*agentEventSub
	mu   sync.RWMutex
}

func newAgentEventBroker() *agentEventBroker {
	return &agentEventBroker{subs: make(map[string][]*agentEventSub)}
}

func (b *agentEventBroker) subscribe(agentName string, done <-chan struct{}) *agentEventSub {
	sub := &agentEventSub{ch: make(chan []byte, 64), done: done}
	b.mu.Lock()
	b.subs[agentName] = append(b.subs[agentName], sub)
	b.mu.Unlock()
	return sub
}

func (b *agentEventBroker) unsubscribe(agentName string, sub *agentEventSub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[agentName]
	for i, s := range list {
		if s == sub {
			b.subs[agentName] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

func (b *agentEventBroker) publish(agentName string, msg []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs[agentName] {
		select {
		case sub.ch <- msg:
		default: // subscriber too slow — drop
		}
	}
}

// AgentHandler handles /api/agents routes.
type AgentHandler struct {
	svc        *agent.AgentService
	costs      *cost.Store
	ws         *workspace.Workspace
	hub        *ws.Hub
	events     events.EventStore
	terminal   *TerminalHandler
	statsStore *stats.Store
	broker     *agentEventBroker
}

// NewAgentHandler creates an AgentHandler.
// costs, ws, hub, and eventStore may be nil; enrichment fields will be omitted when unavailable.
func NewAgentHandler(svc *agent.AgentService, costs *cost.Store, ws *workspace.Workspace, hub *ws.Hub) *AgentHandler {
	return &AgentHandler{svc: svc, costs: costs, ws: ws, hub: hub, broker: newAgentEventBroker()}
}

// SetStatsStore sets the stats store for resource metrics enrichment.
func (h *AgentHandler) SetStatsStore(s *stats.Store) {
	h.statsStore = s
}

// SetEventStore sets the event store for persisting hook events.
func (h *AgentHandler) SetEventStore(es events.EventStore) {
	h.events = es
}

// SetTerminalHandler sets the terminal handler for WebSocket terminal access.
func (h *AgentHandler) SetTerminalHandler(th *TerminalHandler) {
	h.terminal = th
}

// Register mounts agent routes on mux.
// Exact-path routes must be registered before the prefix route "/api/agents/".
func (h *AgentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/agents/generate-name", h.generateName)
	mux.HandleFunc("/api/agents/broadcast", h.broadcast)
	mux.HandleFunc("/api/agents/send-role", h.sendRole)
	mux.HandleFunc("/api/agents/send-pattern", h.sendPattern)
	mux.HandleFunc("/api/agents/stop-all", h.stopAll)
	mux.HandleFunc("/api/agents/sync", h.syncSessions)
	mux.HandleFunc("/api/agents/health", h.health)
	// Bulk operations — must be registered before the catch-all below.
	h.registerBulkRoutes(mux)
	mux.HandleFunc("/api/agents", h.list)
	mux.HandleFunc("/api/agents/", h.byName)
}

type agentDTO struct { //nolint:govet // field order matches JSON/API contract
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    time.Time      `json:"started_at,omitempty"`
	UpdatedAt    time.Time      `json:"updated_at"`
	StoppedAt    *time.Time     `json:"stopped_at,omitempty"`
	Stats        *agentStatsDTO `json:"stats,omitempty"`
	Tool         string         `json:"tool,omitempty"`
	Session      string         `json:"session,omitempty"`
	State        string         `json:"state"`
	Task         string         `json:"task,omitempty"`
	Team         string         `json:"team,omitempty"`
	Name         string         `json:"name"`
	Runtime      string         `json:"runtime_backend,omitempty"`
	Role         string         `json:"role"`
	SessionID    string         `json:"session_id,omitempty"`
	ParentID     string         `json:"parent_id,omitempty"`
	ID           string         `json:"id,omitempty"`
	MCPServers   []string       `json:"mcp_servers,omitempty"`
	Children     []string       `json:"children,omitempty"`
	TotalCostUSD float64        `json:"total_cost_usd"`
	TotalTokens  int64          `json:"total_tokens"`
}

// agentStatsDTO holds resource metrics included when ?include=stats is set.
type agentStatsDTO struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemUsedBytes   int64   `json:"mem_used_bytes"`
	MemLimitBytes  int64   `json:"mem_limit_bytes"`
	MemPercent     float64 `json:"mem_percent"`
	NetRxBytes     int64   `json:"net_rx_bytes"`
	NetTxBytes     int64   `json:"net_tx_bytes"`
	DiskReadBytes  int64   `json:"disk_read_bytes"`
	DiskWriteBytes int64   `json:"disk_write_bytes"`
}

func toDTO(a *agent.Agent) agentDTO {
	return agentDTO{
		ID:        a.ID,
		Name:      a.Name,
		Role:      string(a.Role),
		State:     string(a.State),
		Task:      a.Task,
		Team:      a.Team,
		Tool:      a.Tool,
		Runtime:   a.RuntimeBackend,
		Session:   a.Session,
		SessionID: a.SessionID,
		ParentID:  a.ParentID,
		Children:  a.Children,
		CreatedAt: a.CreatedAt,
		StartedAt: a.StartedAt,
		UpdatedAt: a.UpdatedAt,
		StoppedAt: a.StoppedAt,
	}
}

// buildCostMap queries per-agent cost summaries and returns them keyed by agent ID.
func buildCostMap(ctx context.Context, store *cost.Store) map[string]*cost.Summary {
	summaries, err := store.SummaryByAgent(ctx)
	if err != nil {
		return nil
	}
	m := make(map[string]*cost.Summary, len(summaries))
	for _, s := range summaries {
		m[s.AgentID] = s
	}
	return m
}

func (h *AgentHandler) list(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// State is driven by hooks — no polling or reconciler needed.
		agents, err := h.svc.List(r.Context(), agent.ListOptions{})
		if err != nil {
			httpInternalError(w, "list agents", err)
			return
		}
		dtos := make([]agentDTO, 0, len(agents))
		for _, a := range agents {
			dtos = append(dtos, toDTO(a))
		}

		// Enrich with per-agent cost summaries.
		if h.costs != nil {
			costMap := buildCostMap(r.Context(), h.costs)
			for i := range dtos {
				if summary, ok := costMap[dtos[i].Name]; ok {
					dtos[i].TotalCostUSD = summary.TotalCostUSD
					dtos[i].TotalTokens = summary.TotalTokens
				}
			}
		}

		// Enrich with token usage from agent JSONL session files.
		if h.ws != nil {
			agentsDir := filepath.Join(h.ws.RootDir, ".bc", "agents")
			usages, tokenErr := token.CollectAll(agentsDir)
			if tokenErr == nil {
				// Sum per agent across models
				tokenMap := make(map[string]int64)
				for _, u := range usages {
					tokenMap[u.AgentName] += u.TotalTokens
				}
				for i := range dtos {
					if total, ok := tokenMap[dtos[i].Name]; ok && total > 0 {
						dtos[i].TotalTokens = total
					}
				}
			}
		}

		// Enrich with resource metrics when ?include=stats is set.
		if r.URL.Query().Get("include") == "stats" && h.statsStore != nil {
			latest, statsErr := h.statsStore.QueryLatestAgentMetrics(r.Context())
			if statsErr == nil {
				metricsMap := make(map[string]*stats.AgentMetric, len(latest))
				for i := range latest {
					metricsMap[latest[i].AgentName] = &latest[i]
				}
				for i := range dtos {
					if m, ok := metricsMap[dtos[i].Name]; ok {
						dtos[i].Stats = &agentStatsDTO{
							CPUPercent:     m.CPUPercent,
							MemUsedBytes:   m.MemUsedBytes,
							MemLimitBytes:  m.MemLimitBytes,
							MemPercent:     m.MemPercent,
							NetRxBytes:     m.NetRxBytes,
							NetTxBytes:     m.NetTxBytes,
							DiskReadBytes:  m.DiskReadBytes,
							DiskWriteBytes: m.DiskWriteBytes,
						}
					}
				}
			}
		}

		// Enrich with resolved MCP servers from the agent's role.
		if h.ws != nil && h.ws.RoleManager != nil {
			for i := range dtos {
				if dtos[i].Role != "" {
					resolved, resolveErr := h.ws.RoleManager.ResolveRole(dtos[i].Role)
					if resolveErr == nil && len(resolved.MCPServers) > 0 {
						dtos[i].MCPServers = resolved.MCPServers
					}
				}
			}
		}

		limit, offset := parsePagination(r, 50)
		if offset >= len(dtos) {
			dtos = []agentDTO{}
		} else {
			dtos = dtos[offset:]
			if len(dtos) > limit {
				dtos = dtos[:limit]
			}
		}
		writeJSON(w, http.StatusOK, dtos)

	case http.MethodPost:
		var req struct {
			Name    string `json:"name"`
			Role    string `json:"role"`
			Tool    string `json:"tool"`
			Runtime string `json:"runtime_backend"`
			Parent  string `json:"parent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		a, err := h.svc.Create(r.Context(), agent.CreateOptions{
			Name:    req.Name,
			Role:    agent.Role(req.Role),
			Tool:    req.Tool,
			Runtime: req.Runtime,
			Parent:  req.Parent,
		})
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, toDTO(a))

	default:
		methodNotAllowed(w)
	}
}

func (h *AgentHandler) byName(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/", 2)
	name := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if name == "" {
		httpError(w, "agent name required", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a, err := h.svc.Get(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, toDTO(a))

	case action == "activity":
		h.agentActivity(w, r, name)

	case r.Method == http.MethodPost && action == "start":
		var req struct {
			Runtime  string `json:"runtime"`
			ResumeID string `json:"resume_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck // body optional
		a, err := h.svc.Start(r.Context(), name, agent.StartOptions{
			Runtime:  req.Runtime,
			ResumeID: req.ResumeID,
		})
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, toDTO(a))

	case r.Method == http.MethodPost && action == "stop":
		if err := h.svc.Stop(r.Context(), name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})

	case r.Method == http.MethodPost && action == "send":
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := h.svc.Send(r.Context(), name, req.Message); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})

	case r.Method == http.MethodDelete && action == "":
		force := r.URL.Query().Get("force") == "true"
		if err := h.svc.Delete(r.Context(), name, force); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodPost && action == "hook":
		// Read raw body — stored as-is in event log for full observability.
		rawBody, readErr := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		if readErr != nil {
			httpError(w, "read error", http.StatusBadRequest)
			return
		}

		// Decode just enough to route state updates.
		var payload agent.HookPayload
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !agent.IsKnownEvent(payload.Event) {
			httpError(w, "unknown event: "+string(payload.Event), http.StatusBadRequest)
			return
		}

		// Determine target state: explicit in payload > mapped from event > no change
		task := payload.Task
		targetState, hasState := agent.StateForHookEvent(payload.Event)
		if payload.State != "" {
			if agent.IsValidState(payload.State) {
				targetState = agent.State(payload.State)
				hasState = true
			}
		}

		if hasState {
			if err := h.svc.Manager().UpdateAgentState(name, targetState, task); err != nil {
				log.Debug("hook state update skipped", "agent", name, "error", err)
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": true, "reason": err.Error()})
				return
			}
		}

		// Build structured data map from rich payload fields for the event store.
		eventData := map[string]any{
			"event": string(payload.Event),
		}
		if payload.ToolName != "" {
			eventData["tool_name"] = payload.ToolName
		}
		if payload.ToolInput != nil {
			eventData["tool_input"] = payload.ToolInput
		}
		if payload.Error != "" {
			eventData["error"] = payload.Error
		}
		if payload.Task != "" {
			eventData["task"] = payload.Task
		}
		if payload.TaskID != "" {
			eventData["task_id"] = payload.TaskID
		}
		if payload.TaskTitle != "" {
			eventData["task_title"] = payload.TaskTitle
		}
		if payload.Message != "" {
			eventData["message"] = payload.Message
		}
		if len(payload.Metadata) > 0 {
			eventData["metadata"] = payload.Metadata
		}

		// Persist raw JSON body to event log — raw body in Message for full
		// observability; structured fields in Data for typed queries.
		now := time.Now()
		if h.events != nil {
			_ = h.events.Append(events.Event{ //nolint:errcheck // best-effort logging
				Timestamp: now,
				Type:      events.EventType("hook." + string(payload.Event)),
				Agent:     name,
				Message:   string(rawBody),
				Data:      eventData,
			})
		}

		// Build the SSE data payload for per-agent subscribers.
		ssePayload := map[string]any{
			"event":     string(payload.Event),
			"timestamp": now.UTC().Format(time.RFC3339Nano),
			"agent":     name,
		}
		if payload.ToolName != "" {
			ssePayload["tool_name"] = payload.ToolName
		}
		if payload.ToolInput != nil {
			ssePayload["tool_input"] = payload.ToolInput
		}
		if payload.Error != "" {
			ssePayload["error"] = payload.Error
		}
		if payload.Task != "" {
			ssePayload["task"] = payload.Task
		}
		if payload.TaskID != "" {
			ssePayload["task_id"] = payload.TaskID
		}
		if payload.TaskTitle != "" {
			ssePayload["task_title"] = payload.TaskTitle
		}
		if len(payload.Metadata) > 0 {
			ssePayload["metadata"] = payload.Metadata
		}

		// Publish to per-agent SSE subscribers (GET /api/agents/{name}/events).
		if sseMsg, encErr := buildHookSSEMessage(now, ssePayload); encErr == nil {
			h.broker.publish(name, sseMsg)
		}

		// Publish raw hook JSON via SSE for web UI — same format as event log.
		if h.hub != nil {
			var raw map[string]any
			if err := json.Unmarshal(rawBody, &raw); err == nil {
				raw["agent"] = name
				h.hub.Publish("agent.hook", raw)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case r.Method == http.MethodGet && action == "events":
		h.streamHookEvents(w, r, name)

	case r.Method == http.MethodGet && action == "stats":
		// Return recent Docker stats samples for this agent.
		limit := 20
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
				limit = n
			}
		}
		limit = clampInt(limit, 1, 1000)
		records, err := h.svc.Manager().QueryAgentStats(name, limit)
		if err != nil {
			httpInternalError(w, "stats unavailable", err)
			return
		}
		if records == nil {
			records = []*agent.AgentStatsRecord{}
		}
		writeJSON(w, http.StatusOK, records)

	case r.Method == http.MethodPost && action == "rename":
		var req struct {
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := h.svc.Rename(r.Context(), name, req.NewName); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "renamed", "name": req.NewName})

	case r.Method == http.MethodGet && action == "peek":
		lines := 500
		if lStr := r.URL.Query().Get("lines"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
				lines = n
			}
		}
		lines = clampInt(lines, 1, 10000)
		output, err := h.svc.Peek(r.Context(), name, lines)
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": output})

	case r.Method == http.MethodGet && action == "last-terminal":
		h.lastTerminal(w, r, name)

	case r.Method == http.MethodGet && action == "sessions":
		sessions, err := h.svc.Sessions(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if sessions == nil {
			sessions = []agent.SessionEntry{}
		}
		writeJSON(w, http.StatusOK, sessions)

	case r.Method == http.MethodPost && action == "report":
		var req struct {
			State   string `json:"state"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !agent.IsValidState(req.State) {
			httpError(w, fmt.Sprintf("invalid agent state: %q", req.State), http.StatusBadRequest)
			return
		}
		state := agent.State(req.State)
		if err := h.svc.Manager().UpdateAgentState(name, state, req.Message); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reported"})

	case r.Method == http.MethodGet && action == "output":
		h.streamOutput(w, r, name)

	case r.Method == http.MethodGet && action == "terminal":
		if h.terminal == nil {
			httpError(w, "terminal not available", http.StatusNotImplemented)
			return
		}
		h.terminal.HandleTerminal(w, r, name)

	case r.Method == http.MethodGet && action == "config":
		h.getAgentConfig(w, r, name)

	case r.Method == http.MethodPatch && action == "config":
		h.patchAgentConfig(w, r, name)

	case action == "fork":
		h.forkAgent(w, r, name)

	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

func (h *AgentHandler) generateName(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	name, err := h.svc.GenerateName(r.Context())
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (h *AgentHandler) broadcast(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	sent, err := h.svc.Broadcast(r.Context(), req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": sent})
}

func (h *AgentHandler) sendRole(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Role    string `json:"role"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := h.svc.SendToRole(r.Context(), req.Role, req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) sendPattern(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Pattern string `json:"pattern"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := h.svc.SendToPattern(r.Context(), req.Pattern, req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) stopAll(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	stopped, err := h.svc.StopAll(r.Context())
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"stopped": stopped})
}

// syncSessions reconciles in-memory agent state with actual runtime sessions.
// POST /api/agents/sync
func (h *AgentHandler) syncSessions(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	synced, stopped := h.svc.SyncSessions(r.Context())
	writeJSON(w, http.StatusOK, map[string]int{"synced": synced, "stopped": stopped})
}

// AgentHealthInfo represents health status of an agent.
type AgentHealthInfo struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	LastUpdated   string `json:"last_updated"`
	StaleDuration string `json:"stale_duration,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	TmuxAlive     bool   `json:"tmux_alive"`
	StateFresh    bool   `json:"state_fresh"`
}

func (h *AgentHandler) health(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	timeoutStr := r.URL.Query().Get("timeout")
	timeout := 60 * time.Second
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	agents, err := h.svc.List(r.Context(), agent.ListOptions{})
	if err != nil {
		httpInternalError(w, "list agents", err)
		return
	}

	// Optionally filter to a single agent.
	nameFilter := r.URL.Query().Get("agent")

	mgr := h.svc.Manager()
	results := make([]AgentHealthInfo, 0, len(agents))
	for _, a := range agents {
		if nameFilter != "" && a.Name != nameFilter {
			continue
		}
		health := AgentHealthInfo{
			Name:        a.Name,
			Role:        string(a.Role),
			LastUpdated: a.UpdatedAt.Format(time.RFC3339),
		}
		health.TmuxAlive = mgr.RuntimeForAgent(a.Name).HasSession(r.Context(), a.Name)

		staleDuration := time.Since(a.UpdatedAt)
		health.StateFresh = staleDuration < timeout
		if !health.StateFresh {
			health.StaleDuration = staleDuration.Round(time.Second).String()
		}

		switch {
		case a.State == agent.StateStopped:
			health.Status = "unhealthy"
			health.ErrorMessage = "agent stopped"
		case a.State == agent.StateError:
			health.Status = "unhealthy"
			health.ErrorMessage = "agent in error state"
		case !health.TmuxAlive:
			health.Status = "unhealthy"
			health.ErrorMessage = "tmux session not found"
		case !health.StateFresh:
			health.Status = "degraded"
			health.ErrorMessage = fmt.Sprintf("state stale (%s since last update)", health.StaleDuration)
		default:
			health.Status = "healthy"
		}

		results = append(results, health)
	}

	writeJSON(w, http.StatusOK, results)
}

// buildHookSSEMessage formats a single hook event as an SSE frame:
//
//	id: <timestamp_ms>
//	event: hook
//	data: <json>
func buildHookSSEMessage(ts time.Time, payload map[string]any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := ts.UnixMilli()
	msg := fmt.Sprintf("id: %d\nevent: hook\ndata: %s\n\n", id, data)
	return []byte(msg), nil
}

// streamHookEvents serves GET /api/agents/{name}/events as an SSE stream.
// It replays the last 50 hook events from the event store, then tails new
// events as they arrive via the per-agent broker.
func (h *AgentHandler) streamHookEvents(w http.ResponseWriter, r *http.Request, name string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Verify agent exists.
	if _, err := h.svc.Get(r.Context(), name); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe before replaying history so we don't miss events that arrive
	// between the replay query and the subscription being established.
	sub := h.broker.subscribe(name, r.Context().Done())
	defer h.broker.unsubscribe(name, sub)

	// Replay last 50 hook events from the event store.
	if h.events != nil {
		past, err := h.events.ReadByAgent(name)
		if err == nil {
			// Filter to hook.* events only; take the last 50.
			const replayLimit = 50
			var hookEvts []events.Event
			for i := range past {
				if strings.HasPrefix(string(past[i].Type), "hook.") {
					hookEvts = append(hookEvts, past[i])
				}
			}
			if len(hookEvts) > replayLimit {
				hookEvts = hookEvts[len(hookEvts)-replayLimit:]
			}
			for _, ev := range hookEvts {
				payload := map[string]any{
					"event":     strings.TrimPrefix(string(ev.Type), "hook."),
					"timestamp": ev.Timestamp.UTC().Format(time.RFC3339Nano),
					"agent":     name,
				}
				// Merge structured data fields into payload.
				for k, v := range ev.Data {
					if k != "event" { // avoid duplicate
						payload[k] = v
					}
				}
				if msg, encErr := buildHookSSEMessage(ev.Timestamp, payload); encErr == nil {
					_, _ = w.Write(msg) //nolint:errcheck
				}
			}
			flusher.Flush()
		}
	}

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-sub.ch:
			if !ok {
				return
			}
			_, _ = w.Write(msg) //nolint:errcheck
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n") //nolint:errcheck // SSE comment
			flusher.Flush()
		}
	}
}

// lastTerminal serves GET /api/agents/{name}/last-terminal.
// It returns the last captured terminal output for an agent, which is useful
// for inspecting the final state of stopped agents.
func (h *AgentHandler) lastTerminal(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	lines := 500
	if lStr := r.URL.Query().Get("lines"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			lines = n
		}
	}
	lines = clampInt(lines, 1, 10000)

	output, err := h.svc.Peek(r.Context(), name, lines)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	a, agentErr := h.svc.Get(r.Context(), name)
	resp := map[string]any{
		"output": output,
		"agent":  name,
	}
	if agentErr == nil {
		resp["state"] = string(a.State)
		if a.StoppedAt != nil {
			resp["stopped_at"] = a.StoppedAt.UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// streamOutput streams agent terminal output as SSE events.
// Polls capture-pane every second and sends new lines as events.
func (h *AgentHandler) streamOutput(w http.ResponseWriter, r *http.Request, name string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Verify agent exists
	if _, err := h.svc.Get(r.Context(), name); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial snapshot
	output, err := h.svc.Peek(r.Context(), name, 50)
	if err == nil && output != "" {
		data, _ := json.Marshal(map[string]string{"output": output})
		fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
		flusher.Flush()
	}

	// Poll for new output every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastLen int
	if output != "" {
		lastLen = len(output)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			current, peekErr := h.svc.Peek(r.Context(), name, 200)
			if peekErr != nil {
				continue
			}
			if len(current) > lastLen {
				// Send only the new portion
				newOutput := current[lastLen:]
				data, _ := json.Marshal(map[string]string{"output": newOutput})
				fmt.Fprintf(w, "event: agent.output\ndata: %s\n\n", data) //nolint:errcheck
				flusher.Flush()
				lastLen = len(current)
			}
		}
	}
}
