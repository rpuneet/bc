package provider

// claudeFatalPatterns are Claude Code's refusals to serve a turn at all.
//
// Taken from the Claude Code CLI's own error strings (credit/auth/model
// refusals that leave the prompt up with nothing for the transcript tailer
// to observe). LineStartsWith is Claude's own marker so an agent discussing
// these strings is not reported broken (#3687).
var claudeFatalPatterns = []FailurePattern{
	{
		// "API Error: 401 Invalid API key · Please run /login"
		LineStartsWith: "api error:",
		Contains:       "invalid api key",
		Reason:         "claude has an invalid API key — run /login in the agent's terminal or fix ANTHROPIC_API_KEY",
	},
	{
		// "Not logged in · Please run /login"
		// "Not logged in. Run claude auth login to authenticate."
		LineStartsWith: "not logged in",
		Contains:       "login",
		Reason:         "claude is not logged in — run /login in the agent's terminal",
	},
	{
		// "Failed to authenticate. · Please run /login"
		// "Please run /login · …"
		LineStartsWith: "failed to authenticate",
		Contains:       "authenticate",
		Reason:         "claude failed to authenticate — run /login in the agent's terminal",
	},
	{
		// "Credit balance is too low"
		LineStartsWith: "credit balance is too low",
		Contains:       "credit balance is too low",
		Reason:         "claude's credit balance is too low — add credits or switch accounts",
	},
	{
		// "API Error: … usage limit reached …"
		LineStartsWith: "api error:",
		Contains:       "usage limit reached",
		Reason:         "claude's usage limit is exhausted — wait for reset or upgrade the plan",
	},
	{
		// "Claude Opus is not available with the Claude Pro plan. …"
		LineStartsWith: "claude opus is not available",
		Contains:       "not available",
		Reason:         "claude's model is unavailable on this plan — pick another model for this agent",
	},
}

// DetectFailure implements FailureDetector.
func (p *ClaudeProvider) DetectFailure(pane string) (string, bool) {
	return MatchFailure(pane, paneTailLines, claudeFatalPatterns)
}
