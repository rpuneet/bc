// Package handlers: apps.go implements the /api/apps surface — the
// data-driven replacement for per-platform gateway CRUD. The catalog,
// connect/update/disconnect flow, and QR/OAuth auth dispatch are all
// driven by app descriptors; no per-platform switches.
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// AppsHandler handles /api/apps routes: the descriptor catalog, instance
// CRUD (secret fields → vault, plain fields → preferences), and auth
// flows dispatched on adapter capabilities.
type AppsHandler struct {
	gh    *GatewayHandler
	gw    *gateway.Manager
	ws    *workspace.Workspace
	vault *secret.Store
}

// NewAppsHandler creates an AppsHandler. The gateway handler serves the
// shared per-instance routes (health, channels, proxy, reactions) that
// the apps router delegates to.
func NewAppsHandler(gh *GatewayHandler, gw *gateway.Manager, ws *workspace.Workspace, vault *secret.Store) *AppsHandler {
	return &AppsHandler{gh: gh, gw: gw, ws: ws, vault: vault}
}

// Register mounts apps routes.
func (h *AppsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/apps", h.catalog)
	mux.HandleFunc("/api/apps/", h.router)
}

// --- catalog ---

// appFieldJSON is the wire shape of a descriptor field.
type appFieldJSON struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
}

// appDescriptorJSON is the wire shape of an app descriptor.
type appDescriptorJSON struct { //nolint:govet // field order matches JSON/API contract
	ID     string         `json:"id"`
	Label  string         `json:"label"`
	Auth   string         `json:"auth"`
	Multi  bool           `json:"multi"`
	Fields []appFieldJSON `json:"fields"`
	Docs   []string       `json:"docs"`
	// OAuthAvailable is true when the plugin implements app.OAuthFlow —
	// the UI offers "Sign in with <app>" alongside manual fields.
	OAuthAvailable bool `json:"oauth_available"`
}

// appInstanceJSON is the wire shape of one connected instance with its
// live adapter status. Config holds the plain fields plus server-computed
// has_<field> booleans for secret fields — secret values never leave the
// server. Channels are the adapter's discovered bc channel keys.
type appInstanceJSON struct { //nolint:govet // field order matches JSON/API contract
	Name      string         `json:"name"`
	App       string         `json:"app"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config,omitempty"`
	Connected bool           `json:"connected"`
	BotName   string         `json:"bot_name,omitempty"`
	Error     string         `json:"error,omitempty"`
	Channels  []string       `json:"channels"`
}

func descriptorJSON(p app.Plugin) appDescriptorJSON {
	d := p.Describe()
	fields := make([]appFieldJSON, 0, len(d.Fields))
	for _, f := range d.Fields {
		fields = append(fields, appFieldJSON{
			Key:         f.Key,
			Label:       f.Label,
			Placeholder: f.Placeholder,
			Secret:      f.Secret,
			Required:    f.Required,
		})
	}
	docs := d.Docs
	if docs == nil {
		docs = []string{}
	}
	_, oauthAvailable := p.(app.OAuthFlow)
	return appDescriptorJSON{
		ID:             d.ID,
		Label:          d.Label,
		Auth:           string(d.Auth),
		Multi:          d.Multi,
		Fields:         fields,
		Docs:           docs,
		OAuthAvailable: oauthAvailable,
	}
}

