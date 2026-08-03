package provider

// agyFatalPatterns are agy's refusals to serve a turn at all.
//
// Taken verbatim from an agy agent mycel was reporting as starting while it had
// answered every prompt with a quota error for hours (#3512). A quota that
// resets in six days is not something the agent recovers from on its own, so it
// belongs here; a momentary rate limit would not.
var agyFatalPatterns = []FailurePattern{
	{
		// "⚠ Individual quota reached. Please upgrade your subscription to
		// increase your limits. Resets in 156h34m52s."
		LineStartsWith: "⚠",
		Contains:       "quota reached",
		Reason:         "agy's quota is exhausted — it resets on its own, or raise the limit on the account",
	},
}

// DetectFailure implements FailureDetector.
func (p *AgyProvider) DetectFailure(pane string) (string, bool) {
	return MatchFailure(pane, paneTailLines, agyFatalPatterns)
}
