package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// ModelInfo describes a single provider model with its availability status.
type ModelInfo struct { //nolint:govet // field order matches JSON/API contract
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

// ProviderInfo represents a provider with usage stats.
type ProviderInfo struct { //nolint:govet // field order matches JSON/API contract
	Name        string `json:"name"`
	Description string `json:"description"`
	Binary      string `json:"binary"`
	Command     string `json:"command"`
	InstallHint string `json:"install_hint"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	// Models is the provider's curated model list for UI pickers.
	// Empty means the provider has no model selection.
	Models       []ModelInfo `json:"models"`
	TotalCostUSD float64     `json:"total_cost_usd"`
	TotalTokens  int64       `json:"total_tokens"`
	AgentCount   int         `json:"agent_count"`
	Installed    bool        `json:"installed"`
	Enabled      bool        `json:"enabled"`
}

// ProviderDetail extends ProviderInfo with per-model cost breakdown and agent list.
type ProviderDetail struct {
	Config      map[string]string `json:"config"`
	Agents      []AgentSummary    `json:"agents"`
	CostByModel []ModelCost       `json:"cost_by_model"`
	ProviderInfo
}

// AgentSummary is a lightweight agent reference.
type AgentSummary struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	State string `json:"state"`
}

// ModelCost holds per-model cost data.
type ModelCost struct {
	Model        string  `json:"model"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// ProviderCommand describes a CLI command available for a provider.
type ProviderCommand struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Args        string `json:"args,omitempty"`
}

// MCPServer describes an MCP server configured for a provider.
type MCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	URL       string `json:"url,omitempty"`
	Command   string `json:"command,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// UpdateCheck holds the result of a provider version check.
type UpdateCheck struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateCommand   string `json:"update_command"`
	UpdateAvailable bool   `json:"update_available"`
}

// modelCacheEntry caches DynamicModelLister results to avoid shelling out per request.
type modelCacheEntry struct {
	at     time.Time
	models []ModelInfo
}

// ProviderHandler handles /api/providers routes.
type ProviderHandler struct {
	sf         singleflight.Group
	registry   *provider.Registry
	agents     *agent.AgentService
	costs      *cost.Store
	ws         *workspace.Workspace
	modelCache map[string]modelCacheEntry
	modelMu    sync.Mutex
}

// NewProviderHandler creates a ProviderHandler.
func NewProviderHandler(registry *provider.Registry, agents *agent.AgentService, costs *cost.Store, ws *workspace.Workspace) *ProviderHandler {
	return &ProviderHandler{registry: registry, agents: agents, costs: costs, ws: ws, modelCache: make(map[string]modelCacheEntry)}
}

// Register mounts provider routes on mux.
func (h *ProviderHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/providers", h.list)
	mux.HandleFunc("/api/providers/", h.byName)
}

// fetchModels returns models for provider p, using DynamicModelLister when available.
// Results are cached for 60 seconds. Available=true means live from CLI.
// Concurrent callers for the same provider share a single in-flight shell-out
// (singleflight), preventing the TOCTOU race that caused duplicate CLI invocations.
func (h *ProviderHandler) fetchModels(ctx context.Context, p provider.Provider) []ModelInfo {
	const ttl = 60 * time.Second
	const timeout = 10 * time.Second

	h.modelMu.Lock()
	if e, ok := h.modelCache[p.Name()]; ok && time.Since(e.at) < ttl {
		h.modelMu.Unlock()
		return e.models
	}
	h.modelMu.Unlock()

	// singleflight deduplicates concurrent fetches for the same provider so only
	// one shell-out fires per TTL window even under concurrent requests.
	raw, _, _ := h.sf.Do(p.Name(), func() (any, error) { //nolint:errcheck // sf.Do error is forwarded; closure never returns non-nil
		// Re-check cache inside the flight: a concurrent Do may have already
		// populated it while this goroutine was waiting.
		h.modelMu.Lock()
		if e, ok := h.modelCache[p.Name()]; ok && time.Since(e.at) < ttl {
			h.modelMu.Unlock()
			return e.models, nil
		}
		h.modelMu.Unlock()

		var models []ModelInfo

		if dl, ok := p.(provider.DynamicModelLister); ok {
			tctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if ids, err := dl.ListModels(tctx); err == nil && len(ids) > 0 {
				models = make([]ModelInfo, len(ids))
				for i, id := range ids {
					models[i] = ModelInfo{ID: id, Available: true}
				}
			}
		}

		// Static fallback if dynamic yielded nothing.
		if len(models) == 0 {
			if ml, ok := p.(provider.ModelLister); ok {
				for _, id := range ml.Models() {
					models = append(models, ModelInfo{ID: id, Available: false})
				}
			}
		}

		if models == nil {
			models = []ModelInfo{}
		}

		h.modelMu.Lock()
		h.modelCache[p.Name()] = modelCacheEntry{models: models, at: time.Now()}
		h.modelMu.Unlock()

		return models, nil
	})
	// The closure always returns []ModelInfo; the two-value assertion is a safe guard.
	ms, ok := raw.([]ModelInfo)
	if !ok {
		return []ModelInfo{}
	}
	return ms
}

// listModels returns live models for a provider (with caching).
func (h *ProviderHandler) listModels(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}
	models := h.fetchModels(r.Context(), p)
	writeJSON(w, http.StatusOK, models)
}

// list returns all providers with agent counts and cost stats.
// Each provider's buildProviderInfo (which may shell out via fetchModels) runs
// concurrently so a full cold-cache load stays bounded by the slowest single
// provider rather than the sum of all providers.
func (h *ProviderHandler) list(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	providers := h.registry.List()
	agentCounts := h.countAgents(r.Context())
	costByProvider := h.aggregateCostsByProvider(r.Context())

	// Pre-allocate by index so each goroutine writes to its own slot — no mutex needed.
	infos := make([]ProviderInfo, len(providers))
	var wg sync.WaitGroup
	wg.Add(len(providers))
	for i, p := range providers {
		i, p := i, p
		go func() {
			defer wg.Done()
			infos[i] = h.buildProviderInfo(r.Context(), p, agentCounts, costByProvider)
		}()
	}
	wg.Wait()

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	writeJSON(w, http.StatusOK, infos)
}

// byName handles /api/providers/:name and sub-routes.
func (h *ProviderHandler) byName(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/providers/"), "/", 2)
	name := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if name == "" {
		httpError(w, "provider name required", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		h.detail(w, r, name)
	case r.Method == http.MethodGet && action == "models":
		h.listModels(w, r, name)
	case r.Method == http.MethodGet && action == "commands":
		h.commands(w, r, name)
	case r.Method == http.MethodGet && action == "mcps":
		h.listMCPs(w, r, name)
	case r.Method == http.MethodPost && action == "mcps":
		h.addMCP(w, r, name)
	case r.Method == http.MethodPost && action == "install":
		h.install(w, r, name)
	case r.Method == http.MethodPost && action == "update":
		h.update(w, r, name)
	case r.Method == http.MethodPost && action == "check-update":
		h.checkUpdate(w, r, name)
	case r.Method == http.MethodPatch && action == "config":
		h.patchConfig(w, r, name)
	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

// detail returns a single provider with agents and per-model costs.
func (h *ProviderHandler) detail(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	agentCounts, agentsByProvider := h.agentSummariesByProvider(r.Context())
	costByProvider := h.aggregateCostsByProvider(r.Context())

	info := h.buildProviderInfo(r.Context(), p, agentCounts, costByProvider)

	detail := ProviderDetail{
		ProviderInfo: info,
		Config:       h.providerConfig(name),
		Agents:       agentsByProvider[name],
		CostByModel:  h.costByModelForProvider(r.Context(), name),
	}

	if detail.Agents == nil {
		detail.Agents = []AgentSummary{}
	}
	if detail.CostByModel == nil {
		detail.CostByModel = []ModelCost{}
	}

	writeJSON(w, http.StatusOK, detail)
}

// commands returns available CLI commands for a provider.
func (h *ProviderHandler) commands(w http.ResponseWriter, _ *http.Request, name string) {
	_, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	var cmds []ProviderCommand

	switch name {
	case "claude":
		cmds = []ProviderCommand{
			{Name: "mcp add", Command: "claude mcp add <name> <command>", Description: "Add MCP server", Args: "<name> <command|url>"},
			{Name: "mcp list", Command: "claude mcp list", Description: "List MCP servers"},
			{Name: "mcp remove", Command: "claude mcp remove <name>", Description: "Remove MCP server", Args: "<name>"},
			{Name: "config set", Command: "claude config set <key> <value>", Description: "Set config value", Args: "<key> <value>"},
			{Name: "config list", Command: "claude config list", Description: "List config values"},
			{Name: "version", Command: "claude --version", Description: "Show version"},
			{Name: "resume", Command: "claude --resume <id>", Description: "Resume session", Args: "<session-id>"},
		}
	default:
		binary := name
		cmds = []ProviderCommand{
			{Name: "run", Command: binary, Description: "Run " + name},
			{Name: "version", Command: binary + " --version", Description: "Show version"},
			{Name: "help", Command: binary + " --help", Description: "Show help"},
		}
	}

	writeJSON(w, http.StatusOK, cmds)
}

// listMCPs returns MCP servers configured for a provider.
func (h *ProviderHandler) listMCPs(w http.ResponseWriter, r *http.Request, name string) {
	_, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	var servers []MCPServer

	switch name {
	case "claude":
		servers = h.readClaudeMCPs(r.Context())
	case "cursor":
		servers = h.readCursorMCPs()
	default:
		servers = []MCPServer{}
	}

	writeJSON(w, http.StatusOK, servers)
}

// addMCP adds an MCP server to a provider's configuration.
func (h *ProviderHandler) addMCP(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		URL       string `json:"url"`
		Command   string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httpError(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.URL == "" && req.Command == "" {
		httpError(w, "url or command is required", http.StatusBadRequest)
		return
	}

	if h.ws == nil {
		httpError(w, "workspace not available", http.StatusServiceUnavailable)
		return
	}

	// Use the provider's ConfigAdapter if available.
	type mcpSetup interface {
		SetupMCP(ctx context.Context, targetDir, agentName string, servers map[string]provider.MCPEntry) error
	}
	adapter, hasAdapter := p.(mcpSetup)
	if !hasAdapter {
		httpError(w, name+" does not support MCP configuration", http.StatusBadRequest)
		return
	}

	transport := req.Transport
	if transport == "" {
		if req.URL != "" {
			transport = "sse"
		} else {
			transport = "stdio"
		}
	}
	entry := provider.MCPEntry{
		Transport: transport,
		URL:       req.URL,
		Command:   req.Command,
	}
	if err := adapter.SetupMCP(r.Context(), h.ws.RootDir, "", map[string]provider.MCPEntry{req.Name: entry}); err != nil {
		httpInternalError(w, "add mcp server", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":   "added",
		"provider": name,
		"mcp":      req.Name,
	})
}

// checkUpdate checks if a newer version is available for the provider.
func (h *ProviderHandler) checkUpdate(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	currentVersion := p.Version(r.Context())
	if currentVersion == "" {
		httpError(w, name+" is not installed", http.StatusBadRequest)
		return
	}

	// Return current version info with install hint as update command.
	// Actual latest version checking requires network calls to package registries
	// which can be added per-provider in the future.
	writeJSON(w, http.StatusOK, UpdateCheck{
		CurrentVersion:  currentVersion,
		LatestVersion:   currentVersion,
		UpdateAvailable: false,
		UpdateCommand:   p.InstallHint(),
	})
}

// install returns the install hint for the provider.
func (h *ProviderHandler) install(w http.ResponseWriter, _ *http.Request, name string) {
	h.hintResponse(w, name, "install")
}

// update returns the upgrade hint for the provider.
func (h *ProviderHandler) update(w http.ResponseWriter, _ *http.Request, name string) {
	h.hintResponse(w, name, "update")
}

// hintResponse returns the install/update hint for a provider.
func (h *ProviderHandler) hintResponse(w http.ResponseWriter, name, action string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	hint := p.InstallHint()
	if hint == "" {
		httpError(w, "no "+action+" command available for "+name, http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":        action + "_hint",
		"provider":      name,
		action + "_cmd": hint,
	})
}

// patchConfig updates the provider's command in workspace settings.
func (h *ProviderHandler) patchConfig(w http.ResponseWriter, r *http.Request, name string) {
	if h.ws == nil || h.ws.Config == nil {
		httpError(w, "workspace not available", http.StatusServiceUnavailable)
		return
	}

	_, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if h.ws.Config.Providers.Providers == nil {
		h.ws.Config.Providers.Providers = make(map[string]workspace.ProviderConfig)
	}
	h.ws.Config.Providers.Providers[name] = workspace.ProviderConfig{Command: req.Command}

	if err := h.ws.Save(); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "provider": name, "command": req.Command})
}

// buildProviderInfo builds a ProviderInfo from a provider and pre-computed maps.
func (h *ProviderHandler) buildProviderInfo(
	ctx context.Context,
	p provider.Provider,
	agentCounts map[string]int,
	costByProvider map[string]*costAgg,
) ProviderInfo {
	installed := p.IsInstalled(ctx)
	version := ""
	if installed {
		version = p.Version(ctx)
	}

	status := "not_installed"
	if installed {
		status = "healthy"
	}

	command := p.Command()
	if h.ws != nil && h.ws.Config != nil {
		if cfg := h.ws.Config.GetProvider(p.Name()); cfg != nil {
			command = cfg.Command
		}
	}

	enabled := installed
	if h.ws != nil && h.ws.Config != nil {
		_, enabled = h.ws.Config.Providers.Providers[p.Name()]
		if !enabled {
			enabled = installed
		}
	}

	models := h.fetchModels(ctx, p)

	info := ProviderInfo{
		Name:        p.Name(),
		Description: p.Description(),
		Binary:      p.Binary(),
		Command:     command,
		InstallHint: p.InstallHint(),
		Version:     version,
		Status:      status,
		Models:      models,
		AgentCount:  agentCounts[p.Name()],
		Installed:   installed,
		Enabled:     enabled,
	}

	if agg, ok := costByProvider[p.Name()]; ok {
		info.TotalTokens = agg.tokens
		info.TotalCostUSD = agg.cost
	}

	return info
}

// listAgents returns the raw agent list, or nil on error.
func (h *ProviderHandler) listAgents(ctx context.Context) []*agent.Agent {
	if h.agents == nil {
		return nil
	}
	agents, err := h.agents.List(ctx, agent.ListOptions{})
	if err != nil {
		return nil
	}
	return agents
}

// countAgents returns a count of agents per provider tool name.
// Used by the list endpoint which does not need full agent summaries.
func (h *ProviderHandler) countAgents(ctx context.Context) map[string]int {
	counts := make(map[string]int)
	for _, a := range h.listAgents(ctx) {
		if tool := strings.ToLower(a.Tool); tool != "" {
			counts[tool]++
		}
	}
	return counts
}

// agentSummariesByProvider groups agent summaries by provider tool name.
// Used by the detail endpoint which needs both counts and summaries.
func (h *ProviderHandler) agentSummariesByProvider(ctx context.Context) (map[string]int, map[string][]AgentSummary) {
	counts := make(map[string]int)
	byProvider := make(map[string][]AgentSummary)
	for _, a := range h.listAgents(ctx) {
		tool := strings.ToLower(a.Tool)
		if tool == "" {
			continue
		}
		counts[tool]++
		byProvider[tool] = append(byProvider[tool], AgentSummary{
			Name:  a.Name,
			Role:  string(a.Role),
			State: string(a.State),
		})
	}
	return counts, byProvider
}

// costAgg holds aggregated cost data.
type costAgg struct {
	tokens int64
	cost   float64
}

// aggregateCostsByProvider groups model costs by provider name.
// A model belongs to a provider if the model name contains the provider name.
func (h *ProviderHandler) aggregateCostsByProvider(ctx context.Context) map[string]*costAgg {
	result := make(map[string]*costAgg)
	if h.costs == nil {
		return result
	}

	summaries, err := h.costs.SummaryByModel(ctx)
	if err != nil {
		return result
	}

	providers := h.registry.List()
	for _, s := range summaries {
		model := strings.ToLower(s.Model)
		for _, p := range providers {
			if strings.Contains(model, strings.ToLower(p.Name())) {
				agg, ok := result[p.Name()]
				if !ok {
					agg = &costAgg{}
					result[p.Name()] = agg
				}
				agg.tokens += s.TotalTokens
				agg.cost += s.TotalCostUSD
				break
			}
		}
	}

	return result
}

// costByModelForProvider returns per-model costs for a specific provider.
func (h *ProviderHandler) costByModelForProvider(ctx context.Context, name string) []ModelCost {
	if h.costs == nil {
		return nil
	}

	summaries, err := h.costs.SummaryByModel(ctx)
	if err != nil {
		return nil
	}

	var models []ModelCost
	lowerName := strings.ToLower(name)
	for _, s := range summaries {
		if strings.Contains(strings.ToLower(s.Model), lowerName) {
			models = append(models, ModelCost{
				Model:        s.Model,
				TotalTokens:  s.TotalTokens,
				TotalCostUSD: s.TotalCostUSD,
			})
		}
	}

	return models
}

// providerConfig returns the workspace config for a provider as a string map.
func (h *ProviderHandler) providerConfig(name string) map[string]string {
	cfg := make(map[string]string)
	if h.ws == nil || h.ws.Config == nil {
		return cfg
	}

	if p := h.ws.Config.GetProvider(name); p != nil {
		cfg["command"] = p.Command
	}

	if h.ws.Config.Providers.Default == name {
		cfg["default"] = "true"
	}

	return cfg
}

// readClaudeMCPs reads MCP servers from claude mcp list or .mcp.json.
func (h *ProviderHandler) readClaudeMCPs(ctx context.Context) []MCPServer {
	// Try claude mcp list first
	if servers := h.readClaudeMCPsViaCLI(ctx); servers != nil {
		return servers
	}
	// Fallback: read .mcp.json from workspace root
	return h.readMCPJSON()
}

// readClaudeMCPsViaCLI runs claude mcp list and parses the output.
func (h *ProviderHandler) readClaudeMCPsViaCLI(ctx context.Context) []MCPServer {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, claudePath, "mcp", "list") //nolint:gosec // trusted binary
	if h.ws != nil {
		cmd.Dir = h.ws.RootDir
	}
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Parse the text output: each line is "<name>: <type> <url/command>"
	var servers []MCPServer
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		sName := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])

		s := MCPServer{Name: sName, Enabled: true}
		if strings.HasPrefix(rest, "sse") || strings.HasPrefix(rest, "SSE") {
			s.Transport = "sse"
			s.URL = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(rest, "sse"), "SSE"))
		} else if strings.HasPrefix(rest, "stdio") || strings.HasPrefix(rest, "STDIO") {
			s.Transport = "stdio"
			s.Command = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(rest, "stdio"), "STDIO"))
		} else {
			s.Transport = "stdio"
			s.Command = rest
		}
		servers = append(servers, s)
	}

	return servers
}

// mcpJSONEntry represents one server entry in an mcp.json config file.
type mcpJSONEntry struct {
	Command string   `json:"command,omitempty"`
	URL     string   `json:"url,omitempty"`
	Type    string   `json:"type,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// readMCPJSON reads .mcp.json from workspace root.
func (h *ProviderHandler) readMCPJSON() []MCPServer {
	return h.parseMCPJSONFile(".mcp.json")
}

// readCursorMCPs reads .cursor/mcp.json from workspace root.
func (h *ProviderHandler) readCursorMCPs() []MCPServer {
	return h.parseMCPJSONFile(filepath.Join(".cursor", "mcp.json"))
}

// parseMCPJSONFile reads and parses an mcp.json file at the given relative path.
func (h *ProviderHandler) parseMCPJSONFile(relPath string) []MCPServer {
	if h.ws == nil {
		return []MCPServer{}
	}

	data, err := os.ReadFile(filepath.Join(h.ws.RootDir, relPath)) //nolint:gosec // controlled workspace path
	if err != nil {
		return []MCPServer{}
	}

	var cfg struct {
		MCPServers map[string]mcpJSONEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return []MCPServer{}
	}

	servers := make([]MCPServer, 0, len(cfg.MCPServers))
	for name, entry := range cfg.MCPServers {
		s := MCPServer{Name: name, Enabled: true}
		if entry.Type == "sse" || entry.URL != "" {
			s.Transport = "sse"
			s.URL = entry.URL
		} else {
			s.Transport = "stdio"
			cmd := entry.Command
			if len(entry.Args) > 0 {
				cmd += " " + strings.Join(entry.Args, " ")
			}
			s.Command = cmd
		}
		servers = append(servers, s)
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers
}
