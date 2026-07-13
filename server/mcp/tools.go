package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rpuneet/mycel/pkg/log"
)

// Per-field input length caps. Enforced at handler entry so abusive callers
// can't exhaust memory via /_mcp/* (exempt from the global MaxBodySize
// middleware — see server/handlers/helpers.go).
const (
	maxMessageLen = 64 * 1024 // chat-style messages
	maxCommentLen = 64 * 1024 // file upload comments
	maxChannelLen = 256       // gateway channel identifier
	maxRoleLen    = 256       // role filter on list_agents
	maxTaskLen    = 1024      // report_status task line
	maxPathLen    = 4 * 1024  // file_path on send_file (typical PATH_MAX)
	maxFileSize   = 50 * 1024 * 1024
	maxReadLimit  = 200 // read_channel page size cap
)

// readOnly marks tools with no side effects so MCP clients can skip
// confirmation prompts for them.
var readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true}

// addTools registers every tool on s. Handlers close over agentName — the
// identity derived from the authenticated /_mcp/{agent} path — which is the
// only trusted sender for outbound calls (#2967).
func addTools(s *sdk.Server, cfg Config, agentName string) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "whoami",
		Description: "Returns the current agent's identity, role, state, and workspace",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ emptyIn) (*sdk.CallToolResult, whoamiOut, error) {
		return nil, whoami(cfg, agentName), nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_agents",
		Description: "List all agents in the workspace with their status and role",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in listAgentsIn) (*sdk.CallToolResult, listAgentsOut, error) {
		return listAgents(cfg, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_channels",
		Description: "List all gateway channels (e.g., slack:eng, telegram:trade) messages can be sent to",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ emptyIn) (*sdk.CallToolResult, listChannelsOut, error) {
		return listChannels(cfg)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "read_channel",
		Description: "Read recent messages from a gateway channel",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in readChannelIn) (*sdk.CallToolResult, readChannelOut, error) {
		return readChannel(ctx, cfg, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "send_message",
		Description: "Send a text message to a gateway channel as this agent",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in sendMessageIn) (*sdk.CallToolResult, sendMessageOut, error) {
		return sendMessage(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "send_file",
		Description: "Upload a file to a gateway channel (e.g., share a screenshot to Slack)",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in sendFileIn) (*sdk.CallToolResult, sendFileOut, error) {
		return sendFile(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "report_status",
		Description: "Update this agent's current task line shown in the TUI and web UI",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in reportStatusIn) (*sdk.CallToolResult, reportStatusOut, error) {
		return reportStatus(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "query_costs",
		Description: "Query token usage and cost, for one agent or the whole workspace",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in queryCostsIn) (*sdk.CallToolResult, queryCostsOut, error) {
		return queryCosts(ctx, cfg, in)
	})
}

type emptyIn struct{}

// ─── whoami ─────────────────────────────────────────────────────────────────

type whoamiOut struct {
	Agent     string `json:"agent" jsonschema:"the agent's name"`
	Workspace string `json:"workspace,omitempty" jsonschema:"the workspace name"`
	Role      string `json:"role,omitempty" jsonschema:"the agent's role"`
	State     string `json:"state,omitempty" jsonschema:"the agent's lifecycle state"`
	Task      string `json:"task,omitempty" jsonschema:"the agent's current task line"`
}

func whoami(cfg Config, agentName string) whoamiOut {
	out := whoamiOut{Agent: agentName, Workspace: cfg.Workspace.Name()}
	if cfg.Agents != nil {
		if ag := cfg.Agents.GetAgent(agentName); ag != nil {
			out.Role = string(ag.Role)
			out.State = string(ag.State)
			out.Task = ag.Task
		}
	}
	return out
}

// ─── list_agents ────────────────────────────────────────────────────────────

type listAgentsIn struct {
	Role string `json:"role,omitempty" jsonschema:"filter by role (optional)"`
}

type agentInfo struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	State string `json:"state"`
	Task  string `json:"task,omitempty"`
}

type listAgentsOut struct {
	Agents []agentInfo `json:"agents"`
}

func listAgents(cfg Config, in listAgentsIn) (*sdk.CallToolResult, listAgentsOut, error) {
	var out listAgentsOut
	if len(in.Role) > maxRoleLen {
		return nil, out, fmt.Errorf("role too long: %d bytes (max %d)", len(in.Role), maxRoleLen)
	}
	if cfg.Agents == nil {
		return nil, out, fmt.Errorf("agent manager not available")
	}
	for _, ag := range cfg.Agents.ListAgents() {
		if in.Role != "" && string(ag.Role) != in.Role {
			continue
		}
		out.Agents = append(out.Agents, agentInfo{
			Name:  ag.Name,
			Role:  string(ag.Role),
			State: string(ag.State),
			Task:  ag.Task,
		})
	}
	return nil, out, nil
}

// ─── list_channels ──────────────────────────────────────────────────────────

type channelInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform,omitempty"`
}

type listChannelsOut struct {
	Channels []channelInfo `json:"channels"`
}

func listChannels(cfg Config) (*sdk.CallToolResult, listChannelsOut, error) {
	var out listChannelsOut
	if cfg.Gateway == nil {
		return nil, out, fmt.Errorf("no gateway configured")
	}
	for _, ch := range cfg.Gateway.DiscoveredSources() {
		platform := ""
		if idx := strings.Index(ch, ":"); idx > 0 {
			platform = ch[:idx]
		}
		out.Channels = append(out.Channels, channelInfo{Name: ch, Platform: platform})
	}
	return nil, out, nil
}

// ─── read_channel ───────────────────────────────────────────────────────────

type readChannelIn struct {
	Channel string `json:"channel" jsonschema:"gateway channel name (e.g., slack:eng)"`
	Limit   int    `json:"limit,omitempty" jsonschema:"number of messages to return (default 20)"`
}

type channelMessage struct {
	Time    string `json:"time"`
	Sender  string `json:"sender"`
	Content string `json:"content"`
}

type readChannelOut struct {
	Channel  string           `json:"channel"`
	Messages []channelMessage `json:"messages"`
}

func readChannel(ctx context.Context, cfg Config, in readChannelIn) (*sdk.CallToolResult, readChannelOut, error) {
	out := readChannelOut{Channel: in.Channel}
	if in.Channel == "" {
		return nil, out, fmt.Errorf("channel is required")
	}
	if len(in.Channel) > maxChannelLen {
		return nil, out, fmt.Errorf("channel too long: %d bytes (max %d)", len(in.Channel), maxChannelLen)
	}
	if cfg.Notify == nil {
		return nil, out, fmt.Errorf("notify service not available")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}
	msgs, err := cfg.Notify.Store().GetMessages(ctx, in.Channel, limit, 0)
	if err != nil {
		return nil, out, fmt.Errorf("read failed: %w", err)
	}
	for _, m := range msgs {
		out.Messages = append(out.Messages, channelMessage{
			Time:    m.CreatedAt.Format("2006-01-02 15:04"),
			Sender:  m.Sender,
			Content: m.Content,
		})
	}
	return nil, out, nil
}

// ─── send_message ───────────────────────────────────────────────────────────

type sendMessageIn struct {
	Channel string `json:"channel" jsonschema:"gateway channel name (e.g., slack:eng, telegram:trade)"`
	Message string `json:"message" jsonschema:"message text to send"`
	Sender  string `json:"sender,omitempty" jsonschema:"ignored — the sender is always the authenticated agent"`
}

type sendMessageOut struct {
	Channel string `json:"channel"`
	Sender  string `json:"sender"`
}

func sendMessage(ctx context.Context, cfg Config, agentName string, in sendMessageIn) (*sdk.CallToolResult, sendMessageOut, error) {
	out := sendMessageOut{Channel: in.Channel, Sender: agentName}
	if in.Channel == "" || in.Message == "" {
		return nil, out, fmt.Errorf("channel and message are required")
	}
	if len(in.Channel) > maxChannelLen {
		return nil, out, fmt.Errorf("channel too long: %d bytes (max %d)", len(in.Channel), maxChannelLen)
	}
	if len(in.Message) > maxMessageLen {
		return nil, out, fmt.Errorf("message too long: %d bytes (max %d)", len(in.Message), maxMessageLen)
	}
	// The path-derived agent identity is the only trusted sender (#2967).
	if in.Sender != "" && in.Sender != agentName {
		log.Warn("mcp: ignoring client-supplied sender — using authenticated agent",
			"client_sender", in.Sender, "agent", agentName)
	}
	if cfg.Gateway == nil {
		return nil, out, fmt.Errorf("no gateway configured — cannot send messages")
	}
	sent, err := cfg.Gateway.Send(ctx, in.Channel, agentName, in.Message)
	if err != nil {
		return nil, out, fmt.Errorf("send failed: %w", err)
	}
	if !sent {
		return nil, out, fmt.Errorf("channel %q is not a gateway channel", in.Channel)
	}
	return nil, out, nil
}

// ─── send_file ──────────────────────────────────────────────────────────────

type sendFileIn struct {
	Channel  string `json:"channel" jsonschema:"gateway channel name (e.g., slack:eng)"`
	FilePath string `json:"file_path" jsonschema:"local file path to upload"`
	Comment  string `json:"comment,omitempty" jsonschema:"optional text message to accompany the file"`
}

type sendFileOut struct {
	Channel  string `json:"channel"`
	Filename string `json:"filename"`
	Bytes    int    `json:"bytes"`
}

func sendFile(ctx context.Context, cfg Config, agentName string, in sendFileIn) (*sdk.CallToolResult, sendFileOut, error) {
	out := sendFileOut{Channel: in.Channel}
	if in.Channel == "" || in.FilePath == "" {
		return nil, out, fmt.Errorf("channel and file_path are required")
	}
	if len(in.Channel) > maxChannelLen {
		return nil, out, fmt.Errorf("channel too long: %d bytes (max %d)", len(in.Channel), maxChannelLen)
	}
	if len(in.FilePath) > maxPathLen {
		return nil, out, fmt.Errorf("file_path too long: %d bytes (max %d)", len(in.FilePath), maxPathLen)
	}
	if len(in.Comment) > maxCommentLen {
		return nil, out, fmt.Errorf("comment too long: %d bytes (max %d)", len(in.Comment), maxCommentLen)
	}

	absPath, err := validateFilePath(cfg, in.FilePath)
	if err != nil {
		return nil, out, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, out, fmt.Errorf("file not found: %w", err)
	}
	if info.Size() > maxFileSize {
		return nil, out, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}
	data, err := os.ReadFile(absPath) //nolint:gosec // path validated above
	if err != nil {
		return nil, out, fmt.Errorf("failed to read file: %w", err)
	}

	filename := filepath.Base(absPath)
	out.Filename = filename
	out.Bytes = len(data)

	if cfg.Gateway == nil {
		return nil, out, fmt.Errorf("no gateway configured — file upload requires a gateway channel (e.g., slack:eng)")
	}
	sent, err := cfg.Gateway.SendFile(ctx, in.Channel, agentName, filename, data, detectMIME(filename, data))
	if err != nil {
		return nil, out, fmt.Errorf("file upload failed: %w", err)
	}
	if !sent {
		return nil, out, fmt.Errorf("channel %q is not a gateway channel — file upload only works with gateway channels (slack:*, telegram:*, discord:*)", in.Channel)
	}
	if in.Comment != "" {
		if _, err := cfg.Gateway.Send(ctx, in.Channel, agentName, in.Comment); err != nil {
			log.Warn("mcp: file uploaded but comment failed", "channel", in.Channel, "error", err)
		}
	}
	return nil, out, nil
}

// validateFilePath resolves p — including symlinks, so a link inside an
// allowed root can't point the read at a file outside it — and ensures it
// stays under the workspace root or /tmp, preventing the tool from
// exfiltrating arbitrary host files.
func validateFilePath(cfg Config, p string) (string, error) {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	// Resolve the root too (macOS /tmp and /var are themselves symlinks).
	wsRoot := cfg.Workspace.RootDir
	if r, err := filepath.EvalSymlinks(wsRoot); err == nil {
		wsRoot = r
	}
	if !underDir(resolved, wsRoot) && !underDir(resolved, "/tmp") && !underDir(resolved, "/private/tmp") {
		return "", fmt.Errorf("file path %q is outside workspace root %q", resolved, wsRoot)
	}
	return resolved, nil
}

// underDir reports whether path is dir or lexically contained in it. Both
// arguments must already be symlink-resolved absolute paths.
func underDir(path, dir string) bool {
	dir = strings.TrimSuffix(dir, "/")
	return path == dir || strings.HasPrefix(path, dir+"/")
}

// detectMIME sniffs content and falls back to extension for common types
// (DetectContentType can be imprecise for images and PDFs).
func detectMIME(filename string, data []byte) string {
	mimeType := "application/octet-stream"
	if len(data) >= 512 {
		mimeType = http.DetectContentType(data[:512])
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	case ".pdf":
		mimeType = "application/pdf"
	}
	return mimeType
}

// ─── report_status ──────────────────────────────────────────────────────────

type reportStatusIn struct {
	Task string `json:"task" jsonschema:"one-line description of what the agent is working on"`
}

type reportStatusOut struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

func reportStatus(ctx context.Context, cfg Config, agentName string, in reportStatusIn) (*sdk.CallToolResult, reportStatusOut, error) {
	out := reportStatusOut{Agent: agentName, Task: in.Task}
	if len(in.Task) > maxTaskLen {
		return nil, out, fmt.Errorf("task too long: %d bytes (max %d)", len(in.Task), maxTaskLen)
	}
	if cfg.Agents == nil {
		return nil, out, fmt.Errorf("agent manager not available")
	}
	if err := cfg.Agents.SetAgentTask(ctx, agentName, in.Task); err != nil {
		return nil, out, fmt.Errorf("update failed: %w", err)
	}
	return nil, out, nil
}

// ─── query_costs ────────────────────────────────────────────────────────────

type queryCostsIn struct {
	Agent string `json:"agent,omitempty" jsonschema:"agent name to query (omit for the whole workspace)"`
}

type costSummary struct {
	Agent        string  `json:"agent,omitempty"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type queryCostsOut struct {
	Workspace *costSummary  `json:"workspace,omitempty"`
	Agents    []costSummary `json:"agents,omitempty"`
}

func queryCosts(ctx context.Context, cfg Config, in queryCostsIn) (*sdk.CallToolResult, queryCostsOut, error) {
	var out queryCostsOut
	if cfg.Costs == nil {
		return nil, out, fmt.Errorf("cost store not available")
	}
	if in.Agent != "" {
		s, err := cfg.Costs.AgentSummary(ctx, in.Agent)
		if err != nil {
			return nil, out, fmt.Errorf("query failed: %w", err)
		}
		out.Agents = []costSummary{{
			Agent:        in.Agent,
			InputTokens:  s.InputTokens,
			OutputTokens: s.OutputTokens,
			TotalTokens:  s.TotalTokens,
			TotalCostUSD: s.TotalCostUSD,
		}}
		return nil, out, nil
	}
	ws, err := cfg.Costs.WorkspaceSummary(ctx)
	if err != nil {
		return nil, out, fmt.Errorf("query failed: %w", err)
	}
	out.Workspace = &costSummary{
		InputTokens:  ws.InputTokens,
		OutputTokens: ws.OutputTokens,
		TotalTokens:  ws.TotalTokens,
		TotalCostUSD: ws.TotalCostUSD,
	}
	byAgent, err := cfg.Costs.SummaryByAgent(ctx)
	if err != nil {
		return nil, out, fmt.Errorf("query failed: %w", err)
	}
	for _, s := range byAgent {
		out.Agents = append(out.Agents, costSummary{
			Agent:        s.AgentID,
			InputTokens:  s.InputTokens,
			OutputTokens: s.OutputTokens,
			TotalTokens:  s.TotalTokens,
			TotalCostUSD: s.TotalCostUSD,
		})
	}
	return nil, out, nil
}
