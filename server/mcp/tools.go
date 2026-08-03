package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/avatar"
	"github.com/rpuneet/mycel/pkg/home"
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
	maxTaskLen    = 1024      // initial task line on spawn_agent
	maxPathLen    = 4 * 1024  // file_path on send_file (typical PATH_MAX)
	// maxTemplateLen bounds a template name on spawn_agent. Names are short
	// identifiers, so this only exists to reject junk before it reaches the store.
	maxTemplateLen = 256
	maxFileSize    = 50 * 1024 * 1024
	maxReadLimit   = 200 // read_channel page size cap
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
		Description: "Returns the current agent's identity: name, display_name, role, state, provider/model, its AgentCharacter avatar_url, and a Slack hint (username + icon_url) for posting as itself via chat.postMessage",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ emptyIn) (*sdk.CallToolResult, whoamiOut, error) {
		return nil, whoami(cfg, agentName), nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_agents",
		Description: "List all agents with their status and role",
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
		Name:        "query_costs",
		Description: "Query token usage and cost, for one agent or the whole fleet",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in queryCostsIn) (*sdk.CallToolResult, queryCostsOut, error) {
		return queryCosts(ctx, cfg, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a new child agent under this agent. This agent's role must be permitted to create the requested role (the same role-hierarchy check the daemon enforces for --parent-based creation); an unpermitted role is rejected.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in spawnAgentIn) (*sdk.CallToolResult, spawnAgentOut, error) {
		return spawnAgent(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "send_to_agent",
		Description: "Send a message or task instruction to another agent's session, as this agent",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in sendToAgentIn) (*sdk.CallToolResult, sendToAgentOut, error) {
		return sendToAgent(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "stop_agent",
		Description: "Stop a running agent. Restricted to the root agent or an ancestor (direct or indirect parent) of the target — an agent cannot stop a peer or an unrelated agent.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in stopAgentIn) (*sdk.CallToolResult, stopAgentOut, error) {
		return stopAgent(ctx, cfg, agentName, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_children",
		Description: "List agents directly spawned by this agent",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ emptyIn) (*sdk.CallToolResult, listChildrenOut, error) {
		return listChildren(cfg, agentName)
	})
}

type emptyIn struct{}

// ─── whoami ─────────────────────────────────────────────────────────────────

// slackHint tells an agent exactly how to post to Slack as itself — its own
// AgentCharacter avatar and name — via chat.postMessage directly with the bot
// token, rather than routing through the gateway. Requires the bot token to
// hold the chat:write.customize scope for username/icon_url to take effect.
type slackHint struct {
	Method   string `json:"method" jsonschema:"the Slack Web API method to call"`
	Username string `json:"username" jsonschema:"pass as chat.postMessage username"`
	IconURL  string `json:"icon_url,omitempty" jsonschema:"pass as chat.postMessage icon_url (this agent's avatar); empty until a public avatar base is configured"`
	Scope    string `json:"scope" jsonschema:"OAuth scope the bot token must hold for username/icon_url to apply"`
	Note     string `json:"note" jsonschema:"guidance on posting as this agent"`
}

type whoamiOut struct {
	Slack       *slackHint `json:"slack,omitempty" jsonschema:"how to post to Slack as this agent"`
	Agent       string     `json:"agent" jsonschema:"the agent's name"`
	DisplayName string     `json:"display_name" jsonschema:"human-friendly name for display (e.g. Slack)"`
	Workspace   string     `json:"workspace,omitempty" jsonschema:"the workspace name"`
	Role        string     `json:"role,omitempty" jsonschema:"the agent's role"`
	State       string     `json:"state,omitempty" jsonschema:"the agent's lifecycle state"`
	Task        string     `json:"task,omitempty" jsonschema:"the agent's current task line"`
	Provider    string     `json:"provider,omitempty" jsonschema:"the AI provider/tool backing this agent (e.g. claude)"`
	Model       string     `json:"model,omitempty" jsonschema:"the provider model, if pinned (empty means provider default)"`
	AvatarURL   string     `json:"avatar_url" jsonschema:"URL of this agent's AgentCharacter avatar (PNG). Public when a public avatar base is configured, else the local daemon endpoint"`
	AvatarLocal string     `json:"avatar_local_url" jsonschema:"the daemon-local avatar endpoint, always reachable from the mycel UI"`
}

// displayName humanizes an agent name for display: "zen-zebra" → "Zen Zebra".
func displayName(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	if len(words) == 0 {
		return name
	}
	return strings.Join(words, " ")
}

func whoami(cfg Config, agentName string) whoamiOut {
	local := localAvatarURL(cfg, agentName)
	public := avatar.PublicURL(agentName)

	out := whoamiOut{
		Agent:       agentName,
		DisplayName: displayName(agentName),
		Workspace:   cfg.Home.Name(),
		AvatarLocal: local,
	}
	// avatar_url prefers the genuinely public URL (Slack can fetch it); it
	// falls back to the daemon-local endpoint honestly when no public base is
	// configured — the local one works in the mycel UI but not from Slack.
	if public != "" {
		out.AvatarURL = public
	} else {
		out.AvatarURL = local
	}
	if cfg.Agents != nil {
		if ag := cfg.Agents.GetAgent(agentName); ag != nil {
			out.Role = string(ag.Role)
			out.State = string(ag.State)
			out.Task = ag.Task
			out.Provider = ag.Tool
			out.Model = ag.Model
		}
	}

	hint := &slackHint{
		Method:   "chat.postMessage",
		Username: agentName,
		IconURL:  public, // only a public URL is usable as Slack icon_url
		Scope:    "chat:write.customize",
		Note:     "Post directly with the bot token: chat.postMessage with username and icon_url set to these values. If icon_url is empty, a public avatar base is not yet configured — post with username only.",
	}
	out.Slack = hint
	return out
}

// localAvatarURL builds the daemon-local avatar endpoint for an agent, using
// the address `mycel up` recorded in ~/.mycel/run/daemon.addr so it points at
// the port the daemon actually listens on. Reachable from the mycel UI; not
// from Slack (loopback), which is why avatar_url prefers the public URL.
func localAvatarURL(cfg Config, agentName string) string {
	base := "http://127.0.0.1:9374"
	if p, err := home.DaemonAddrPath(); err == nil {
		// p is a fixed daemon-owned path (~/.mycel/run/daemon.addr), never
		// user input — no path-traversal surface.
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // trusted daemon path
			if s := strings.TrimSpace(string(b)); s != "" {
				base = s
			}
		}
	}
	return strings.TrimRight(base, "/") + "/api/agents/" + agentName + "/avatar.png"
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
// stays under the repo root or /tmp, preventing the tool from
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
	repoRoot := cfg.Home.RootDir
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil {
		repoRoot = r
	}
	if !underDir(resolved, repoRoot) && !underDir(resolved, "/tmp") && !underDir(resolved, "/private/tmp") {
		return "", fmt.Errorf("file path %q is outside repo root %q", resolved, repoRoot)
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
	h, err := cfg.Costs.TotalSummary(ctx)
	if err != nil {
		return nil, out, fmt.Errorf("query failed: %w", err)
	}
	out.Workspace = &costSummary{
		InputTokens:  h.InputTokens,
		OutputTokens: h.OutputTokens,
		TotalTokens:  h.TotalTokens,
		TotalCostUSD: h.TotalCostUSD,
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

// ─── spawn_agent ────────────────────────────────────────────────────────────
//
// Generalizes the root-agent/merge-queue pattern (see the workspace root
// CLAUDE.md's create_agent MCP tool) so ANY agent — not just a hardcoded
// root — can spawn children, subject to the same role-hierarchy check the
// daemon already enforces for --parent-based creation
// (Manager.SpawnAgentWithOptions → agent.CanCreateRole via ParentID).

type spawnAgentIn struct {
	Name     string `json:"name,omitempty" jsonschema:"name for the new agent (letters, numbers, dash, underscore); omit to auto-generate a unique name"`
	Role     string `json:"role" jsonschema:"role for the new agent; this agent's role must be permitted to create it (role hierarchy), or the spawn is rejected"`
	Task     string `json:"task,omitempty" jsonschema:"one-line initial task description recorded on the new agent (shown in the TUI/web UI)"`
	Provider string `json:"provider,omitempty" jsonschema:"AI provider/tool for the new agent (e.g. claude); empty uses the fleet default"`
	Repo     string `json:"repo,omitempty" jsonschema:"absolute path of the git repo to bind the new agent to; empty binds it to this agent's own repo"`
	Template string `json:"template,omitempty" jsonschema:"template to spawn the new agent from; recorded on the agent so the template's guardrails (max cost, stuck timeout) are enforced against it. Empty inherits this agent's own template"`
}

type spawnAgentOut struct {
	Agent    string `json:"agent"`
	Role     string `json:"role"`
	State    string `json:"state"`
	ParentID string `json:"parent_id,omitempty"`
	// Template reports what the child was actually spawned with, which is not
	// always what was asked for: an omitted template is inherited from the
	// caller. Reported so guardrail coverage is visible rather than assumed.
	Template string `json:"template,omitempty"`
}

func spawnAgent(ctx context.Context, cfg Config, agentName string, in spawnAgentIn) (*sdk.CallToolResult, spawnAgentOut, error) {
	var out spawnAgentOut
	if in.Role == "" {
		return nil, out, fmt.Errorf("role is required")
	}
	if len(in.Role) > maxRoleLen {
		return nil, out, fmt.Errorf("role too long: %d bytes (max %d)", len(in.Role), maxRoleLen)
	}
	if in.Name != "" && !agent.IsValidAgentName(in.Name) {
		return nil, out, fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", in.Name, agent.MaxAgentNameLength)
	}
	if len(in.Task) > maxTaskLen {
		return nil, out, fmt.Errorf("task too long: %d bytes (max %d)", len(in.Task), maxTaskLen)
	}
	if len(in.Repo) > maxPathLen {
		return nil, out, fmt.Errorf("repo too long: %d bytes (max %d)", len(in.Repo), maxPathLen)
	}
	if len(in.Template) > maxTemplateLen {
		return nil, out, fmt.Errorf("template too long: %d bytes (max %d)", len(in.Template), maxTemplateLen)
	}
	if cfg.AgentSvc == nil || cfg.Agents == nil {
		return nil, out, fmt.Errorf("agent orchestration not available")
	}

	// Default to the caller's own repo so a spawned child shares the caller's
	// worktree root unless a different repo is explicitly named.
	//
	// Inherit the caller's template for a different reason: guardrails
	// (MaxCostUSD, StuckTimeoutMin) are enforced per template, and an agent with
	// no template recorded is exempt from them entirely. Without inheritance a
	// child would be unguarded whenever the field was simply left out — which is
	// every existing caller, since the field did not exist until now.
	repo, template := in.Repo, in.Template
	if repo == "" || template == "" {
		if caller := cfg.Agents.GetAgent(agentName); caller != nil {
			if repo == "" {
				repo = caller.Repo
			}
			if template == "" {
				template = caller.Template
			}
		}
	}

	// Parent is always the authenticated caller — never client-supplied —
	// so AgentService.Create → SpawnAgentWithOptions enforces
	// agent.CanCreateRole(caller.Role, requested role) for us. A caller
	// whose role isn't permitted to create the requested role gets a
	// clear error and no agent is spawned.
	child, err := cfg.AgentSvc.Create(ctx, agent.CreateOptions{
		Name:     in.Name,
		Role:     agent.Role(in.Role),
		Tool:     in.Provider,
		Repo:     repo,
		Parent:   agentName,
		Template: template,
	})
	if err != nil {
		return nil, out, fmt.Errorf("spawn failed: %w", err)
	}

	out.Agent = child.Name
	out.Role = string(child.Role)
	out.State = string(child.State)
	out.ParentID = child.ParentID
	out.Template = child.Template

	if in.Task != "" {
		if taskErr := cfg.Agents.SetAgentTask(ctx, child.Name, in.Task); taskErr != nil {
			log.Warn("spawn_agent: failed to set initial task", "agent", child.Name, "error", taskErr)
		}
	}
	return nil, out, nil
}

// ─── send_to_agent ──────────────────────────────────────────────────────────

type sendToAgentIn struct {
	Agent   string `json:"agent" jsonschema:"name of the target agent"`
	Message string `json:"message" jsonschema:"message or task instruction text to send"`
}

type sendToAgentOut struct {
	Agent string `json:"agent"`
}

func sendToAgent(ctx context.Context, cfg Config, agentName string, in sendToAgentIn) (*sdk.CallToolResult, sendToAgentOut, error) {
	out := sendToAgentOut{Agent: in.Agent}
	if in.Agent == "" || in.Message == "" {
		return nil, out, fmt.Errorf("agent and message are required")
	}
	if len(in.Agent) > agent.MaxAgentNameLength {
		return nil, out, fmt.Errorf("agent name too long: %d bytes (max %d)", len(in.Agent), agent.MaxAgentNameLength)
	}
	if len(in.Message) > maxMessageLen {
		return nil, out, fmt.Errorf("message too long: %d bytes (max %d)", len(in.Message), maxMessageLen)
	}
	if cfg.AgentSvc == nil {
		return nil, out, fmt.Errorf("agent orchestration not available")
	}
	log.Debug("mcp: send_to_agent", "from", agentName, "to", in.Agent)
	if err := cfg.AgentSvc.Send(ctx, in.Agent, in.Message); err != nil {
		return nil, out, fmt.Errorf("send failed: %w", err)
	}
	return nil, out, nil
}

// ─── stop_agent ─────────────────────────────────────────────────────────────

type stopAgentIn struct {
	Agent string `json:"agent" jsonschema:"name of the agent to stop"`
}

type stopAgentOut struct {
	Agent string `json:"agent"`
}

func stopAgent(ctx context.Context, cfg Config, agentName string, in stopAgentIn) (*sdk.CallToolResult, stopAgentOut, error) {
	out := stopAgentOut(in)
	if in.Agent == "" {
		return nil, out, fmt.Errorf("agent is required")
	}
	if len(in.Agent) > agent.MaxAgentNameLength {
		return nil, out, fmt.Errorf("agent name too long: %d bytes (max %d)", len(in.Agent), agent.MaxAgentNameLength)
	}
	if cfg.AgentSvc == nil || cfg.Agents == nil {
		return nil, out, fmt.Errorf("agent orchestration not available")
	}
	if in.Agent == agentName {
		return nil, out, fmt.Errorf("cannot stop yourself via stop_agent")
	}
	if !canStopAgent(cfg, agentName, in.Agent) {
		return nil, out, fmt.Errorf("agent %q is not permitted to stop %q: only the root agent or an ancestor of the target may stop it", agentName, in.Agent)
	}
	if err := cfg.AgentSvc.Stop(ctx, in.Agent); err != nil {
		return nil, out, fmt.Errorf("stop failed: %w", err)
	}
	return nil, out, nil
}

// canStopAgent enforces the permission model for stop_agent: there is no
// per-agent "can_stop" capability today (pkg/agent's Permission enum is
// unwired for MCP callers), so this gates on the same parent/child
// relationship the daemon already tracks via Agent.ParentID — the root
// agent (singleton, workspace owner) may stop anyone; any other agent may
// only stop its own descendants (children, grandchildren, ...), never a
// peer or an unrelated agent.
func canStopAgent(cfg Config, caller, target string) bool {
	if c := cfg.Agents.GetAgent(caller); c != nil && c.Role == agent.RoleRoot {
		return true
	}
	for _, d := range cfg.Agents.ListDescendants(caller) {
		if d.Name == target {
			return true
		}
	}
	return false
}

// ─── list_children ──────────────────────────────────────────────────────────

type listChildrenOut struct {
	Children []agentInfo `json:"children"`
}

func listChildren(cfg Config, agentName string) (*sdk.CallToolResult, listChildrenOut, error) {
	var out listChildrenOut
	if cfg.Agents == nil {
		return nil, out, fmt.Errorf("agent manager not available")
	}
	for _, c := range cfg.Agents.ListChildren(agentName) {
		out.Children = append(out.Children, agentInfo{
			Name:  c.Name,
			Role:  string(c.Role),
			State: string(c.State),
			Task:  c.Task,
		})
	}
	return nil, out, nil
}
