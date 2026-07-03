package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
)

// resolveSender returns the authoritative sender identity for outbound MCP
// tool calls (send_message, send_file).
//
// Security (issue #2967): the agent identity attached to the request context
// (set from the authenticated SSE connection — either the agent-scoped path
// /_mcp/<agent>/{sse,message} or the ?agent= query param) is the ONLY
// trusted source of identity. Any client-supplied "sender" value is
// advisory: if it disagrees with the context agent we log a warning and
// override it to prevent MCP sender spoofing.
//
// If no agent is bound to the context (legacy / unauthenticated flows) we
// fall back to the client value, and finally to "agent".
func resolveSender(ctx context.Context, clientSender string) string {
	ctxAgent := AgentFromContext(ctx)
	if ctxAgent != "" {
		if clientSender != "" && clientSender != ctxAgent {
			log.Warn("mcp: ignoring client-supplied sender — using authenticated agent",
				"client_sender", clientSender, "ctx_agent", ctxAgent)
		}
		return ctxAgent
	}
	if clientSender != "" {
		return clientSender
	}
	return "agent"
}

// definedTools returns the static list of tools this server exposes.
func definedTools() []Tool {
	return []Tool{
		{
			Name:        "send_file",
			Description: "Upload a file to a bc gateway channel (e.g., share a screenshot to Slack)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Gateway channel name (e.g., slack:all-bc)",
					},
					"file_path": map[string]any{
						"type":        "string",
						"description": "Local file path to upload",
					},
					"comment": map[string]any{
						"type":        "string",
						"description": "Optional text message to accompany the file",
					},
				},
				"required": []string{"channel", "file_path"},
			},
		},
		{
			Name:        "send_message",
			Description: "Send a text message to a gateway channel (e.g., slack:eng, telegram:trade)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Gateway channel name (e.g., slack:eng, telegram:trade)",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Message text to send",
					},
					"sender": map[string]any{
						"type":        "string",
						"description": "Sender name (defaults to agent identity)",
					},
				},
				"required": []string{"channel", "message"},
			},
		},
		{
			Name:        "list_channels",
			Description: "List all gateway channels with their platform and subscriber count",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "read_channel",
			Description: "Read recent messages from a gateway channel",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Gateway channel name (e.g., slack:eng)",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Number of messages to return (default 20)",
					},
				},
				"required": []string{"channel"},
			},
		},
		{
			Name:        "whoami",
			Description: "Returns the current agent's identity, role, workspace, and capabilities",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "list_agents",
			Description: "List all agents in the workspace with their status and role",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role": map[string]any{
						"type":        "string",
						"description": "Filter by role (optional)",
					},
				},
			},
		},
	}
}

// Per-field input length caps for MCP tool arguments. Enforced at handler
// entry so abusive callers can't exhaust memory via the /_mcp/* routes (which
// are exempt from the global MaxBodySize middleware — see
// server/handlers/helpers.go).
const (
	maxMessageLen = 64 * 1024 // 64KB — chat-style messages
	maxCommentLen = 64 * 1024 // 64KB — file upload comments
	maxChannelLen = 256       // gateway channel identifier
	maxSenderLen  = 256       // sender / agent display name
	maxRoleLen    = 256       // role filter on list_agents
	maxPathLen    = 4 * 1024  // file_path on send_file (matches typical PATH_MAX)
)

// validateLen returns a tool error result if v exceeds maxBytes. The bool
// indicates whether the caller should return early.
func validateLen(field, v string, maxBytes int) (*toolsCallResult, bool) {
	if len(v) > maxBytes {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("%s too long: %d bytes (max %d)", field, len(v), maxBytes))},
			IsError: true,
		}, true
	}
	return nil, false
}

// ─── send_message ───────────────────────────────────────────────────────────

