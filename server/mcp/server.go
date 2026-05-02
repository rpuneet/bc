package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// ctxKeyAgent is the context key for the agent ID extracted from the SSE connection.
type contextKey string

const ctxKeyAgent contextKey = "agent"

// AgentFromContext returns the agent ID from the context, if set.
func AgentFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyAgent).(string); ok {
		return v
	}
	return ""
}

// ContextWithAgent returns ctx with the given authenticated agent ID
// attached. The agent identity is treated as the only trusted source of
// "sender" for outbound MCP tool calls (#2967). Used by the SSE message
// handler and by tests that need to simulate an authenticated request.
func ContextWithAgent(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, ctxKeyAgent, agentID)
}

// Server is a bc MCP server. It owns handles to workspace state and dispatches
// JSON-RPC 2.0 requests from either stdio or SSE transports.
type Server struct {
	ws       *workspace.Workspace
	agents   *agent.Manager
	costs    *cost.Store
	gateway  *gateway.Manager
	notify   *notify.Service
	broker   *SSEBroker
	version  string
	ownCosts bool
}

// Config holds the dependencies needed to build a Server.
// When Channels or Costs are provided, the server reuses them (e.g. from bcd)
// instead of opening its own connections.
type Config struct {
	Workspace *workspace.Workspace
	Agents    *agent.Manager   // optional: pre-built agent manager
	Costs     *cost.Store      // optional: pre-built cost store
	Gateway   *gateway.Manager // optional: gateway manager for file uploads
	Notify    *notify.Service  // optional: notify service for messaging
	Version   string           // bc binary version, e.g. "1.2.3"
}

// New creates a Server. Call Close when done.
// When stores are provided via Config, the caller owns their lifecycle and
// Close will not close them.
func New(cfg Config) (*Server, error) {
	if cfg.Workspace == nil {
		return nil, fmt.Errorf("workspace is required")
	}

	// Track whether we created stores ourselves (so Close knows what to clean up).
	var ownCosts bool

	// Agent manager
	mgr := cfg.Agents
	if mgr == nil {
		mgr = agent.NewWorkspaceManager(cfg.Workspace.AgentsDir(), cfg.Workspace.RootDir)
		if err := mgr.LoadState(); err != nil {
			_ = err // Non-fatal
		}
	}

	// Cost store
	costStore := cfg.Costs
	if costStore == nil {
		costStore = cost.NewStore(cfg.Workspace.RootDir)
		if err := costStore.Open(); err != nil {
			_ = err // Non-fatal
		}
		ownCosts = true
	}

	v := cfg.Version
	if v == "" {
		v = "dev"
	}

	return &Server{
		ws:       cfg.Workspace,
		agents:   mgr,
		costs:    costStore,
		gateway:  cfg.Gateway,
		notify:   cfg.Notify,
		version:  v,
		ownCosts: ownCosts,
	}, nil
}

// Close releases resources held by the server.
// Only closes stores that the server created itself (not injected ones).
func (s *Server) Close() error {
	if s.ownCosts && s.costs != nil {
		return s.costs.Close()
	}
	return nil
}

// SetBroker attaches an SSE broker for MCP notifications.
// Message delivery is now handled directly by the OnMessage callback in
// server.go — no poller needed.
func (s *Server) SetBroker(broker *SSEBroker) {
	s.broker = broker
}

// ChannelMessagePayload is the notification payload for new channel messages.
type ChannelMessagePayload struct {
	Time    time.Time `json:"time"`
	Channel string    `json:"channel"`
	Sender  string    `json:"sender"`
	Message string    `json:"message"`
}

// NewChannelNotification creates a notifications/message JSON-RPC notification.
func NewChannelNotification(ch, sender, message string, t time.Time) Notification {
	return Notification{
		JSONRPC: "2.0",
		Method:  "notifications/message",
		Params: ChannelMessagePayload{
			Channel: ch,
			Sender:  sender,
			Message: message,
			Time:    t,
		},
	}
}

// Handle processes a single JSON-RPC request and returns the response.
// For notifications (no ID), the returned Response has a nil ID and no result/error set.
func (s *Server) Handle(ctx context.Context, req Request) Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		// Notification — no response needed; caller should discard
		return Response{}
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return errResponse(req.ID, ErrMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method))
	}
}

