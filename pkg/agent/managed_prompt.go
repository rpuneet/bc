package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
)

// Markers for the mycel-owned prompt section. Rewritten idempotently on
// every spawn/restart so appends never stack (#3648).
const (
	managedPromptStart = "<!-- mycel-managed:start -->"
	managedPromptEnd   = "<!-- mycel-managed:end -->"
)

// ChannelLister returns notification channels an agent is subscribed to.
// Implemented by notify.Store; optional on Manager (nil → omit subscriptions).
type ChannelLister interface {
	ChannelsForAgent(ctx context.Context, agent string) ([]string, error)
}

// SetChannelLister wires the notification subscription source used when
// syncing the mycel-managed prompt block.
func (m *Manager) SetChannelLister(l ChannelLister) {
	m.mu.Lock()
	m.channelLister = l
	m.mu.Unlock()
}

// ManagedPromptInput is everything needed to render the mycel-managed block.
type ManagedPromptInput struct {
	AgentName            string
	Role                 string
	InjectedInstructions string
	MCPServers           []string
	SecretEnvKeys        []string
	Apps                 map[string]app.InstanceConfig
	Subscriptions        []string // channel keys, e.g. slack:general, gmail:*
}

// buildManagedPromptBlock renders the marked mycel-managed section.
func buildManagedPromptBlock(in ManagedPromptInput) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(managedPromptStart)
	sb.WriteString("\n")
	sb.WriteString("## mycel context\n\n")
	sb.WriteString("_This section is managed by mycel — rewritten on every spawn/restart. Do not edit by hand._\n\n")

	sb.WriteString("### Identity\n")
	if in.AgentName != "" {
		sb.WriteString(fmt.Sprintf("- Agent: `%s` (also `MYCEL_AGENT_ID`)\n", in.AgentName))
	}
	if in.Role != "" {
		sb.WriteString(fmt.Sprintf("- Role: `%s`\n", in.Role))
	}
	sb.WriteString("\n")

	if text := strings.TrimSpace(in.InjectedInstructions); text != "" {
		sb.WriteString("### Instructions\n\n")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	sb.WriteString("### Available resources\n")
	sb.WriteString("MCP servers: " + summarizeNames(in.MCPServers) + "\n")
	sb.WriteString("Credential env vars: " + summarizeNames(in.SecretEnvKeys) + "\n\n")

	if apps := connectedAppSummary(in.Apps); apps != "" {
		sb.WriteString("### Connected apps\n")
		sb.WriteString(apps)
		sb.WriteString("\n")
	}

	if creds := strings.TrimSpace(appPromptInstructions(in.Apps)); creds != "" {
		// appPromptInstructions already includes "## Platform Credentials" header.
		sb.WriteString(strings.TrimPrefix(creds, "\n"))
		sb.WriteString("\n")
	}

	sb.WriteString("### Notification subscriptions\n")
	if len(in.Subscriptions) == 0 {
		sb.WriteString("none\n")
	} else {
		subs := append([]string(nil), in.Subscriptions...)
		sort.Strings(subs)
		for _, ch := range subs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", ch))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(managedPromptEnd)
	sb.WriteString("\n")
	return sb.String()
}

// connectedAppSummary lists enabled app instance names (platform labels).
func connectedAppSummary(apps map[string]app.InstanceConfig) string {
	if len(apps) == 0 {
		return ""
	}
	var lines []string
	for name, ic := range apps {
		if !ic.Enabled {
			continue
		}
		label := ic.App
		if plugin, ok := app.Get(ic.App); ok {
			label = plugin.Describe().Label
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s)", name, label))
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// syncManagedPromptFile rewrites the mycel-managed section in promptFile.
// User/role content outside the markers is preserved. Legacy un-marked
// "## mycel instructions" / "## Platform Credentials" appends are stripped
// so repeated restarts cannot stack blocks.
func syncManagedPromptFile(promptFile, managedBlock string) error {
	cleaned := filepath.Clean(promptFile)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("refusing to write managed prompt to traversal path %q", promptFile)
	}

	existing := ""
	data, err := os.ReadFile(cleaned) //nolint:gosec // controlled agent worktree path
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read prompt file: %w", err)
		}
	} else {
		existing = string(data)
	}

	base := stripManagedSections(existing)
	base = strings.TrimRight(base, "\n")
	var out strings.Builder
	if base != "" {
		out.WriteString(base)
		out.WriteString("\n")
	}
	out.WriteString(managedBlock)

	if err := os.MkdirAll(filepath.Dir(cleaned), 0o750); err != nil {
		return fmt.Errorf("mkdir prompt dir: %w", err)
	}
	if err := os.WriteFile(cleaned, []byte(out.String()), 0o600); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}
	return nil
}

// stripManagedSections removes marked mycel blocks and legacy appended headers.
func stripManagedSections(content string) string {
	for {
		start := strings.Index(content, managedPromptStart)
		end := strings.Index(content, managedPromptEnd)
		if start >= 0 && end > start {
			end += len(managedPromptEnd)
			// Drop a leading newline left behind the block.
			if start > 0 && content[start-1] == '\n' {
				start--
			}
			content = content[:start] + content[end:]
			continue
		}
		break
	}

	// Strip legacy append-only blocks that predate markers (#3648).
	for {
		idx := lastLegacyManagedIndex(content)
		if idx < 0 {
			break
		}
		content = content[:idx]
	}
	return content
}

func lastLegacyManagedIndex(content string) int {
	candidates := []string{
		"\n## mycel instructions\n",
		"\n## Platform Credentials\n",
		"## mycel instructions\n", // file-leading
		"## Platform Credentials\n",
	}
	best := -1
	for _, c := range candidates {
		if i := strings.LastIndex(content, c); i > best {
			best = i
		}
	}
	return best
}

// syncAgentManagedPrompt builds and writes the managed block for one agent.
func syncAgentManagedPrompt(
	ctx context.Context,
	promptFile string,
	cfg *home.Config,
	apps map[string]app.InstanceConfig,
	agentName, role string,
	mcpServers, secretEnvKeys, subscriptions []string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	injected := ""
	if cfg != nil {
		injected = cfg.InjectedInstructions
	}
	// Skip writing an empty managed block when there is nothing useful —
	// but still strip legacy/managed sections so restarts clean up.
	block := buildManagedPromptBlock(ManagedPromptInput{
		AgentName:            agentName,
		Role:                 role,
		InjectedInstructions: injected,
		MCPServers:           mcpServers,
		SecretEnvKeys:        secretEnvKeys,
		Apps:                 apps,
		Subscriptions:        subscriptions,
	})
	return syncManagedPromptFile(promptFile, block)
}

// syncManagedPromptForAgent is the spawn/restart entry point used by
// createAgent / startAgent. Failures are logged by callers.
func (m *Manager) syncManagedPromptForAgent(
	ctx context.Context,
	wtDir, toolName, agentName, role string,
	mcpServers, secretEnvKeys []string,
) error {
	var subs []string
	m.mu.RLock()
	lister := m.channelLister
	apps := m.appsConfig
	cfg := m.wsConfig
	m.mu.RUnlock()
	if lister != nil {
		channels, err := lister.ChannelsForAgent(ctx, agentName)
		if err != nil {
			log.Warn("managed prompt: list subscriptions failed", "agent", agentName, "error", err)
		} else {
			subs = channels
		}
	}
	return syncAgentManagedPrompt(
		ctx,
		injectedPromptFile(wtDir, toolName),
		cfg,
		apps,
		agentName,
		role,
		mcpServers,
		secretEnvKeys,
		subs,
	)
}
