package provider

// cursorFatalPatterns are Cursor Agent's refusals to serve a turn at all.
//
// Taken from cursor-agent's own auth/error lines (missing login, invalid
// CURSOR_API_KEY, failed model load). LineStartsWith is the CLI's marker so
// an agent discussing these strings is not reported broken (#3687).
var cursorFatalPatterns = []FailurePattern{
	{
		// "Error: Authentication required. Please run 'agent login' first, or set CURSOR_API_KEY environment variable."
		LineStartsWith: "error:",
		Contains:       "authentication required",
		Reason:         "cursor is not authenticated — run agent login or set CURSOR_API_KEY",
	},
	{
		// "Authentication required to use Cursor Agent. Please run 'agent login' to authenticate."
		LineStartsWith: "authentication required",
		Contains:       "login",
		Reason:         "cursor is not authenticated — run agent login or set CURSOR_API_KEY",
	},
	{
		// "Authentication failed: your Cursor credentials or API key are invalid or expired."
		LineStartsWith: "authentication failed:",
		Contains:       "api key",
		Reason:         "cursor credentials or API key are invalid — run agent login or fix CURSOR_API_KEY",
	},
	{
		// "Not logged in. Run `agent login` first."
		LineStartsWith: "not logged in",
		Contains:       "login",
		Reason:         "cursor is not logged in — run agent login in the agent's terminal",
	},
	{
		// "Failed to load models: …" after an auth/entitlement failure
		LineStartsWith: "failed to load models:",
		Contains:       "failed to load models",
		Reason:         "cursor could not load models — check auth and model access for this account",
	},
}

// DetectFailure implements FailureDetector.
func (p *CursorProvider) DetectFailure(pane string) (string, bool) {
	return MatchFailure(pane, paneTailLines, cursorFatalPatterns)
}
