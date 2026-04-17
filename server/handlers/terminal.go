package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/rpuneet/bc/pkg/agent"
	"github.com/rpuneet/bc/pkg/log"
)

// TerminalHandler handles /api/agents/:name/terminal WebSocket connections.
// It bridges the browser to a tmux session via a PTY.
type TerminalHandler struct {
	svc        *agent.AgentService
	corsOrigin string
}

// NewTerminalHandler creates a TerminalHandler.
// corsOrigin is the allowed origin for WebSocket connections (empty or "*" allows all).
func NewTerminalHandler(svc *agent.AgentService, corsOrigin string) *TerminalHandler {
	return &TerminalHandler{
		svc:        svc,
		corsOrigin: corsOrigin,
	}
}

// resolveSvc returns the context-scoped agent service with closure fallback.
func (h *TerminalHandler) resolveSvc(r *http.Request) *agent.AgentService {
	if view := WorkspaceFromContext(r.Context()); view != nil && view.Agents != nil {
		return view.Agents
	}
	return h.svc
}

// HandleTerminal upgrades an HTTP request to a WebSocket and bridges it to a tmux session.
func (h *TerminalHandler) HandleTerminal(w http.ResponseWriter, r *http.Request, agentName string) {
	svc := h.resolveSvc(r)
	mgr := svc.Manager()

	// Verify agent exists and is active
	ag := mgr.GetAgent(agentName)
	if ag == nil {
		httpError(w, "agent not found", http.StatusNotFound)
		return
	}
	if ag.State == agent.StateStopped || ag.State == agent.StateError {
		httpError(w, "agent is not active", http.StatusConflict)
		return
	}

	// Get the runtime backend and verify session exists
	rt := mgr.RuntimeForAgent(agentName)
	if rt == nil {
		httpError(w, "no runtime backend for agent", http.StatusInternalServerError)
		return
	}
	if !rt.HasSession(r.Context(), agentName) {
		httpError(w, "no active session for agent", http.StatusConflict)
		return
	}

	// Upgrade to WebSocket with origin check
	upgrader := websocket.Upgrader{
		CheckOrigin: h.checkOrigin,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn("terminal: websocket upgrade failed", "agent", agentName, "error", err)
		return // Upgrade already sent error response
	}

	// Start tmux attach in a PTY
	cmd := rt.AttachCmd(r.Context(), agentName)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Warn("terminal: pty start failed", "agent", agentName, "error", err)
		writeWSError(conn, "failed to attach to session")
		_ = conn.Close()
		return
	}

	// Set initial terminal size
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}) //nolint:errcheck

	log.Info("terminal: attached", "agent", agentName)

	// Use sync.Once to ensure cleanup runs exactly once when either goroutine exits.
	// This prevents the deadlock where ptmx.Read blocks forever after the WebSocket
	// disconnects — closing ptmx unblocks the read, and closing conn unblocks ReadMessage.
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = conn.Close() //nolint:errcheck
			_ = ptmx.Close() //nolint:errcheck
		})
	}
	defer func() {
		cleanup()
		_ = cmd.Wait() //nolint:errcheck // reap zombie process
	}()

	var wg sync.WaitGroup

	// PTY → WebSocket (read from tmux, send to browser)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cleanup()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// WebSocket → PTY (read from browser, send to tmux)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cleanup()
		for {
			msgType, msg, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			switch msgType {
			case websocket.TextMessage:
				// Check for resize control messages
				if len(msg) > 0 && msg[0] == '{' && strings.Contains(string(msg), "resize") {
					handleResize(ptmx, msg)
					continue
				}
				// Regular text input
				if _, writeErr := ptmx.Write(msg); writeErr != nil {
					return
				}
			case websocket.BinaryMessage:
				if _, writeErr := ptmx.Write(msg); writeErr != nil {
					return
				}
			}
		}
	}()

	wg.Wait()
	log.Info("terminal: detached", "agent", agentName)
}

// checkOrigin validates the WebSocket origin against the configured CORS origin.
func (h *TerminalHandler) checkOrigin(r *http.Request) bool {
	if h.corsOrigin == "" || h.corsOrigin == "*" {
		return true
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == h.corsOrigin
}

// resizeMsg is the JSON structure for terminal resize messages.
type resizeMsg struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// handleResize parses a resize message and updates the PTY size.
func handleResize(ptmx *os.File, msg []byte) {
	var rm resizeMsg
	if err := json.Unmarshal(msg, &rm); err != nil {
		return
	}
	if rm.Cols > 0 && rm.Rows > 0 && rm.Cols <= 65535 && rm.Rows <= 65535 { //nolint:gosec // bounds checked
		_ = pty.Setsize(ptmx, &pty.Winsize{ //nolint:errcheck
			Rows: uint16(rm.Rows), //nolint:gosec // bounds checked above
			Cols: uint16(rm.Cols), //nolint:gosec // bounds checked above
		})
	}
}

// writeWSError sends an error message over WebSocket before closing.
func writeWSError(conn *websocket.Conn, msg string) {
	deadline := time.Now().Add(5 * time.Second)
	_ = conn.WriteControl( //nolint:errcheck
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, msg),
		deadline,
	)
}

// isWebSocketRequest returns true if the request is a WebSocket upgrade.
// Used by the Gzip middleware to skip compression for WebSocket connections,
// since gzip wraps the ResponseWriter and breaks http.Hijacker.
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
