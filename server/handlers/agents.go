package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/token"
	"github.com/rpuneet/mycel/server/ws"
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
	costs      *cost.Service
	home       *home.Home
	hub        *ws.Hub
	events     events.EventStore
	terminal   *TerminalHandler
	statsStore *stats.Store
	tmplStore  *template.Store
	broker     *agentEventBroker
}

// NewAgentHandler creates an AgentHandler.
// costs, home, hub, and eventStore may be nil; enrichment fields will be omitted when unavailable.
func NewAgentHandler(svc *agent.AgentService, costs *cost.Service, home *home.Home, hub *ws.Hub) *AgentHandler {
	h := &AgentHandler{svc: svc, costs: costs, home: home, hub: hub, broker: newAgentEventBroker()}
	if svc != nil {
		// The per-agent SSE broker stays handler-owned; the service invokes
		// this callback after each successfully ingested hook event so
		// GET /api/agents/{name}/events subscribers receive it.
		svc.SetOnHookEvent(func(agentName string, ts time.Time, payload map[string]any) {
			if msg, err := buildHookSSEMessage(ts, payload); err == nil {
				h.broker.publish(agentName, msg)
			}
		})
	}
	return h
}

// SetTemplateStore sets the template store used during agent creation.
func (h *AgentHandler) SetTemplateStore(store *template.Store) {
	h.tmplStore = store
}

// SetStatsStore sets the stats store for resource metrics enrichment.
func (h *AgentHandler) SetStatsStore(s *stats.Store) {
	h.statsStore = s
}

// SetEventStore sets the event store for persisting hook events.
// It is kept on the handler for SSE replay (streamHookEvents) and forwarded
// to the agent service, which owns hook-event persistence (IngestHookEvent).
func (h *AgentHandler) SetEventStore(es events.EventStore) {
	h.events = es
	if h.svc != nil {
		h.svc.SetHookEventStore(es)
	}
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
	mux.HandleFunc("/api/agents/activity", h.activity)
	// Bulk operations — must be registered before the catch-all below.
	h.registerBulkRoutes(mux)
	mux.HandleFunc("/api/agents", h.list)
	mux.HandleFunc("/api/agents/", h.byName)
}

// avatarDTO holds agent avatar configuration.
type avatarDTO struct {
	Variant string `json:"variant,omitempty"`
	Color   string `json:"color,omitempty"`
}

