// failure.go — recognising a provider CLI that is running but cannot work.
//
// A provider process can be perfectly alive and still refuse every turn: no API
// key, a spent quota, a model the account isn't entitled to. mycel used to
// report those agents as idle or working with an empty Live feed, because
// nothing they do reaches the daemon — there is nothing to report. The reason is
// on their terminal, in prose, and that is the only place it exists (#3512).
//
// So the terminal is treated as what it is: a last-resort activity source for
// fatal conditions. Providers declare the lines that mean "this agent cannot
// serve a turn", and the daemon matches them against recent pane output only
// when an agent has already gone quiet.
package provider

import (
	"regexp"
	"strings"
)

// ansiEscape matches the escape sequences a terminal UI paints its output with.
//
// Captured panes are not plain text: providers colour their own error messages,
// so the line that reads "Error: No API key found for amazon-bedrock" actually
// begins with a colour sequence. Anything anchoring to the start of a line has
// to see through that first.
//
// Covered: CSI sequences (colour, cursor movement, line clears), OSC sequences
// (hyperlinks and window titles, which terminate with BEL or ST), and the
// two-character escapes. An unterminated OSC is left alone rather than risking
// swallowing the newline that separates two lines.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]" +
	"|\x1b\\][^\x07\x1b\n]*(?:\x07|\x1b\\\\)" +
	"|\x1b[@-Z\\\\-_]")

// StripANSI removes terminal escape sequences from s, leaving its text and line
// structure intact.
func StripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	return ansiEscape.ReplaceAllString(s, "")
}

// paneTailLines is how much of an agent's terminal a detector reads.
//
// Enough to hold a wrapped error and the prompt line under it, short enough that
// a message from an earlier, recovered session has scrolled out of range.
const paneTailLines = 25

// FailureDetector is optionally implemented by providers whose CLI reports
// fatal, unrecoverable conditions to its terminal and nowhere else.
//
// This is deliberately the lowest-confidence signal mycel has, so it is only
// consulted for agents that have already produced no activity for minutes.
// Matching prose is unpleasant, but a CLI that prints "No API key found" and
// then sits there offers nothing else — and leaving the user with a silent feed
// and no reason is worse than reading its screen.
type FailureDetector interface {
	// DetectFailure examines recent terminal output, newest content last, and
	// returns a short reason when the CLI cannot serve a turn.
	//
	// Implementations must only report conditions that will not resolve on
	// their own: a missing credential, an exhausted quota, a model the account
	// cannot use. Anything the agent can recover from by itself — a rate limit
	// it will retry, a tool that failed once — is not a failure here, because
	// the cost of a false positive is an agent reported broken while it works.
	DetectFailure(pane string) (reason string, failed bool)
}

// FailurePattern maps a line of provider output to the reason it implies.
//
// Literal fragments rather than regexes: these are matched against terminal
// text that providers reflow and colour unpredictably, and a fragment short
// enough to survive that is also short enough to read here and check against the
// real message.
type FailurePattern struct {
	// LineStartsWith is the provider's own error marker — the beginning of the
	// line it prints the failure on, ignoring leading whitespace (e.g. "error:",
	// or the glyph a CLI prefixes warnings with).
	//
	// It is what separates a provider reporting a failure from an agent
	// discussing one. An agent reading this very file writes lines containing
	// "no api key found for"; only the CLI writes a line that starts with
	// "Error:" and contains it. Empty matches any line, which should be rare —
	// prefer a marker, since a false positive tells the user a working agent is
	// broken.
	LineStartsWith string
	// Contains is the most distinctive short fragment of the message, matched
	// case-insensitively within the same line as the marker.
	Contains string
	// Reason is what the user is told, in mycel's voice rather than the
	// provider's: what is wrong and, where it fits, what to do about it.
	Reason string
}

// MatchFailure returns the reason for the first pattern found in the tail of
// pane, or false when none match.
//
// Only the tail is searched, because a pane holds scrollback: an error from an
// hour ago followed by a working session must not mark a healthy agent as
// broken. tailLines of 0 or less searches the whole pane.
func MatchFailure(pane string, tailLines int, patterns []FailurePattern) (string, bool) {
	for _, line := range paneTail(StripANSI(pane), tailLines) {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		for _, p := range patterns {
			if p.Contains == "" {
				continue
			}
			if p.LineStartsWith != "" && !strings.HasPrefix(trimmed, strings.ToLower(p.LineStartsWith)) {
				continue
			}
			if strings.Contains(trimmed, strings.ToLower(p.Contains)) {
				return p.Reason, true
			}
		}
	}
	return "", false
}

// paneTail returns the last n lines of s, in order.
func paneTail(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if n <= 0 || len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