func (s *Server) toolSendMessage(ctx context.Context, raw json.RawMessage) (*toolsCallResult, error) {
	var args struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
		Sender  string `json:"sender"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Channel == "" || args.Message == "" {
		return &toolsCallResult{
			Content: []ToolContent{textContent("channel and message are required")},
			IsError: true,
		}, nil
	}
	if res, stop := validateLen("channel", args.Channel, maxChannelLen); stop {
		return res, nil
	}
	if res, stop := validateLen("message", args.Message, maxMessageLen); stop {
		return res, nil
	}
	if res, stop := validateLen("sender", args.Sender, maxSenderLen); stop {
		return res, nil
	}
	// Always derive the sender from the authenticated context — never trust
	// the client-supplied value. See resolveSender / issue #2967.
	args.Sender = resolveSender(ctx, args.Sender)

	if s.gateway == nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent("no gateway configured — cannot send messages")},
			IsError: true,
		}, nil
	}

	sent, err := s.gateway.Send(ctx, args.Channel, args.Sender, args.Message)
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("send failed: %s", err))},
			IsError: true,
		}, nil
	}
	if !sent {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("channel %q is not a gateway channel", args.Channel))},
			IsError: true,
		}, nil
	}

	return &toolsCallResult{
		Content: []ToolContent{textContent(fmt.Sprintf("Sent to %s as %s", args.Channel, args.Sender))},
	}, nil
}

// ─── list_channels ──────────────────────────────────────────────────────────

func (s *Server) toolListChannels() (*toolsCallResult, error) {
	if s.gateway == nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent("no gateway configured")},
			IsError: true,
		}, nil
	}

	channels := s.gateway.DiscoveredSources()
	if len(channels) == 0 {
		return &toolsCallResult{
			Content: []ToolContent{textContent("(no channels)")},
		}, nil
	}

	var sb strings.Builder
	for _, ch := range channels {
		platform := ""
		if idx := strings.Index(ch, ":"); idx > 0 {
			platform = ch[:idx]
		}
		sb.WriteString(fmt.Sprintf("%-30s  platform=%s\n", ch, platform))
	}

	return &toolsCallResult{
		Content: []ToolContent{textContent(sb.String())},
	}, nil
}

// ─── read_channel ───────────────────────────────────────────────────────────

func (s *Server) toolReadChannel(ctx context.Context, raw json.RawMessage) (*toolsCallResult, error) {
	var args struct {
		Channel string `json:"channel"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Channel == "" {
		return &toolsCallResult{
			Content: []ToolContent{textContent("channel is required")},
			IsError: true,
		}, nil
	}
	if res, stop := validateLen("channel", args.Channel, maxChannelLen); stop {
		return res, nil
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}

	if s.notify == nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent("notify service not available")},
			IsError: true,
		}, nil
	}

	msgs, err := s.notify.Store().GetMessages(ctx, args.Channel, args.Limit, 0)
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("read failed: %s", err))},
			IsError: true,
		}, nil
	}

	if len(msgs) == 0 {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("(no messages in %s)", args.Channel))},
		}, nil
	}

	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.CreatedAt.Format("15:04"), m.Sender, m.Content))
	}

	return &toolsCallResult{
		Content: []ToolContent{textContent(sb.String())},
	}, nil
}

// ─── whoami ──────────────────────────────────────────────────────────────────

func (s *Server) toolWhoami(ctx context.Context) (*toolsCallResult, error) {
	agentID := AgentFromContext(ctx)
	if agentID == "" {
		if s.ws != nil {
			nick := s.ws.Config.User.Name
			nick = strings.TrimPrefix(nick, "@")
			if nick != "" {
				agentID = nick
			}
		}
	}
	if agentID == "" {
		agentID = "unknown"
	}

	info := map[string]any{
		"agent":     agentID,
		"workspace": "",
	}
	if s.ws != nil {
		info["workspace"] = s.ws.Name()
	}

	// Look up agent details if available
	if s.agents != nil {
		if ag := s.agents.GetAgent(agentID); ag != nil {
			info["role"] = ag.Role
			info["state"] = string(ag.State)
			if ag.Task != "" {
				info["task"] = ag.Task
			}
		}
	}

	b, _ := json.MarshalIndent(info, "", "  ")
	return &toolsCallResult{
		Content: []ToolContent{textContent(string(b))},
	}, nil
}

// ─── list_agents ─────────────────────────────────────────────────────────────

