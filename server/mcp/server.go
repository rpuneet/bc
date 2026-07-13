// Package mcp is the agent-facing MCP server embedded in the mycel daemon.
//
// It is built on the official MCP Go SDK (streamable HTTP transport) and is
// mounted at /_mcp/{agent}. The agent name in the path is the ONLY trusted
// source of sender identity for outbound tools (#2967): each agent gets its
// own mcp.Server instance whose tool handlers close over that identity, so a
// client can never speak as another agent by supplying a different sender.
package mcp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// Config holds the daemon-owned dependencies the tool handlers dispatch to.
// All stores are shared with bcd; the MCP layer owns no connections.
type Config struct {
	Workspace *workspace.Workspace
	Agents    *agent.Manager
	Costs     *cost.Store
	Gateway   *gateway.Manager
	Notify    *notify.Service
	Version   string
}

// Handler serves the agent-scoped MCP endpoints. It lazily builds one
// sdk.Server per agent (tool closures capture the agent identity) and
// delegates the wire protocol to the SDK's streamable HTTP handler.
type Handler struct {
	servers map[string]*sdk.Server
	http    http.Handler
	cfg     Config
	mu      sync.RWMutex
}

// New creates a Handler. Workspace is required; every other dependency is
// optional and the corresponding tools degrade with a tool error.
func New(cfg Config) (*Handler, error) {
	if cfg.Workspace == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	h := &Handler{cfg: cfg, servers: make(map[string]*sdk.Server)}
	// Stateless: every POST is self-contained, so long-running daemons never
	// accumulate abandoned sessions from agents that were killed mid-flight.
	h.http = sdk.NewStreamableHTTPHandler(h.serverForRequest, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	return h, nil
}

// Register mounts the handler at /_mcp/{agent} on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/_mcp/", h)
}

// ServeHTTP validates the agent path segment before handing the request to
// the SDK transport, so unknown paths get a JSON 404 instead of a protocol
// error deep inside the SDK.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if agentFromPath(r.URL.Path) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found: expected /_mcp/{agent}"}`))
		return
	}
	h.http.ServeHTTP(w, r)
}

// serverForRequest returns the per-agent MCP server for the request path,
// building and caching it on first use. Returns nil (SDK responds 4xx) if
// the path does not name a valid agent.
func (h *Handler) serverForRequest(r *http.Request) *sdk.Server {
	name := agentFromPath(r.URL.Path)
	if name == "" {
		return nil
	}

	h.mu.RLock()
	s, ok := h.servers[name]
	h.mu.RUnlock()
	if ok {
		return s
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok = h.servers[name]; ok {
		return s
	}
	s = sdk.NewServer(&sdk.Implementation{Name: "mycel", Version: h.cfg.Version}, nil)
	addTools(s, h.cfg, name)
	h.servers[name] = s
	return s
}

// agentFromPath extracts and validates the agent name from /_mcp/{agent}
// (a trailing /sse segment from pre-rebuild .mcp.json files is tolerated so
// stale configs fail with a clear protocol error, not a 404). Returns ""
// when the path carries no valid agent name.
func agentFromPath(path string) string {
	rest, ok := strings.CutPrefix(path, "/_mcp/")
	if !ok || rest == "" {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimSuffix(rest, "/"), "/sse")
	if !agent.IsValidAgentName(name) {
		return ""
	}
	return name
}