type agentDTO struct { //nolint:govet // field order matches JSON/API contract
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
	StoppedAt  *time.Time     `json:"stopped_at,omitempty"`
	ArchivedAt *time.Time     `json:"archived_at,omitempty"`
	Stats      *agentStatsDTO `json:"stats,omitempty"`
	Avatar     *avatarDTO     `json:"avatar,omitempty"`
	// Env holds the agent's configured environment variables. Values
	// with ${secret:NAME} references are returned as the reference —
	// resolved values never leave the daemon.
	Env          map[string]string `json:"env,omitempty"`
	Tool         string            `json:"tool,omitempty"`
	Model        string            `json:"model,omitempty"`
	Session      string            `json:"session,omitempty"`
	State        string            `json:"state"`
	Task         string            `json:"task,omitempty"`
	Team         string            `json:"team,omitempty"`
	Name         string            `json:"name"`
	Runtime      string            `json:"runtime_backend,omitempty"`
	Role         string            `json:"role"`
	Template     string            `json:"template,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	ParentID     string            `json:"parent_id,omitempty"`
	ID           string            `json:"id,omitempty"`
	Repo         string            `json:"repo,omitempty"`
	MCPServers   []string          `json:"mcp_servers,omitempty"`
	Children     []string          `json:"children,omitempty"`
	TotalCostUSD float64           `json:"total_cost_usd"`
	TotalTokens  int64             `json:"total_tokens"`
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
		ID:         a.ID,
		Name:       a.Name,
		Role:       string(a.Role),
		State:      string(a.State),
		Task:       a.Task,
		Team:       a.Team,
		Tool:       a.Tool,
		Model:      a.Model,
		Runtime:    a.RuntimeBackend,
		Session:    a.Session,
		SessionID:  a.SessionID,
		ParentID:   a.ParentID,
		Children:   a.Children,
		CreatedAt:  a.CreatedAt,
		StartedAt:  a.StartedAt,
		UpdatedAt:  a.UpdatedAt,
		StoppedAt:  a.StoppedAt,
		ArchivedAt: a.ArchivedAt,
		Repo:       a.Repo,
		Env:        a.Env,
	}
}

// buildCostMap queries per-agent cost summaries and returns them keyed by agent ID.
func buildCostMap(ctx context.Context, store *cost.Service) map[string]*cost.Summary {
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

// costForAgent aggregates ledger summaries for an agent. The ledger keys
// agent_id by the worktree/session name the transcripts were imported
// under: bare `<name>` (flat layout) or the legacy `bc-<repoBase>-<name>`.
// Both candidates are derived exactly from the agent's own name and repo —
// no suffix scanning, so `web` can never absorb `other-web`'s costs.
func costForAgent(costMap map[string]*cost.Summary, name, repo string) (costUSD float64, tokens int64, found bool) {
	if name == "" {
		return 0, 0, false
	}
	candidates := []string{name}
	if repo != "" {
		candidates = append(candidates, "bc-"+filepath.Base(repo)+"-"+name)
	}
	for _, id := range candidates {
		if s, ok := costMap[id]; ok {
			costUSD += s.TotalCostUSD
			tokens += s.TotalTokens
			found = true
		}
	}
	return costUSD, tokens, found
}

func (h *AgentHandler) list(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	costs := h.costs
	homeRef := h.home
	switch r.Method {
	case http.MethodGet:
		// State is driven by hooks — no polling or reconciler needed.
		q := r.URL.Query()
		agents, err := svc.List(r.Context(), agent.ListOptions{
			IncludeArchived: q.Get("includeArchived") == "1" || q.Get("includeArchived") == "true",
			OnlyArchived:    q.Get("onlyArchived") == "1" || q.Get("onlyArchived") == "true",
		})
		if err != nil {
			httpInternalError(w, "list agents", err)
			return
		}
		dtos := make([]agentDTO, 0, len(agents))
		for _, a := range agents {
			dtos = append(dtos, toDTO(a))
		}

		// Enrich with per-agent cost summaries.
		if costs != nil {
			costMap := buildCostMap(r.Context(), costs)
			for i := range dtos {
				if costUSD, tokens, ok := costForAgent(costMap, dtos[i].Name, dtos[i].Repo); ok {
					dtos[i].TotalCostUSD = costUSD
					dtos[i].TotalTokens = tokens
				}
			}
		}

		// Enrich with token usage from agent JSONL session files.
		if homeRef != nil {
			agentsDir := filepath.Join(homeRef.RootDir, ".bc", "agents")
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
		if homeRef != nil && homeRef.RoleManager != nil {
			for i := range dtos {
				if dtos[i].Role != "" {
					resolved, resolveErr := homeRef.RoleManager.ResolveRole(dtos[i].Role)
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
			Avatar *avatarDTO `json:"avatar,omitempty"`
			// Env holds user-configured environment variables. Values may
			// contain ${secret:NAME} references, stored verbatim and
			// resolved against the vault at spawn time.
			Env      map[string]string `json:"env,omitempty"`
			Name     string            `json:"name"`
			Role     string            `json:"role"`
			Tool     string            `json:"tool"`
			Model    string            `json:"model,omitempty"`
			Runtime  string            `json:"runtime_backend"`
			Parent   string            `json:"parent"`
			Template string            `json:"template,omitempty"`
			// Repo is the absolute path of the git repo the agent binds
			// to. Empty defaults to the repo bcd was booted against.
			Repo string `json:"repo,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		for k := range req.Env {
			if !agent.IsValidEnvName(k) {
				httpError(w, fmt.Sprintf("invalid env var name %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", k), http.StatusBadRequest)
				return
			}
		}
		// Default role to "base" when template is provided without explicit role.
		role := req.Role
		if role == "" && req.Template != "" {
			role = "base"
		}
		a, err := svc.Create(r.Context(), agent.CreateOptions{
			Name:    req.Name,
			Role:    agent.Role(role),
			Tool:    req.Tool,
			Model:   req.Model,
			Runtime: req.Runtime,
			Parent:  req.Parent,
			Repo:    req.Repo,
			Env:     req.Env,
		})
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		dto := toDTO(a)
		dto.Avatar = req.Avatar
		dto.Template = req.Template
		// Apply template: write CLAUDE.md and .mcp.json to the agent's worktree.
		if req.Template != "" && h.tmplStore != nil {
			if applyErr := h.applyTemplate(svc, a, req.Template, req.Avatar); applyErr != nil {
				log.Warn("template apply failed", "agent", a.Name, "template", req.Template, "error", applyErr)
			}
		}
		writeJSON(w, http.StatusCreated, dto)

	default:
		methodNotAllowed(w)
	}
}

