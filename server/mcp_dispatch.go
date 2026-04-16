// mcp_dispatch.go — scoped MCP SSE routing for multi-workspace bcd.
//
// Introduced in phase M6: the canonical MCP endpoint becomes
//
//	/_mcp/ws/<wsID>/<agent>/{sse,message}
//
// dispatched per-request by the WorkspaceManager. Each loaded workspace
// maintains its own MCP server instance (built lazily on first scoped
// call). Legacy /_mcp/<agent>/* paths continue to target the launch
// workspace's server for backward compatibility with agents that were
// spawned before the path scheme changed.
package server

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/rpuneet/bc/pkg/log"
	servermcp "github.com/rpuneet/bc/server/mcp"
)

// perWorkspaceMCP tracks one MCP server per workspace ID so dispatch is
// deterministic across concurrent SSE subscribers.
type perWorkspaceMCP struct {
	servers map[string]*servermcp.Server // wsID -> mcp server
	brokers map[string]*servermcp.SSEBroker
	mu      sync.Mutex
}

var mcpRegistry = &perWorkspaceMCP{
	servers: make(map[string]*servermcp.Server),
	brokers: make(map[string]*servermcp.SSEBroker),
}

// scopedMCPDispatch returns an HTTP handler that parses paths of the form
// /_mcp/ws/<wsID>/<agent>/<action> and dispatches them to the workspace's
// MCP server. The first request for a given wsID lazily constructs that
// workspace's MCP server using its loaded WorkspaceServices.
func scopedMCPDispatch(mgr *WorkspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remainder := strings.TrimPrefix(r.URL.Path, "/_mcp/ws/")
		parts := strings.SplitN(remainder, "/", 3)
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		wsID, agentName, action := parts[0], parts[1], parts[2]
		if wsID == "" || agentName == "" || action == "" {
			http.NotFound(w, r)
			return
		}

		// Lazy-load workspace services.
		svc := mgr.Get(wsID)
		if svc == nil {
			var err error
			svc, err = mgr.Load(r.Context(), wsID)
			if err != nil {
				log.Warn("scoped MCP: load workspace failed", "id", wsID, "error", err)
				http.Error(w, `{"error":"failed to load workspace"}`, http.StatusInternalServerError)
				return
			}
		}

		srv, broker, err := ensureMCPServer(wsID, svc)
		if err != nil {
			log.Warn("scoped MCP: server build failed", "id", wsID, "error", err)
			http.Error(w, `{"error":"mcp unavailable"}`, http.StatusInternalServerError)
			return
		}

		// Inject agent identity via query param — the SSE broker + JSON-RPC
		// handler both read ?agent=<name>.
		q := r.URL.Query()
		q.Set("agent", agentName)
		r.URL.RawQuery = q.Encode()

		switch action {
		case "sse":
			broker.SSEHandler()(w, r)
		case "message":
			srv.HandleSSEMessage(context.Background(), broker)(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

// ensureMCPServer returns a cached MCP server for wsID or builds one on
// first access. Caller must hold the workspace services (non-nil).
func ensureMCPServer(wsID string, svc *WorkspaceServices) (*servermcp.Server, *servermcp.SSEBroker, error) {
	mcpRegistry.mu.Lock()
	defer mcpRegistry.mu.Unlock()

	if srv, ok := mcpRegistry.servers[wsID]; ok {
		return srv, mcpRegistry.brokers[wsID], nil
	}

	cfg := servermcp.Config{Workspace: svc.Workspace, Costs: svc.Costs}
	if svc.Agents != nil {
		cfg.Agents = svc.Agents.Manager()
	}
	if svc.Gateway != nil {
		cfg.Gateway = svc.Gateway
	}
	if svc.Notify != nil {
		cfg.Notify = svc.Notify
	}
	srv, err := servermcp.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	broker := servermcp.NewSSEBroker()
	broker.SetMessageEndpoint("/_mcp/ws/" + wsID + "/{agent}/message")
	srv.SetBroker(broker)

	mcpRegistry.servers[wsID] = srv
	mcpRegistry.brokers[wsID] = broker
	return srv, broker, nil
}