// ─── initialize ──────────────────────────────────────────────────────────────

func (s *Server) handleInitialize(req Request) Response {
	// Accept any client capabilities — we don't require specific ones.
	return okResponse(req.ID, initializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: serverCapabilities{
			Resources: &resourcesCapability{},
			Tools:     &toolsCapability{},
		},
		ServerInfo: serverInfo{
			Name:    "bc",
			Version: s.version,
		},
	})
}

// ─── resources/list ──────────────────────────────────────────────────────────

func (s *Server) handleResourcesList(req Request) Response {
	return okResponse(req.ID, resourcesListResult{
		Resources: definedResources(),
	})
}

// definedResources returns the static list of resources this server exposes.
func definedResources() []Resource {
	return []Resource{
		{
			URI:         "bc://workspace/status",
			Name:        "Workspace Status",
			Description: "Workspace name, path, version, and configuration summary",
			MIMEType:    "application/json",
		},
		{
			URI:         "bc://agents",
			Name:        "Agents",
			Description: "All agents with their state, role, tool, and worktree info",
			MIMEType:    "application/json",
		},
		{
			URI:         "bc://channels",
			Name:        "Channels",
			Description: "All channels with members and recent message counts",
			MIMEType:    "application/json",
		},
		{
			URI:         "bc://costs",
			Name:        "Costs",
			Description: "Workspace cost summary and per-agent breakdown",
			MIMEType:    "application/json",
		},
		{
			URI:         "bc://roles",
			Name:        "Roles",
			Description: "Role definitions with capabilities, permissions, and MCP server associations",
			MIMEType:    "application/json",
		},
		{
			URI:         "bc://tools",
			Name:        "Tools",
			Description: "AI agent tools available in this workspace",
			MIMEType:    "application/json",
		},
	}
}

// ─── resources/read ──────────────────────────────────────────────────────────

func (s *Server) handleResourcesRead(req Request) Response {
	var p resourcesReadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errResponse(req.ID, ErrInvalidParams, "invalid params: "+err.Error())
	}

	var (
		content string
		err     error
	)

	switch p.URI {
	case "bc://workspace/status":
		content, err = s.readWorkspaceStatus()
	case "bc://agents":
		content, err = s.readAgents()
	case "bc://channels":
		content, err = s.readChannels()
	case "bc://costs":
		content, err = s.readCosts()
	case "bc://roles":
		content, err = s.readRoles()
	case "bc://tools":
		content, err = s.readTools()
	default:
		return errResponse(req.ID, ErrInvalidParams, fmt.Sprintf("unknown resource URI: %s", p.URI))
	}

	if err != nil {
		return errResponse(req.ID, ErrInternal, err.Error())
	}

	return okResponse(req.ID, resourcesReadResult{
		Contents: []ResourceContent{
			{URI: p.URI, MIMEType: "application/json", Text: content},
		},
	})
}

// ─── tools/list ──────────────────────────────────────────────────────────────

func (s *Server) handleToolsList(req Request) Response {
	return okResponse(req.ID, toolsListResult{Tools: definedTools()})
}

// ─── tools/call ──────────────────────────────────────────────────────────────

func (s *Server) handleToolsCall(ctx context.Context, req Request) Response {
	var p toolsCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errResponse(req.ID, ErrInvalidParams, "invalid params: "+err.Error())
	}

	var (
		result *toolsCallResult
		err    error
	)

	switch p.Name {
	case "send_message":
		result, err = s.toolSendMessage(ctx, p.Arguments)
	case "list_channels":
		result, err = s.toolListChannels()
	case "read_channel":
		result, err = s.toolReadChannel(ctx, p.Arguments)
	case "send_file":
		result, err = s.toolSendFile(ctx, p.Arguments)
	case "whoami":
		result, err = s.toolWhoami(ctx)
	case "list_agents":
		result, err = s.toolListAgents(p.Arguments)
	default:
		return errResponse(req.ID, ErrInvalidParams, fmt.Sprintf("unknown tool: %s", p.Name))
	}

	if err != nil {
		return okResponse(req.ID, toolsCallResult{
			Content: []ToolContent{textContent(err.Error())},
			IsError: true,
		})
	}

	return okResponse(req.ID, result)
}

// ─── JSON marshaling helper ──────────────────────────────────────────────────

func marshalJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