// catalog handles GET /api/apps — every registered descriptor plus the
// connected instances with adapter status.
func (h *AppsHandler) catalog(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	catalog := make([]appDescriptorJSON, 0)
	for _, p := range app.List() {
		catalog = append(catalog, descriptorJSON(p))
	}

	var discovered []string
	if h.gw != nil {
		discovered = h.gw.DiscoveredSources()
	}
	channelsFor := func(name string) []string {
		chs := []string{}
		prefix := name + ":"
		for _, ch := range discovered {
			if strings.HasPrefix(ch, prefix) {
				chs = append(chs, ch)
			}
		}
		return chs
	}

	instances := make([]appInstanceJSON, 0)
	seen := make(map[string]bool)
	if h.ws != nil && h.ws.Config != nil {
		names := make([]string, 0, len(h.ws.Config.Apps))
		for name := range h.ws.Config.Apps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ic := h.ws.Config.Apps[name]
			seen[name] = true
			cfg := make(map[string]any, len(ic.Config))
			for k, v := range ic.Config {
				cfg[k] = v
			}
			// Secret fields surface as has_<field> booleans so the UI can
			// render a "configured" state without ever seeing the value.
			if plugin, ok := app.Get(ic.App); ok {
				for _, f := range plugin.Describe().Fields {
					if f.Secret {
						cfg["has_"+f.Key] = h.hasSecret(name, f.Key)
					}
				}
			}
			inst := appInstanceJSON{
				Name:     name,
				App:      ic.App,
				Enabled:  ic.Enabled,
				Config:   cfg,
				Channels: channelsFor(name),
			}
			if h.gw != nil {
				status := h.gw.AdapterStatus(name)
				inst.Connected = status.Connected
				inst.BotName = status.BotName
				inst.Error = status.Error
			}
			instances = append(instances, inst)
		}
	}

	// Dynamically registered adapters not in config (e.g. an adapter
	// started programmatically) still surface as instances.
	if h.gw != nil {
		for _, name := range h.gw.AdapterNames() {
			if seen[name] {
				continue
			}
			status := h.gw.AdapterStatus(name)
			instances = append(instances, appInstanceJSON{
				Name:      name,
				App:       instanceApp(name),
				Enabled:   true,
				Connected: status.Connected,
				BotName:   status.BotName,
				Error:     status.Error,
				Channels:  channelsFor(name),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"catalog":   catalog,
		"instances": instances,
	})
}

// --- per-instance routes ---

// router dispatches /api/apps/{name}[/...].
func (h *AppsHandler) router(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/apps/")
	if path == "" {
		httpError(w, "app instance name required", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}

	switch rest {
	case "":
		switch r.Method {
		case http.MethodPost:
			h.update(w, r, name)
		case http.MethodDelete:
			h.delete(w, r, name)
		default:
			methodNotAllowed(w)
		}
	case "auth":
		h.auth(w, r, name)
	case "auth/status":
		h.authStatus(w, r, name)
	default:
		if h.gh == nil {
			serviceUnavailable(w, r, "gateway", "gateway handler not available")
			return
		}
		h.gh.appScopedRoute(w, r, name, rest)
	}
}

// instanceApp derives the descriptor ID from an instance name
// ("telegram:alerts" → "telegram").
func instanceApp(name string) string {
	if i := strings.Index(name, ":"); i >= 0 {
		return name[:i]
	}
	return name
}

// hasSecret reports whether the vault holds a non-empty value for an
// instance's secret field. Nil-safe for handlers without a vault.
func (h *AppsHandler) hasSecret(instance, key string) bool {
	if h == nil || h.vault == nil {
		return false
	}
	v, err := h.vault.GetValue(app.SecretName(instance, key))
	return err == nil && v != ""
}

// stateDir returns the per-instance state directory.
func (h *AppsHandler) stateDir(instance string) string {
	if h.ws == nil {
		return ""
	}
	return filepath.Join(h.ws.StateDir(), "apps", instance)
}

// update handles POST /api/apps/{name} — connect or update an instance.
// Submitted config values are split by the descriptor: secret fields go
// to the vault under app:<name>:<key>, plain fields persist in the
// workspace config. The adapter is then hot-(re)started.
func (h *AppsHandler) update(w http.ResponseWriter, r *http.Request, name string) {
	if h.ws == nil || h.ws.Config == nil {
		serviceUnavailable(w, r, "workspace", "workspace not available")
		return
	}

	var req struct {
		Config  map[string]string `json:"config"`
		Enabled *bool             `json:"enabled"`
		App     string            `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.App == "" {
		req.App = instanceApp(name)
	}

	plugin, ok := app.Get(req.App)
	if !ok {
		httpError(w, "unknown app: "+req.App, http.StatusBadRequest)
		return
	}
	d := plugin.Describe()
	if instanceApp(name) != d.ID {
		httpError(w, "instance name must be \""+d.ID+"\" or \""+d.ID+":<label>\"", http.StatusBadRequest)
		return
	}
	if name != d.ID && !d.Multi {
		httpError(w, "app "+d.ID+" does not support labeled instances", http.StatusBadRequest)
		return
	}

	fields := make(map[string]app.FieldSpec, len(d.Fields))
	for _, f := range d.Fields {
		fields[f.Key] = f
	}

	// Start from the existing plain config so partial updates don't wipe
	// unrelated fields.
	cfg := h.ws.Config
	existing, exists := cfg.Apps[name]
	plain := make(map[string]string, len(req.Config))
	for k, v := range existing.Config {
		plain[k] = v
	}
	secretVals := make(map[string]string)
	for k, v := range req.Config {
		f, known := fields[k]
		if !known {
			httpError(w, "unknown field \""+k+"\" for app "+d.ID, http.StatusBadRequest)
			return
		}
		if f.Secret {
			secretVals[k] = v
			continue
		}
		if v == "" {
			delete(plain, k)
		} else {
			plain[k] = v
		}
	}
	if len(plain) == 0 {
		plain = nil
	}
	if err := app.ValidateConfig(d, plain); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(secretVals) > 0 && h.vault == nil {
		serviceUnavailable(w, r, "secrets", "secrets vault not available")
		return
	}
	for k, v := range secretVals {
		vaultKey := app.SecretName(name, k)
		if v == "" {
			_ = h.vault.Delete(vaultKey) //nolint:errcheck // absent key is fine
			continue
		}
		if err := h.vault.Set(vaultKey, v, "app credential"); err != nil {
			httpInternalError(w, "store secret", err)
			return
		}
	}

	enabled := true
	if exists {
		enabled = existing.Enabled
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if cfg.Apps == nil {
		cfg.Apps = make(map[string]app.InstanceConfig)
	}
	cfg.Apps[name] = app.InstanceConfig{App: d.ID, Enabled: enabled, Config: plain}
	if err := cfg.Save(h.ws.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	resp := map[string]any{"status": "updated", "name": name, "app": d.ID, "enabled": enabled}
	if warning := h.restartAdapter(name, enabled); warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

// restartAdapter stops any running adapter for the instance and, when
// enabled, builds and hot-starts a fresh one from the saved config.
// Returns a warning message instead of failing the request — config is
// already persisted, and a bad token should be visible, not fatal.
func (h *AppsHandler) restartAdapter(name string, enabled bool) string {
	if h.gw == nil || h.ws == nil || h.ws.Config == nil {
		return ""
	}
	if err := h.gw.StopAdapter(name); err != nil {
		log.Warn("apps: stop adapter", "name", name, "error", err)
	}
	if !enabled {
		return ""
	}
	adapter, err := h.buildAdapter(name)
	if err != nil {
		return "adapter build failed: " + err.Error()
	}
	if err := h.gw.StartAdapter(adapter); err != nil {
		return "adapter start failed: " + err.Error()
	}
	log.Info("apps: adapter hot-started", "name", name)
	return ""
}

// buildAdapter builds the live adapter for a configured instance using
// its plugin and the vault-backed secret source.
func (h *AppsHandler) buildAdapter(name string) (gateway.NotificationAdapter, error) {
	ic := h.ws.Config.Apps[name]
	plugin, _ := app.Get(ic.App)
	var secrets app.SecretSource
	if h.vault != nil {
		secrets = app.VaultSecrets{Store: h.vault, Instance: name}
	}
	inst := app.ResolveInstance(name, ic, secrets)
	return plugin.Build(inst, app.Env{StateDir: h.stateDir(name)})
}

// delete handles DELETE /api/apps/{name} — stop the adapter, drop the
// config entry, purge app:<name>:* vault keys, and remove the state dir.
func (h *AppsHandler) delete(w http.ResponseWriter, r *http.Request, name string) {
	if h.ws == nil || h.ws.Config == nil {
		serviceUnavailable(w, r, "workspace", "workspace not available")
		return
	}
	cfg := h.ws.Config
	if _, ok := cfg.Apps[name]; !ok {
		httpError(w, "app instance not found: "+name, http.StatusNotFound)
		return
	}

	if h.gw != nil {
		if err := h.gw.StopAdapter(name); err != nil {
			log.Warn("apps: stop adapter on delete", "name", name, "error", err)
		}
	}

	delete(cfg.Apps, name)
	if err := cfg.Save(h.ws.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	if h.vault != nil {
		prefix := app.SecretName(name, "")
		if metas, err := h.vault.List(); err == nil {
			for _, meta := range metas {
				if strings.HasPrefix(meta.Name, prefix) {
					if err := h.vault.Delete(meta.Name); err != nil {
						log.Warn("apps: delete vault key", "key", meta.Name, "error", err)
					}
				}
			}
		}
	}

	if dir := h.stateDir(name); dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			log.Warn("apps: remove state dir", "dir", dir, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// auth handles POST /api/apps/{name}/auth — begins the instance's auth
// flow, dispatched on capabilities, never a platform switch: plugins
// implementing app.OAuthFlow get a browser flow (auth happens before an
// adapter exists), adapters implementing app.QRPairer get QR pairing.
func (h *AppsHandler) auth(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if plugin, ok := app.Get(instanceApp(name)); ok {
		if flow, isOAuth := plugin.(app.OAuthFlow); isOAuth {
			h.beginOAuth(w, r, name, plugin, flow)
			return
		}
	}
	pairer, ok := h.ensurePairer(w, r, name)
	if !ok {
		return
	}

	info, err := pairer.StartPairing(r.Context())
	if err != nil {
		httpInternalError(w, "start pairing", err)
		return
	}
	// When pairing completes synchronously (device already paired /
	// reconnect), record the authenticated identity in the vault so
	// agents can reference the session. Values are never logged.
	if info.State == "connected" && info.Phone != "" && h.vault != nil {
		key := strings.ToUpper(instanceApp(name)) + "_SESSION"
		if err := h.vault.Set(key, info.Phone, "app session"); err != nil {
			log.Warn("apps: store session identity", "key", key, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, info)
}

// authStatus handles GET /api/apps/{name}/auth/status — polls auth
// progress: OAuth sessions via ?session=<id> on the plugin, QR pairing
// on the running adapter.
func (h *AppsHandler) authStatus(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if plugin, ok := app.Get(instanceApp(name)); ok {
		if flow, isOAuth := plugin.(app.OAuthFlow); isOAuth {
			h.pollOAuth(w, r, name, flow)
			return
		}
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}
	adapter := h.gw.GetAdapter(name)
	if adapter == nil {
		writeJSON(w, http.StatusOK, app.PairInfo{State: "idle"})
		return
	}
	pairer, ok := adapter.(app.QRPairer)
	if !ok {
		httpError(w, "auth flow not supported for "+instanceApp(name), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, pairer.PairStatus())
}

// ensurePairer resolves (creating and starting if necessary) the
// instance's adapter and asserts the QRPairer capability. Writes the
// HTTP error response and returns ok=false on failure.
func (h *AppsHandler) ensurePairer(w http.ResponseWriter, r *http.Request, name string) (app.QRPairer, bool) {
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return nil, false
	}
	if h.ws == nil || h.ws.Config == nil {
		serviceUnavailable(w, r, "workspace", "workspace not available")
		return nil, false
	}

	appID := instanceApp(name)
	if _, ok := app.Get(appID); !ok {
		httpError(w, "unknown app: "+appID, http.StatusBadRequest)
		return nil, false
	}

	cfg := h.ws.Config
	if _, exists := cfg.Apps[name]; !exists {
		// Pair-first flow (e.g. WhatsApp): create the instance so the
		// paired session survives daemon restarts.
		if cfg.Apps == nil {
			cfg.Apps = make(map[string]app.InstanceConfig)
		}
		cfg.Apps[name] = app.InstanceConfig{App: appID, Enabled: true}
		if err := cfg.Save(h.ws.SettingsFile()); err != nil {
			httpInternalError(w, "save config", err)
			return nil, false
		}
	}

	adapter := h.gw.GetAdapter(name)
	if adapter == nil {
		built, err := h.buildAdapter(name)
		if err != nil {
			httpInternalError(w, "build adapter", err)
			return nil, false
		}
		if err := h.gw.StartAdapter(built); err != nil {
			httpInternalError(w, "start adapter", err)
			return nil, false
		}
		adapter = h.gw.GetAdapter(name)
	}

	pairer, ok := adapter.(app.QRPairer)
	if !ok {
		httpError(w, "auth flow not supported for "+appID, http.StatusBadRequest)
		return nil, false
	}
	return pairer, true
}

// authSessionJSON is the wire shape of a begun OAuth session. Device
// flow carries verification_url + user_code; callback flow carries
// auth_url.
type authSessionJSON struct { //nolint:govet // field order matches JSON/API contract
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	State           string    `json:"state"`
	AuthURL         string    `json:"auth_url,omitempty"`
	VerificationURL string    `json:"verification_url,omitempty"`
	UserCode        string    `json:"user_code,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

// beginOAuth handles POST /api/apps/{name}/auth for OAuth-capable
// plugins. The optional request body {"config": {...}} carries plain
// descriptor fields (e.g. oauth_client_id) which are persisted with the
// instance — auth-first flows create the instance just like pair-first.
func (h *AppsHandler) beginOAuth(w http.ResponseWriter, r *http.Request, name string, plugin app.Plugin, flow app.OAuthFlow) {
	if h.ws == nil || h.ws.Config == nil {
		serviceUnavailable(w, r, "workspace", "workspace not available")
		return
	}
	d := plugin.Describe()
	if instanceApp(name) != d.ID {
		httpError(w, "instance name must be \""+d.ID+"\" or \""+d.ID+":<label>\"", http.StatusBadRequest)
		return
	}

	var req struct {
		Config map[string]string `json:"config"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			httpError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Merge submitted plain fields over the stored config and persist the
	// instance, preserving an existing enabled state.
	cfg := h.ws.Config
	existing, exists := cfg.Apps[name]
	plain := make(map[string]string, len(existing.Config)+len(req.Config))
	for k, v := range existing.Config {
		plain[k] = v
	}
	for k, v := range req.Config {
		if v == "" {
			delete(plain, k)
			continue
		}
		plain[k] = v
	}
	if len(plain) == 0 {
		plain = nil
	}
	if err := app.ValidateConfig(d, plain); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	enabled := true
	if exists {
		enabled = existing.Enabled
	}
	if cfg.Apps == nil {
		cfg.Apps = make(map[string]app.InstanceConfig)
	}
	cfg.Apps[name] = app.InstanceConfig{App: d.ID, Enabled: enabled, Config: plain}
	if err := cfg.Save(h.ws.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	var secrets app.SecretSource
	if h.vault != nil {
		secrets = app.VaultSecrets{Store: h.vault, Instance: name}
	}
	inst := app.ResolveInstance(name, cfg.Apps[name], secrets)
	session, err := flow.BeginAuth(r.Context(), inst)
	if err != nil {
		// Begin failures are actionable user/config errors (missing
		// client ID, upstream rejection) — surface the message.
		httpError(w, "begin auth: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, authSessionJSON{
		ID:              session.ID,
		Kind:            session.Kind,
		State:           app.AuthStatePending,
		AuthURL:         session.AuthURL,
		VerificationURL: session.VerificationURL,
		UserCode:        session.UserCode,
		ExpiresAt:       session.ExpiresAt,
		IntervalSeconds: int(session.Interval / time.Second),
	})
}

// pollOAuth handles GET /api/apps/{name}/auth/status?session=<id> for
// OAuth-capable plugins. On completion the returned secrets are persisted
// to the vault exactly like POST /api/apps/{name}, the instance is
// enabled, and the adapter hot-started. Secrets never cross the wire.
func (h *AppsHandler) pollOAuth(w http.ResponseWriter, r *http.Request, name string, flow app.OAuthFlow) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		httpError(w, "session query parameter required", http.StatusBadRequest)
		return
	}
	res, err := flow.PollAuth(r.Context(), app.AuthSession{ID: sessionID})
	if err != nil {
		httpInternalError(w, "poll auth", err)
		return
	}
	if res.State != app.AuthStateComplete {
		writeJSON(w, http.StatusOK, res)
		return
	}

	if len(res.Secrets) > 0 {
		if h.vault == nil {
			serviceUnavailable(w, r, "secrets", "secrets vault not available")
			return
		}
		for k, v := range res.Secrets {
			if err := h.vault.Set(app.SecretName(name, k), v, "app credential"); err != nil {
				httpInternalError(w, "store secret", err)
				return
			}
		}
	}

	// Enable the instance and hot-start its adapter with the fresh
	// credentials.
	if h.ws != nil && h.ws.Config != nil {
		cfg := h.ws.Config
		ic, exists := cfg.Apps[name]
		if !exists {
			ic = app.InstanceConfig{App: instanceApp(name)}
		}
		if !exists || !ic.Enabled {
			ic.Enabled = true
			cfg.Apps[name] = ic
			if err := cfg.Save(h.ws.SettingsFile()); err != nil {
				httpInternalError(w, "save config", err)
				return
			}
		}
	}
	resp := map[string]any{"state": app.AuthStateComplete}
	if warning := h.restartAdapter(name, true); warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}
