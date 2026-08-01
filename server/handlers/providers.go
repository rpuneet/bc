package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
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
	// Interactive is true when the command needs a TTY or mutates auth/session
	// state; the UI must not auto-run it (offer copy + "run in your terminal").
	Interactive bool `json:"interactive"`
	// Runnable is true when the command is safe to execute inline via the
	// guarded run endpoint (no TTY, no required args).
	Runnable bool `json:"runnable"`
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
type UpdateCheck struct { //nolint:govet // field order matches JSON/API contract
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateCommand  string `json:"update_command"`
	// Checked is true when LatestVersion/UpdateAvailable come from a real,
	// freshly-fetched registry lookup. False means the provider's install
	// mechanism has no queryable registry (e.g. a curl-piped script or a
	// bare download URL) — the UI must not claim "up to date" in that case.
	Checked         bool `json:"checked"`
	UpdateAvailable bool `json:"update_available"`
}

// modelCacheEntry caches DynamicModelLister results to avoid shelling out per request.
type modelCacheEntry struct {
	at     time.Time
	models []ModelInfo
}

// statusCacheEntry caches a provider's IsInstalled/Version result to avoid
// re-exec'ing LookPath + "<binary> --version" on every GET /api/providers.
type statusCacheEntry struct {
	at        time.Time
	version   string
	installed bool
}

// ProviderHandler handles /api/providers routes.
type ProviderHandler struct {
	sf          singleflight.Group
	registry    *provider.Registry
	agents      *agent.AgentService
	costs       *cost.Service
	h           *home.Home
	modelCache  map[string]modelCacheEntry
	statusCache map[string]statusCacheEntry
	modelMu     sync.Mutex
	statusMu    sync.Mutex
}

// NewProviderHandler creates a ProviderHandler.
func NewProviderHandler(registry *provider.Registry, agents *agent.AgentService, costs *cost.Service, h *home.Home) *ProviderHandler {
	return &ProviderHandler{
		registry:    registry,
		agents:      agents,
		costs:       costs,
		h:           h,
		modelCache:  make(map[string]modelCacheEntry),
		statusCache: make(map[string]statusCacheEntry),
	}
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

// fetchInstallStatus returns provider p's IsInstalled/Version, cached for 60
// seconds so buildProviderInfo doesn't re-exec LookPath + "<binary>
// --version" on every request. Mirrors fetchModels' cache+singleflight
// shape: concurrent callers for the same provider share one in-flight
// check, and a "status:" key prefix keeps this singleflight.Group entry
// distinct from fetchModels' same-name key on the shared h.sf group.
func (h *ProviderHandler) fetchInstallStatus(ctx context.Context, p provider.Provider) (installed bool, version string) {
	const ttl = 60 * time.Second

	h.statusMu.Lock()
	if e, ok := h.statusCache[p.Name()]; ok && time.Since(e.at) < ttl {
		h.statusMu.Unlock()
		return e.installed, e.version
	}
	h.statusMu.Unlock()

	raw, _, _ := h.sf.Do("status:"+p.Name(), func() (any, error) { //nolint:errcheck // sf.Do error is forwarded; closure never returns non-nil
		h.statusMu.Lock()
		if e, ok := h.statusCache[p.Name()]; ok && time.Since(e.at) < ttl {
			h.statusMu.Unlock()
			return e, nil
		}
		h.statusMu.Unlock()

		e := statusCacheEntry{at: time.Now(), installed: p.IsInstalled(ctx)}
		if e.installed {
			e.version = p.Version(ctx)
		}

		h.statusMu.Lock()
		h.statusCache[p.Name()] = e
		h.statusMu.Unlock()

		return e, nil
	})
	e, ok := raw.(statusCacheEntry)
	if !ok {
		return false, ""
	}
	return e.installed, e.version
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
	case r.Method == http.MethodPost && action == "run":
		h.runCommand(w, r, name)
	case r.Method == http.MethodGet && action == "mcps":
		h.listMCPs(w, r, name)
	case r.Method == http.MethodPost && action == "mcps":
		h.addMCP(w, r, name)
	case r.Method == http.MethodPost && action == "install":
		h.install(w, r, name)
	case r.Method == http.MethodPost && action == "update":
		h.update(w, r, name)
	case r.Method == http.MethodPost && action == "uninstall":
		h.uninstall(w, r, name)
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
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, providerCommands(p, name))
}

