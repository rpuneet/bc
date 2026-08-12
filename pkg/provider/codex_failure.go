package provider

// codexFatalPatterns are Codex CLI's refusals to serve a turn at all.
//
// Taken from the Codex binary's own auth/quota/access messages. Codex often
// prints the failure as the start of the line (no "Error:" prefix), so the
// marker is the beginning of that message — an agent discussing the same
// prose under a tool bullet will not match (#3687).
var codexFatalPatterns = []FailurePattern{
	{
		// "no Codex credentials were found"
		// "Run codex login or provide an API key through a supported auth env var."
		LineStartsWith: "no codex credentials were found",
		Contains:       "no codex credentials were found",
		Reason:         "codex has no credentials — run codex login or set OPENAI_API_KEY",
	},
	{
		// "Not signed in. Please run 'codex login' to sign in with ChatGPT, …"
		LineStartsWith: "not signed in",
		Contains:       "codex login",
		Reason:         "codex is not signed in — run codex login in the agent's terminal",
	},
	{
		// "You're out of credits. Your workspace is out of credits. …"
		LineStartsWith: "you're out of credits",
		Contains:       "out of credits",
		Reason:         "codex is out of credits — add credits or switch workspaces",
	},
	{
		// "Usage limit reached. You've reached your usage limit. …"
		LineStartsWith: "usage limit reached",
		Contains:       "usage limit",
		Reason:         "codex's usage limit is exhausted — wait for reset or raise the limit",
	},
	{
		// "You do not have access to Codex"
		LineStartsWith: "you do not have access to codex",
		Contains:       "do not have access",
		Reason:         "this account cannot use Codex — switch accounts or ask a workspace admin",
	},
	{
		// "Unknown model `…` for spawn_agent. Available models: …"
		LineStartsWith: "unknown model",
		Contains:       "unknown model",
		Reason:         "codex's model is unavailable — pick another model for this agent",
	},
}

// DetectFailure implements FailureDetector.
func (p *CodexProvider) DetectFailure(pane string) (string, bool) {
	return MatchFailure(pane, paneTailLines, codexFatalPatterns)
}