func (s *Server) toolListAgents(raw json.RawMessage) (*toolsCallResult, error) {
	var args struct {
		Role string `json:"role"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &args) //nolint:errcheck // optional args
	}
	if res, stop := validateLen("role", args.Role, maxRoleLen); stop {
		return res, nil
	}

	if s.agents == nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent("agent manager not available")},
			IsError: true,
		}, nil
	}

	agents := s.agents.ListAgents()
	var sb strings.Builder
	for _, ag := range agents {
		if args.Role != "" && string(ag.Role) != args.Role {
			continue
		}
		task := ag.Task
		if task == "" {
			task = "-"
		}
		sb.WriteString(fmt.Sprintf("%-20s  role=%-12s  state=%-8s  task=%s\n",
			ag.Name, ag.Role, ag.State, task))
	}
	if sb.Len() == 0 {
		sb.WriteString("(no agents)")
	}

	return &toolsCallResult{
		Content: []ToolContent{textContent(sb.String())},
	}, nil
}

// ─── send_file ──────────────────────────────────────────────────────────────

const maxFileSize = 50 * 1024 * 1024 // 50MB

func (s *Server) toolSendFile(ctx context.Context, raw json.RawMessage) (*toolsCallResult, error) {
	var args struct {
		Channel  string `json:"channel"`
		FilePath string `json:"file_path"`
		Comment  string `json:"comment"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Channel == "" || args.FilePath == "" {
		return &toolsCallResult{
			Content: []ToolContent{textContent("channel and file_path are required")},
			IsError: true,
		}, nil
	}
	if res, stop := validateLen("channel", args.Channel, maxChannelLen); stop {
		return res, nil
	}
	if res, stop := validateLen("file_path", args.FilePath, maxPathLen); stop {
		return res, nil
	}
	if res, stop := validateLen("comment", args.Comment, maxCommentLen); stop {
		return res, nil
	}

	// Validate file path is under workspace to prevent reading arbitrary files
	absPath, err := filepath.Abs(args.FilePath)
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("invalid file path: %s", err))},
			IsError: true,
		}, nil
	}

	// Security: ensure path is under workspace root or /tmp to prevent path traversal
	wsRoot := "/"
	if s.ws != nil {
		wsRoot = s.ws.RootDir
	}
	absPath = filepath.Clean(absPath)
	inRoot := absPath == wsRoot ||
		strings.HasPrefix(absPath, strings.TrimSuffix(wsRoot, "/")+"/") ||
		strings.HasPrefix(absPath, "/tmp/")
	if !inRoot {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("file path %q is outside workspace root %q", absPath, wsRoot))},
			IsError: true,
		}, nil
	}
	// Re-rooting a cleaned absolute path is a no-op but guarantees the
	// final path cannot traverse outside "/".
	absPath = filepath.Clean("/" + absPath)

	// Check file size before reading into memory
	info, err := os.Stat(absPath)
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("file not found: %s", err))},
			IsError: true,
		}, nil
	}
	if info.Size() > maxFileSize {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), maxFileSize))},
			IsError: true,
		}, nil
	}

	data, err := os.ReadFile(absPath) //nolint:gosec // path validated above
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("failed to read file: %s", err))},
			IsError: true,
		}, nil
	}

	// Detect MIME type — try content-based first, fall back to extension
	filename := filepath.Base(absPath)
	mimeType := "application/octet-stream"
	if len(data) >= 512 {
		mimeType = http.DetectContentType(data[:512])
	}
	// Override with extension for known types (DetectContentType can be imprecise)
	switch {
	case strings.HasSuffix(filename, ".png"):
		mimeType = "image/png"
	case strings.HasSuffix(filename, ".jpg"), strings.HasSuffix(filename, ".jpeg"):
		mimeType = "image/jpeg"
	case strings.HasSuffix(filename, ".gif"):
		mimeType = "image/gif"
	case strings.HasSuffix(filename, ".webp"):
		mimeType = "image/webp"
	case strings.HasSuffix(filename, ".pdf"):
		mimeType = "application/pdf"
	}

	// Get sender from context — always trust ctx over client input (#2967).
	// send_file currently has no client-supplied sender field, but route
	// through resolveSender for consistency and future-proofing.
	sender := resolveSender(ctx, "")

	// Route through gateway manager
	if s.gateway == nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent("no gateway configured — file upload requires a gateway channel (e.g., slack:all-bc)")},
			IsError: true,
		}, nil
	}

	sent, err := s.gateway.SendFile(ctx, args.Channel, sender, filename, data, mimeType)
	if err != nil {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("file upload failed: %s", err))},
			IsError: true,
		}, nil
	}
	if !sent {
		return &toolsCallResult{
			Content: []ToolContent{textContent(fmt.Sprintf("channel %q is not a gateway channel — file upload only works with gateway channels (slack:*, telegram:*, discord:*)", args.Channel))},
			IsError: true,
		}, nil
	}

	return &toolsCallResult{
		Content: []ToolContent{textContent(fmt.Sprintf("Uploaded %s (%d bytes) to %s", filename, len(data), args.Channel))},
	}, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// commandExists reports whether a command is available on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