// providerCommands returns p's curated command list (or a generic default) for
// display. Interactive/Runnable are resolved for the UI.
//
// Only providers exposing a curated CommandLister get runnable=true entries:
// their commands are constant literals safe to execute via the guarded /run
// endpoint. The generic default interpolates the provider name into its command
// strings, so those entries are always reported runnable=false — the run
// endpoint (resolveRunnableArgv) likewise refuses non-CommandLister providers,
// keeping the display flag and the execution policy in lockstep.
func providerCommands(p provider.Provider, name string) []ProviderCommand {
	cl, isCurated := p.(provider.CommandLister)
	var listed []provider.Command
	if isCurated {
		listed = cl.Commands()
	} else {
		// Generic default for providers without a curated command list.
		binary := name
		listed = []provider.Command{
			{Name: "run", Command: binary, Description: "Run " + name, Interactive: true},
			{Name: "version", Command: binary + " --version", Description: "Show version"},
			{Name: "help", Command: binary + " --help", Description: "Show help"},
		}
	}
	cmds := make([]ProviderCommand, 0, len(listed))
	for _, c := range listed {
		cmds = append(cmds, ProviderCommand{
			Name:        c.Name,
			Command:     c.Command,
			Description: c.Description,
			Args:        c.Args,
			Interactive: c.Interactive,
			Runnable:    isCurated && c.Runnable(),
		})
	}
	return cmds
}

// listMCPs returns MCP servers configured for a provider. Providers
// without the MCPConfigReader capability list as empty.
func (h *ProviderHandler) listMCPs(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	servers := []MCPServer{}
	if mr, ok := p.(provider.MCPConfigReader); ok {
		rootDir := ""
		if h.h != nil {
			rootDir = h.h.RootDir
		}
		for _, s := range mr.ReadMCPs(r.Context(), rootDir) {
			servers = append(servers, MCPServer{
				Name:      s.Name,
				Transport: s.Transport,
				URL:       s.URL,
				Command:   s.Command,
				Enabled:   s.Enabled,
			})
		}
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

	if h.h == nil {
		httpError(w, "global config not available", http.StatusServiceUnavailable)
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
	if err := adapter.SetupMCP(r.Context(), h.h.RootDir, "", map[string]provider.MCPEntry{req.Name: entry}); err != nil {
		httpInternalError(w, "add mcp server", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":   "added",
		"provider": name,
		"mcp":      req.Name,
	})
}

// checkUpdate performs a real latest-version check for the provider and
// reports whether an update is available. Providers whose InstallHint is an
// npm/npx install line are checked against the public npm registry — the
// same "compare a live upstream to the installed version" approach
// About.tsx uses for the daemon itself (npm + GitHub releases). Providers
// installed via a non-registry mechanism (curl-piped script, bare download
// URL) have no queryable source of truth; UpdateCheck.Checked is false for
// those and the UI must show current-version-only, never a false "latest".
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

	hint := p.InstallHint()
	result := UpdateCheck{
		CurrentVersion: currentVersion,
		UpdateCommand:  hint,
	}

	if pkg, ok := npmPackageForHint(hint); ok {
		if latest, err := fetchNpmLatestVersion(r.Context(), pkg); err == nil && latest != "" {
			result.LatestVersion = latest
			result.Checked = true
			result.UpdateAvailable = normalizeVersion(latest) != normalizeVersion(currentVersion)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// runnableInstallHint reports whether hint is safe to execute directly as a
// shell command (npm/npx/curl/brew/…) rather than being a bare URL a human
// must open manually (e.g. cursor's "https://cursor.sh"). Mirrors
// providerInstallCmd's predicate in deps_install.go so the install and
// update paths agree on what's automatable.
func runnableInstallHint(hint string) bool {
	h := strings.TrimSpace(hint)
	return h != "" && !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "https://")
}

// npmPackageForHint extracts the npm package name from a provider's install
// hint when the hint is an npm/npx install line ("npm install -g <pkg>",
// "npm i -g <pkg>", "npx -y <pkg>", "npx <pkg>"). Returns ok=false for any
// other install mechanism — those have no npm registry entry to query.
func npmPackageForHint(hint string) (string, bool) {
	h := strings.TrimSpace(hint)
	for _, pfx := range []string{"npm install -g ", "npm i -g ", "npx -y ", "npx "} {
		if !strings.HasPrefix(h, pfx) {
			continue
		}
		pkg := strings.TrimSpace(strings.TrimPrefix(h, pfx))
		if sp := strings.IndexByte(pkg, ' '); sp >= 0 {
			pkg = pkg[:sp]
		}
		if pkg != "" {
			return pkg, true
		}
	}
	return "", false
}

// normalizeVersion strips a leading "v" so "v1.2.3" and "1.2.3" compare equal.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// npmRegistryBaseURL and npmHTTPClient are package vars so tests can point
// checkUpdate at a local httptest.Server instead of the real npm registry.
var npmRegistryBaseURL = "https://registry.npmjs.org/"
var npmHTTPClient = &http.Client{Timeout: 5 * time.Second}

// fetchNpmLatestVersion queries the public npm registry for pkg's current
// "latest" dist-tag version — a real network check, not a guess.
func fetchNpmLatestVersion(ctx context.Context, pkg string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, npmRegistryBaseURL+url.PathEscape(pkg)+"/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := npmHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned %d for %s", resp.StatusCode, pkg)
	}

	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("npm registry: empty version for %s", pkg)
	}
	return body.Version, nil
}