// agentHTTPStatus maps domain errors from pkg/agent to the appropriate HTTP
// status code.  Callers use this instead of hardcoding 400/404.
func agentHTTPStatus(err error) int {
	switch {
	case errors.Is(err, agent.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, agent.ErrAlreadyRunning):
		return http.StatusConflict
	case errors.Is(err, agent.ErrNotRunning):
		return http.StatusConflict
	case errors.Is(err, agent.ErrInvalidState):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (h *AgentHandler) byName(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
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
	// Agent names are used to build filesystem paths (worktrees, env and
	// loop config, logs) — reject anything that is not a valid agent name
	// before it reaches any path construction.
	if !agent.IsValidAgentName(name) {
		httpError(w, "invalid agent name", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a, err := svc.Get(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
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
		a, err := svc.Start(r.Context(), name, agent.StartOptions{
			Runtime:  req.Runtime,
			ResumeID: req.ResumeID,
		})
		if err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, toDTO(a))

	case r.Method == http.MethodPost && action == "stop":
		if err := svc.Stop(r.Context(), name); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})

	case r.Method == http.MethodPost && action == "archive":
		if err := svc.Archive(r.Context(), name); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})

	case r.Method == http.MethodPost && action == "unarchive":
		if err := svc.Unarchive(r.Context(), name); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "active"})

	case r.Method == http.MethodPost && action == "send":
		var req struct {
			Message string `json:"message"`
		}
		// Strict decode: agents were silently no-op'ing DMs to each other
		// because the /send endpoint accepted the gateway body shape
		// ({sender, content}) and typed an empty string into the target's
		// session (#3174). Reject unknown fields and empty messages so the
		// caller gets a clear 400 instead of a false success.
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			httpError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			httpError(w, "message must not be empty", http.StatusBadRequest)
			return
		}
		if err := svc.Send(r.Context(), name, req.Message); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})

	case r.Method == http.MethodDelete && action == "":
		force := r.URL.Query().Get("force") == "true"
		if err := svc.Delete(r.Context(), name, force); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
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

		// The transport-independent core (state transition, event
		// persistence, broadcast) lives in the agent service so HTTP
		// hooks and future transcript tailers share one ingest path.
		if err := svc.IngestHookEvent(r.Context(), name, payload, rawBody); err != nil {
			var skipped *agent.HookStateSkippedError
			if errors.As(err, &skipped) {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": true, "reason": skipped.Err.Error()})
				return
			}
			httpError(w, err.Error(), http.StatusBadRequest)
			return
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
		records, err := svc.Manager().QueryAgentStats(name, limit)
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
		if err := svc.Rename(r.Context(), name, req.NewName); err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
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
		output, err := svc.Peek(r.Context(), name, lines)
		if err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": output})

	case r.Method == http.MethodGet && action == "last-terminal":
		h.lastTerminal(w, r, name)

	case r.Method == http.MethodGet && action == "sessions":
		sessions, err := svc.Sessions(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), agentHTTPStatus(err))
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
		if err := svc.Manager().UpdateAgentState(r.Context(), name, state, req.Message); err != nil {
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

	case action == "mcps" || strings.HasPrefix(action, "mcps/"):
		h.agentMCPs(w, r, name, action)

	case r.Method == http.MethodGet && action == "stats-computed":
		h.agentComputedStats(w, r, name)

	case r.Method == http.MethodGet && action == "env":
		h.getAgentEnv(w, r, name)

	case r.Method == http.MethodPut && action == "env":
		h.putAgentEnv(w, r, name)

	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

func (h *AgentHandler) generateName(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	name, err := svc.GenerateName(r.Context())
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (h *AgentHandler) broadcast(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
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
	sent, err := svc.Broadcast(r.Context(), req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": sent})
}

func (h *AgentHandler) sendRole(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
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
	result, err := svc.SendToRole(r.Context(), req.Role, req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) sendPattern(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
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
	result, err := svc.SendToPattern(r.Context(), req.Pattern, req.Message)
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) stopAll(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	stopped, err := svc.StopAll(r.Context())
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"stopped": stopped})
}

// syncSessions reconciles in-memory agent state with actual runtime sessions.
// POST /api/agents/sync
func (h *AgentHandler) syncSessions(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	synced, stopped := svc.SyncSessions(r.Context())
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
	svc := h.svc
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

	agents, err := svc.List(r.Context(), agent.ListOptions{})
	if err != nil {
		httpInternalError(w, "list agents", err)
		return
	}

	// Optionally filter to a single agent.
	nameFilter := r.URL.Query().Get("agent")

	mgr := svc.Manager()
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
	svc := h.svc
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Verify agent exists.
	if _, err := svc.Get(r.Context(), name); err != nil {
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
	svc := h.svc
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

	output, err := svc.Peek(r.Context(), name, lines)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	a, agentErr := svc.Get(r.Context(), name)
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
	svc := h.svc
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Verify agent exists
	if _, err := svc.Get(r.Context(), name); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial snapshot
	output, err := svc.Peek(r.Context(), name, 50)
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
			current, peekErr := svc.Peek(r.Context(), name, 200)
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

// applyTemplate writes template files (CLAUDE.md and .mcp.json) to the agent's
// worktree. Called after successful agent creation when a template name is provided.
// svc is the per-request resolved agent service used for worktree-path fallback.
func (h *AgentHandler) applyTemplate(svc *agent.AgentService, a *agent.Agent, tmplName string, _ *avatarDTO) error {
	tmpl, prompt, err := h.tmplStore.Get(tmplName)
	if err != nil {
		return fmt.Errorf("load template %q: %w", tmplName, err)
	}

	// Determine worktree path: prefer stored WorktreeDir, fall back to computed.
	wtDir := a.WorktreeDir
	if wtDir == "" && svc != nil {
		wtDir = svc.Manager().WorktreePath(a.Name)
	}
	if wtDir == "" {
		return fmt.Errorf("worktree path not available for agent %q", a.Name)
	}
	wtDir = filepath.Clean(wtDir)
	if strings.Contains(wtDir, "..") {
		return fmt.Errorf("unsafe worktree path for agent %q", a.Name)
	}

	if err := os.MkdirAll(wtDir, 0750); err != nil {
		return fmt.Errorf("ensure worktree dir: %w", err)
	}

	// Write system prompt as CLAUDE.md when the template has one.
	if prompt != "" {
		claudePath := filepath.Join(wtDir, "CLAUDE.md")
		if writeErr := os.WriteFile(claudePath, []byte(prompt), 0600); writeErr != nil { //nolint:gosec // trusted repo path
			return fmt.Errorf("write CLAUDE.md: %w", writeErr)
		}
		log.Debug("template CLAUDE.md written", "agent", a.Name, "template", tmplName)
	}

	// Merge template's MCPs into any existing .mcp.json.
	// The previous behavior emitted empty {url:"",type:""} stubs that
	// clobbered the role-generated config.  Instead we:
	//   1. Read the existing file (if any) to preserve real entries.
	//   2. Insert stub entries only for names that are not already present.
	// This way the role-generated MCP config is never wiped.
	if len(tmpl.MCPs) > 0 {
		mcpPath := filepath.Join(wtDir, ".mcp.json")

		// Read existing config; ignore missing file.
		existing := agentMCPFile{MCPServers: make(map[string]agentMCPEntry)}
		if raw, readErr := os.ReadFile(mcpPath); readErr == nil { //nolint:gosec // trusted repo path
			// Best-effort parse; ignore corrupt files.
			_ = json.Unmarshal(raw, &existing)
			if existing.MCPServers == nil {
				existing.MCPServers = make(map[string]agentMCPEntry)
			}
		}

		// Add stub entries only for names not already configured.
		added := 0
		for _, mcpName := range tmpl.MCPs {
			if _, ok := existing.MCPServers[mcpName]; !ok {
				existing.MCPServers[mcpName] = agentMCPEntry{}
				added++
			}
		}

		b, marshalErr := json.MarshalIndent(existing, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal .mcp.json: %w", marshalErr)
		}
		if writeErr := os.WriteFile(mcpPath, b, 0600); writeErr != nil { //nolint:gosec // trusted repo path
			return fmt.Errorf("write .mcp.json: %w", writeErr)
		}
		log.Debug("template .mcp.json merged", "agent", a.Name, "template", tmplName, "added", added, "total", len(existing.MCPServers))
	}

	return nil
}

// ── MCP management endpoints ─────────────────────────────────────────────────

// mcpServerDTO is the JSON representation of a single MCP server entry.
type mcpServerDTO struct { //nolint:govet // field order matches JSON/API contract
	Env     map[string]string `json:"env,omitempty"`
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Type    string            `json:"type,omitempty"`
}

// agentMCPFile is the on-disk representation of .mcp.json.
type agentMCPFile struct {
	MCPServers map[string]agentMCPEntry `json:"mcpServers"`
}

type agentMCPEntry struct {
	Env     map[string]string `json:"env,omitempty"`
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
	Args    []string          `json:"args,omitempty"`
}

// agentMCPs dispatches GET/POST /api/agents/{name}/mcps and
// DELETE /api/agents/{name}/mcps/{mcp}.
func (h *AgentHandler) agentMCPs(w http.ResponseWriter, r *http.Request, agentName, action string) {
	svc := h.svc
	// action is either "mcps" or "mcps/<mcp-server-name>"
	mcpName := strings.TrimPrefix(action, "mcps/")
	if mcpName == "mcps" {
		mcpName = "" // bare "mcps" — no sub-resource
	}

	a, err := svc.Get(r.Context(), agentName)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	wtDir := a.WorktreeDir
	if wtDir == "" {
		wtDir = svc.Manager().WorktreePath(agentName)
	}
	wtDir = filepath.Clean(wtDir)
	if strings.Contains(wtDir, "..") {
		httpError(w, "unsafe worktree path", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && mcpName == "":
		h.listAgentMCPs(w, r, a, wtDir)

	case r.Method == http.MethodPost && mcpName == "":
		h.addAgentMCP(w, r, a, wtDir)

	case r.Method == http.MethodDelete && mcpName != "":
		h.deleteAgentMCP(w, r, wtDir, mcpName)

	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

// readMCPFile reads .mcp.json from the given directory.
// Returns an empty file struct when the file does not exist.
func readMCPFile(wtDir string) (agentMCPFile, error) {
	var cfg agentMCPFile
	cfg.MCPServers = make(map[string]agentMCPEntry)

	mcpPath := filepath.Join(wtDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath) //nolint:gosec // trusted repo path
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read .mcp.json: %w", err)
	}
	if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
		return cfg, fmt.Errorf("parse .mcp.json: %w", jsonErr)
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]agentMCPEntry)
	}
	return cfg, nil
}

// writeMCPFile persists cfg to .mcp.json in wtDir.
func writeMCPFile(wtDir string, cfg agentMCPFile) error {
	if err := os.MkdirAll(wtDir, 0750); err != nil {
		return fmt.Errorf("ensure worktree dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal .mcp.json: %w", err)
	}
	mcpPath := filepath.Join(wtDir, ".mcp.json")
	if writeErr := os.WriteFile(mcpPath, b, 0600); writeErr != nil { //nolint:gosec // trusted path
		return fmt.Errorf("write .mcp.json: %w", writeErr)
	}
	return nil
}

// listAgentMCPs handles GET /api/agents/{name}/mcps.
func (h *AgentHandler) listAgentMCPs(w http.ResponseWriter, _ *http.Request, a *agent.Agent, wtDir string) {
	var servers []mcpServerDTO

	if wtDir != "" {
		cfg, err := readMCPFile(wtDir)
		if err != nil {
			httpInternalError(w, "read mcp config", err)
			return
		}
		for name, entry := range cfg.MCPServers {
			servers = append(servers, mcpServerDTO{
				Name:    name,
				URL:     entry.URL,
				Command: entry.Command,
				Type:    entry.Type,
				Env:     entry.Env,
			})
		}
	}

	// Fall back to role-resolved MCP servers if .mcp.json is empty.
	if len(servers) == 0 && h.home != nil && h.home.RoleManager != nil && string(a.Role) != "" {
		if resolved, resolveErr := h.home.RoleManager.ResolveRole(string(a.Role)); resolveErr == nil {
			for _, name := range resolved.MCPServers {
				servers = append(servers, mcpServerDTO{Name: name})
			}
		}
	}

	if servers == nil {
		servers = []mcpServerDTO{}
	}
	writeJSON(w, http.StatusOK, servers)
}

// addAgentMCP handles POST /api/agents/{name}/mcps.
func (h *AgentHandler) addAgentMCP(w http.ResponseWriter, r *http.Request, a *agent.Agent, wtDir string) {
	if wtDir == "" {
		httpError(w, "worktree path not available for this agent", http.StatusUnprocessableEntity)
		return
	}

	var req struct {
		Env     map[string]string `json:"env,omitempty"`
		Name    string            `json:"name"`
		URL     string            `json:"url,omitempty"`
		Command string            `json:"command,omitempty"`
		Type    string            `json:"type,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httpError(w, "name is required", http.StatusBadRequest)
		return
	}

	cfg, err := readMCPFile(wtDir)
	if err != nil {
		httpInternalError(w, "read mcp config", err)
		return
	}

	cfg.MCPServers[req.Name] = agentMCPEntry{
		URL:     req.URL,
		Command: req.Command,
		Type:    req.Type,
		Env:     req.Env,
	}

	if writeErr := writeMCPFile(wtDir, cfg); writeErr != nil {
		httpInternalError(w, "write mcp config", writeErr)
		return
	}

	// For tmux agents running claude, also run the provider's MCP add command
	// so it registers in the claude CLI config (not just .mcp.json).
	if a != nil && a.RuntimeBackend != "docker" && strings.EqualFold(a.Tool, "claude") {
		mcpName := req.Name
		worktreeDir := wtDir
		go func() {
			cmd := exec.CommandContext(context.Background(), "claude", "mcp", "add", mcpName) //nolint:gosec // mcpName validated above
			cmd.Dir = worktreeDir
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				log.Warn("mcp add command failed", "agent", a.Name, "mcp", mcpName, "error", cmdErr, "output", string(out))
			} else {
				log.Info("mcp add command succeeded", "agent", a.Name, "mcp", mcpName)
			}
		}()
	}

	writeJSON(w, http.StatusCreated, mcpServerDTO{
		Name:    req.Name,
		URL:     req.URL,
		Command: req.Command,
		Type:    req.Type,
		Env:     req.Env,
	})
}

// deleteAgentMCP handles DELETE /api/agents/{name}/mcps/{mcp}.
func (h *AgentHandler) deleteAgentMCP(w http.ResponseWriter, _ *http.Request, wtDir, mcpName string) {
	if wtDir == "" {
		httpError(w, "worktree path not available for this agent", http.StatusUnprocessableEntity)
		return
	}

	cfg, err := readMCPFile(wtDir)
	if err != nil {
		httpInternalError(w, "read mcp config", err)
		return
	}

	if _, exists := cfg.MCPServers[mcpName]; !exists {
		httpError(w, fmt.Sprintf("MCP server %q not found", mcpName), http.StatusNotFound)
		return
	}

	delete(cfg.MCPServers, mcpName)

	if writeErr := writeMCPFile(wtDir, cfg); writeErr != nil {
		httpInternalError(w, "write mcp config", writeErr)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Env var persistence ───────────────────────────────────────────────────────

// envVarEntry is a single key/value environment variable entry.
// Values holding ${secret:NAME} references travel as the reference —
// resolved values never leave the daemon.
type envVarEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// envEntries converts an agent env map to a stable key-sorted list.
func envEntries(env map[string]string) []envVarEntry {
	vars := make([]envVarEntry, 0, len(env))
	for k, v := range env {
		vars = append(vars, envVarEntry{Key: k, Value: v})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	return vars
}

// getAgentEnv handles GET /api/agents/{name}/env. Returns the agent's
// configured env vars from the store (the same map injected at spawn).
func (h *AgentHandler) getAgentEnv(w http.ResponseWriter, r *http.Request, agentName string) {
	a, err := h.svc.Get(r.Context(), agentName)
	if err != nil {
		httpError(w, err.Error(), agentHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, envEntries(a.Env))
}

// putAgentEnv handles PUT /api/agents/{name}/env. Replaces the agent's
// configured env vars; changes are injected on the next restart.
func (h *AgentHandler) putAgentEnv(w http.ResponseWriter, r *http.Request, agentName string) {
	var vars []envVarEntry
	if err := json.NewDecoder(r.Body).Decode(&vars); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	env := make(map[string]string, len(vars))
	for _, v := range vars {
		if !agent.IsValidEnvName(v.Key) {
			httpError(w, fmt.Sprintf("invalid env var name %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", v.Key), http.StatusBadRequest)
			return
		}
		env[v.Key] = v.Value
	}

	if err := h.svc.SetEnv(r.Context(), agentName, env); err != nil {
		httpError(w, err.Error(), agentHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, envEntries(env))
}
