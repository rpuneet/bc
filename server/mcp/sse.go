package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ServeSSE starts an HTTP server that implements the MCP SSE transport.
//
// Endpoints:
//   - GET  /sse      — client connects; receives server→client events as SSE
//   - POST /message  — client sends JSON-RPC requests; response sent via SSE
//
// addr must be a host:port pair. If addr is a bare ":port" it is rewritten
// to "127.0.0.1:port" so the server only listens on localhost — never on all
// interfaces — which prevents accidental network exposure.
//
// The server shuts down cleanly when ctx is canceled.
func (s *Server) ServeSSE(ctx context.Context, addr string) error {
	addr = LocalhostAddr(addr)

	broker := NewSSEBroker()
	s.SetBroker(broker)

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.handleSSE)
	mux.HandleFunc("/message", s.HandleSSEMessage(ctx, broker))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","server":"bc-mcp","version":%q}`, s.version) //nolint:errcheck // writing to response
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Shut down when ctx is canceled
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background()) //nolint:contextcheck
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("SSE server error: %w", err)
	}
	return nil
}

// HandleSSEMessage processes POST /message — client→server direction.
// Exported so tests can mount it on their own ServeMux.
func (s *Server) HandleSSEMessage(ctx context.Context, broker *SSEBroker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			resp := errResponse(nil, ErrParse, "parse error: "+err.Error())
			broker.send(resp)
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// Pass agent identity from query param into context for tool handlers.
		if agentID := r.URL.Query().Get("agent"); agentID != "" {
			ctx = ContextWithAgent(ctx, agentID)
		}

		resp := s.Handle(ctx, req)

		// Notifications have no ID — no response to send
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		broker.send(resp)
		w.WriteHeader(http.StatusAccepted)
	}
}

// ─── SSE broker ───────────────────────────────────────────────────────────────

// sseClient tracks a connected SSE client and its agent identity.
type sseClient struct {
	ch        chan []byte
	agentName string // empty for non-agent clients (e.g., web UI)
}

// SSEBroker fans out SSE messages to all connected clients.
type SSEBroker struct {
	clients         map[chan []byte]*sseClient
	messageEndpoint string
	corsOrigin      string
	mu              sync.Mutex
}

// NewSSEBroker creates an SSEBroker ready to use.
//
// The default CORS origin is "*" — safe because bcd binds to loopback by
// default. Callers that expose bcd beyond loopback should call
// SetCORSOrigin to lock the SSE endpoint to a specific origin (issue #2960).
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients:         make(map[chan []byte]*sseClient),
		messageEndpoint: "/message",
		corsOrigin:      "*",
	}
}

// SetCORSOrigin overrides the Access-Control-Allow-Origin value emitted on
// SSE responses. Pass an empty string to keep the wildcard default. Used by
// bcd to mirror the API CORSOrigin setting onto the MCP SSE transport so
// MCP cannot bypass the configured origin policy (issue #2960).
func (b *SSEBroker) SetCORSOrigin(origin string) {
	if origin == "" {
		return
	}
	b.mu.Lock()
	b.corsOrigin = origin
	b.mu.Unlock()
}

// SetMessageEndpoint overrides the endpoint URL returned to new SSE clients
// in the initial `event: endpoint` line. Used by scoped mounts (phase M6)
// so each per-workspace broker advertises its scoped /message path.
func (b *SSEBroker) SetMessageEndpoint(endpoint string) {
	b.mu.Lock()
	b.messageEndpoint = endpoint
	b.mu.Unlock()
}

func (b *SSEBroker) subscribe(agentName string) chan []byte {
	ch := make(chan []byte, 8)
	b.mu.Lock()
	b.clients[ch] = &sseClient{ch: ch, agentName: agentName}
	b.mu.Unlock()
	return ch
}

func (b *SSEBroker) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *SSEBroker) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	msg := append([]byte("data: "), data...)
	msg = append(msg, '\n', '\n')

	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default: // Drop if the client is slow
		}
	}
}

