package provider

// Curated per-provider CLI command lists (the CommandLister capability).
//
// These drive the "Available commands" surface on the provider detail page.
// They are copyable references, not executed from the browser: several are
// interactive (login/auth open a device flow or prompt) and cannot be
// meaningfully streamed to a web UI. Each list is drawn from the provider
// CLI's own documented subcommands/flags — see each provider file's header
// comment for the source of truth. Providers whose CLI genuinely lacks a
// category (e.g. OpenClaw has no --model flag) simply omit it rather than
// showing a dead control.
//
// Claude's Commands() lives in claude.go alongside its other richer wiring.

// Commands returns the curated CLI command list for OpenAI Codex.
func (p *CodexProvider) Commands() []Command {
	return []Command{
		{Name: "login", Command: "codex login", Description: "Sign in to OpenAI (device flow)"},
		{Name: "login status", Command: "codex login status", Description: "Show current auth status"},
		{Name: "logout", Command: "codex logout", Description: "Sign out"},
		{Name: "exec", Command: "codex exec <prompt>", Description: "Run one non-interactive task", Args: "<prompt>"},
		{Name: "resume", Command: "codex resume", Description: "Resume a previous session"},
		{Name: "mcp list", Command: "codex mcp list", Description: "List configured MCP servers"},
		{Name: "mcp add", Command: "codex mcp add <name> -- <command>", Description: "Add an MCP server", Args: "<name> -- <command>"},
		{Name: "version", Command: "codex --version", Description: "Show version"},
	}
}

// Commands returns the curated CLI command list for the pi coding agent.
// pi is flag-driven (no subcommands): model routing and session control are
// all flags on the base invocation.
func (p *PiProvider) Commands() []Command {
	return []Command{
		{Name: "run", Command: "pi", Description: "Start the pi agent"},
		{Name: "model", Command: "pi --model <provider/model>", Description: "Run with a specific model", Args: "<provider/model>"},
		{Name: "continue", Command: "pi --continue", Description: "Continue the previous session"},
		{Name: "resume", Command: "pi --resume", Description: "Pick a session to resume"},
		{Name: "session", Command: "pi --session <id>", Description: "Resume a specific session", Args: "<session-id>"},
		{Name: "version", Command: "pi --version", Description: "Show version"},
	}
}

// Commands returns the curated CLI command list for the Antigravity CLI (agy).
func (p *AgyProvider) Commands() []Command {
	return []Command{
		{Name: "run", Command: "agy", Description: "Start the Antigravity agent"},
		{Name: "models", Command: "agy models", Description: "List available Gemini models"},
		{Name: "model", Command: "agy --model '<model>'", Description: "Run with a specific model", Args: "<model>"},
		{Name: "continue", Command: "agy --continue", Description: "Continue the previous conversation"},
		{Name: "conversation", Command: "agy --conversation <uuid>", Description: "Resume a specific conversation", Args: "<uuid>"},
		{Name: "version", Command: "agy --version", Description: "Show version"},
	}
}

// Commands returns the curated CLI command list for Cursor Agent.
func (p *CursorProvider) Commands() []Command {
	return []Command{
		{Name: "login", Command: "cursor-agent login", Description: "Sign in to Cursor"},
		{Name: "logout", Command: "cursor-agent logout", Description: "Sign out"},
		{Name: "status", Command: "cursor-agent status", Description: "Show auth and account status"},
		{Name: "list models", Command: "cursor-agent --list-models", Description: "List available models"},
		{Name: "model", Command: "cursor-agent --model <model>", Description: "Run with a specific model", Args: "<model>"},
		{Name: "resume", Command: "cursor-agent --resume <id>", Description: "Resume a session", Args: "<session-id>"},
		{Name: "version", Command: "cursor-agent --version", Description: "Show version"},
	}
}

// Commands returns the curated CLI command list for OpenClaw.
// OpenClaw drives model selection in the TUI or via per-agent routing, so no
// --model flag is surfaced (see the provider header comment).
func (p *OpenclawProvider) Commands() []Command {
	return []Command{
		{Name: "tui", Command: "openclaw tui --local", Description: "Start the local TUI agent"},
		{Name: "session", Command: "openclaw tui --local --session <key>", Description: "Reattach to a session", Args: "<session-key>"},
		{Name: "agents add", Command: "openclaw agents add --model <provider/model>", Description: "Add a routed agent with a model", Args: "<provider/model>"},
		{Name: "version", Command: "openclaw --version", Description: "Show version"},
		{Name: "help", Command: "openclaw --help", Description: "Show help"},
	}
}
