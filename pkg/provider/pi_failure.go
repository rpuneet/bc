package provider

// piFatalPatterns are pi's refusals to serve a turn at all.
//
// Every one of these was taken verbatim from a pi agent that mycel was
// reporting as idle or working with an empty Live feed (#3512). pi prints the
// message once, leaves the prompt up, and writes no session transcript — so
// there is nothing for the tailer to find and no other trace of why.
var piFatalPatterns = []FailurePattern{
	{
		// "Error: No API key found for amazon-bedrock."
		LineStartsWith: "error:",
		Contains:       "no api key found for",
		Reason:         "pi has no API key for its configured provider — run /login in the agent's terminal",
	},
	{
		// "Error: 404 The model `qwen/qwen3-32b` does not exist or you do not
		// have access to it."
		LineStartsWith: "error:",
		Contains:       "do not have access to it",
		Reason:         "pi's model is unavailable to this account — pick another model for this agent",
	},
}

// DetectFailure implements FailureDetector.
func (p *PiProvider) DetectFailure(pane string) (string, bool) {
	return MatchFailure(pane, paneTailLines, piFatalPatterns)
}