// SendToAgents sends a notification only to clients whose agent name is in the set.
// Used for channel-membership-filtered message delivery.
func (b *SSEBroker) SendToAgents(v any, agents map[string]bool) {
	if len(agents) == 0 {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	msg := append([]byte("data: "), data...)
	msg = append(msg, '\n', '\n')

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, client := range b.clients {
		if client.agentName == "" || !agents[client.agentName] {
			continue
		}
		select {
		case client.ch <- msg:
		default:
		}
	}
}

// SSEHandler returns an http.HandlerFunc for the SSE endpoint.
// Exported so tests in mcp_test can mount it on their own ServeMux.
func (b *SSEBroker) SSEHandler() http.HandlerFunc {
	return b.handleSSE
}

// handleSSE streams server→client events over SSE.
func (b *SSEBroker) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	b.mu.Lock()
	origin := b.corsOrigin
	b.mu.Unlock()
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)

	agentID := r.URL.Query().Get("agent")
	ch := b.subscribe(agentID)
	defer b.unsubscribe(ch)

	// Build the message endpoint URL with agent identity.
	// If the agent connected via /mcp/{agent}/sse, point to /mcp/{agent}/message.
	// Otherwise fall back to legacy ?agent= query param.
	endpoint := b.messageEndpoint
	if agentID != "" {
		// Derive agent-scoped message path from the SSE request path.
		// e.g., /mcp/swift-hawk/sse → /mcp/swift-hawk/message
		if strings.Contains(r.URL.Path, "/"+agentID+"/sse") {
			endpoint = strings.Replace(r.URL.Path, "/sse", "/message", 1)
		} else {
			endpoint += "?agent=" + url.QueryEscape(agentID)
		}
	}
	// Send endpoint event so client knows where to POST
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint) //nolint:errcheck // writing to response
	flusher.Flush()

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.Write(msg) //nolint:errcheck
			flusher.Flush()
		case <-keepalive.C:
			// SSE comment line — prevents idle timeout, ignored by clients
			fmt.Fprint(w, ": keepalive\n\n") //nolint:errcheck // writing to response
			flusher.Flush()
		}
	}
}

// MountOn registers MCP SSE endpoints on an existing ServeMux under the given prefix.
// Supports both legacy paths (/mcp/sse) and agent-scoped paths (/mcp/{agent}/sse).
// Returns the broker so callers can push notifications directly.
func MountOn(mux *http.ServeMux, srv *Server, prefix string) *SSEBroker {
	broker := NewSSEBroker()
	broker.messageEndpoint = prefix + "/message"
	srv.SetBroker(broker)

	// Legacy endpoints (no agent identity — backward compatible)
	mux.HandleFunc(prefix+"/sse", broker.handleSSE)
	mux.HandleFunc(prefix+"/message", srv.HandleSSEMessage(context.Background(), broker))

	// Agent-scoped endpoints: /mcp/{agent}/sse and /mcp/{agent}/message
	// The agent name is extracted from the URL path, not a query param.
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		// Parse: prefix + "/{agent}/{action}"
		remainder := strings.TrimPrefix(r.URL.Path, prefix+"/")
		parts := strings.SplitN(remainder, "/", 2)
		if len(parts) != 2 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		agentName := parts[0]
		action := parts[1]

		switch action {
		case "sse":
			// Inject agent name — handleSSE reads from query param,
			// so set it on the URL to reuse the same handler.
			q := r.URL.Query()
			q.Set("agent", agentName)
			r.URL.RawQuery = q.Encode()
			broker.handleSSE(w, r)
		case "message":
			q := r.URL.Query()
			q.Set("agent", agentName)
			r.URL.RawQuery = q.Encode()
			srv.HandleSSEMessage(context.Background(), broker)(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	return broker
}

// LocalhostAddr rewrites a bare ":port" address to "127.0.0.1:port".
// Explicit host addresses (e.g. "0.0.0.0:8811") are returned unchanged so
// callers that deliberately want to bind all interfaces can still do so.
func LocalhostAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