// install returns the install hint for the provider. The web UI prefers the
// real streamed installer at POST /api/deps/install (id=<provider name>,
// which resolves the same InstallHint via providerInstallCmd in
// deps_install.go); this endpoint remains for API callers that just want the
// hint text.
func (h *ProviderHandler) install(w http.ResponseWriter, _ *http.Request, name string) {
	h.hintResponse(w, name, "install")
}

// update performs a real update by re-running the provider's install command
// (e.g. "npm install -g <pkg>", which always resolves to the latest
// published version) and streaming live output back as NDJSON — the same
// on-host execution model install-via-deps and /api/deps/install use
// (streamInstall, shared within this package). Loopback-only: it shells out
// on the host.
func (h *ProviderHandler) update(w http.ResponseWriter, r *http.Request, name string) {
	if !checkLoopback(w, r) {
		return
	}

	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	hint := strings.TrimSpace(p.InstallHint())
	if !runnableInstallHint(hint) {
		httpError(w, "no automatic updater for "+name, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	emit := func(v any) bool {
		payload, mErr := json.Marshal(v)
		if mErr != nil {
			return false
		}
		if _, wErr := w.Write(append(payload, '\n')); wErr != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	emit(map[string]string{"type": "start", "command": hint})
	streamInstall(r.Context(), hint, emit)
}

// isRequiredProvider reports whether name is mycel's currently configured
// default provider (providers.default in prefs.json) — the one every new
// agent spawns with unless told otherwise. Uninstalling it out from under a
// running daemon would strand that default, so uninstall refuses it; the
// user can uninstall it after switching the default provider elsewhere.
func isRequiredProvider(h *home.Home, name string) bool {
	if h == nil {
		return false
	}
	return strings.EqualFold(h.DefaultProvider(), name)
}

// uninstall removes a provider's CLI by deriving a remove command from its
// vetted InstallHint (npm-global or Homebrew — the same two package
// managers resolveUninstall in deps_install.go supports) and streaming live
// output back as NDJSON, mirroring update's execution model. Refuses the
// configured default provider (isRequiredProvider) and any hint that
// doesn't map to an unambiguous uninstall command. Loopback-only: it shells
// out on the host.
func (h *ProviderHandler) uninstall(w http.ResponseWriter, r *http.Request, name string) {
	if !checkLoopback(w, r) {
		return
	}

	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	if isRequiredProvider(h.h, name) {
		httpError(w, name+" is the default provider and cannot be uninstalled", http.StatusBadRequest)
		return
	}

	cmd, ok := deriveUninstall(p.InstallHint())
	if !ok {
		httpError(w, "no automatic uninstaller for "+name, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	emit := func(v any) bool {
		payload, mErr := json.Marshal(v)
		if mErr != nil {
			return false
		}
		if _, wErr := w.Write(append(payload, '\n')); wErr != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	emit(map[string]string{"type": "start", "command": cmd})
	streamInstall(r.Context(), cmd, emit)
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

// patchConfig updates the provider's command in the global settings.
func (h *ProviderHandler) patchConfig(w http.ResponseWriter, r *http.Request, name string) {
	if h.h == nil || h.h.Config == nil {
		httpError(w, "global config not available", http.StatusServiceUnavailable)
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

	if h.h.Config.Providers.Providers == nil {
		h.h.Config.Providers.Providers = make(map[string]home.ProviderConfig)
	}
	h.h.Config.Providers.Providers[name] = home.ProviderConfig{Command: req.Command}

	if err := h.h.Save(); err != nil {
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
	installed, version := h.fetchInstallStatus(ctx, p)

	status := "not_installed"
	if installed {
		status = "healthy"
	}

	command := p.Command()
	if h.h != nil && h.h.Config != nil {
		if cfg := h.h.Config.GetProvider(p.Name()); cfg != nil {
			command = cfg.Command
		}
	}

	enabled := installed
	if h.h != nil && h.h.Config != nil {
		_, enabled = h.h.Config.Providers.Providers[p.Name()]
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

// providerConfig returns the global config for a provider as a string map.
func (h *ProviderHandler) providerConfig(name string) map[string]string {
	cfg := make(map[string]string)
	if h.h == nil || h.h.Config == nil {
		return cfg
	}

	if p := h.h.Config.GetProvider(name); p != nil {
		cfg["command"] = p.Command
	}

	if h.h.Config.Providers.Default == name {
		cfg["default"] = "true"
	}

	return cfg
}
