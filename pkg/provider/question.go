// question.go — recognizing a provider CLI that has stopped to ask its user
// something.
//
// Providers that report this through a hook are handled where state is derived
// (pkg/agent/waiting.go). This is for the ones that do not: cursor-agent has no
// hook that fires while it holds a prompt open, so the only place it says so is
// the screen (#3582).
//
// A sibling of failure.go rather than part of it. The two read the same
// terminal for opposite kinds of condition: a failure is terminal and wants
// reporting once, a question resolves the moment someone answers and has to be
// watched until it does.
package provider

import (
	"regexp"
	"strings"
)

// questionTailLines is how much of an agent's terminal a question detector
// reads.
//
// Shorter than paneTailLines, and the difference is the point: a CLI's failure
// message is prose that can be several lines above its prompt, while the chrome
// it draws to say it is waiting sits at the bottom of the screen. Reading
// further up only offers more of the agent's own output to trip over.
const questionTailLines = 20

// QuestionDetector is optionally implemented by providers whose CLI blocks on
// prompts it never reports.
//
// This is a low-confidence signal read from an agent's screen, so it is only
// consulted for agents that have already gone quiet, and implementations should
// prefer precision to recall. The two errors are not symmetric: missing a
// question costs the delay until the guardrail timeout notices the silence,
// while inventing one puts a working agent in front of a person who then finds
// nothing to answer — and a state that cries wolf gets ignored, which is the
// failure this whole capability exists to fix.
type QuestionDetector interface {
	// DetectQuestion examines recent terminal output, newest content last, and
	// returns a short description of what the CLI is waiting to be told.
	DetectQuestion(pane string) (question string, waiting bool)
}

// QuestionPattern maps a line of provider output to the question it implies.
//
// Literal fragments rather than regexes, for the same reason FailurePattern
// uses them: these are matched against text a terminal reflows and colors
// unpredictably, and a fragment short enough to survive that is also short
// enough to check by eye against the string the provider actually prints.
type QuestionPattern struct {
	// LineStartsWith is the gutter or marker the provider draws the prompt
	// with, matched against the line ignoring leading whitespace.
	//
	// It is what separates a CLI asking a question from an agent writing about
	// one. Empty matches any line and should be paired with AtPaneEnd.
	LineStartsWith string
	// Contains is the most distinctive short fragment of the prompt, matched
	// case-insensitively within the same line as the marker.
	Contains string
	// Question is what the user is told they are being asked, in mycel's voice
	// — enough to decide whether to go and answer it now.
	Question string
	// AtPaneEnd restricts the match to the last non-blank line of the pane.
	//
	// For a CLI that prints its prompt and waits, that line is the prompt, and
	// nothing the agent writes afterwards can be mistaken for one. It is the
	// only thing making the provider-independent shapes below safe to match at
	// all, since "(y/n)" appears in plenty of output that is not a prompt.
	AtPaneEnd bool
}

// commonQuestionPatterns are the prompt shapes that are not any one provider's,
// matched only as the last thing on the screen.
//
// A full-screen TUI always draws its own footer below the prompt, so these
// never fire for one — which is deliberate. They cover the CLI that writes a
// question to the terminal and blocks on a read, where the question really is
// the final line, and they stay quiet everywhere else.
var commonQuestionPatterns = []QuestionPattern{
	{Contains: "(y/n)", Question: "waiting for a yes/no answer", AtPaneEnd: true},
	{Contains: "[y/n]", Question: "waiting for a yes/no answer", AtPaneEnd: true},
	{Contains: "do you want to proceed?", Question: "waiting for permission to proceed", AtPaneEnd: true},
	{Contains: "press enter to continue", Question: "waiting for someone to press enter", AtPaneEnd: true},
}

// menuOption and menuCursor match a numbered choice menu: any of its options,
// and the one the CLI has highlighted.
//
// The number is what makes the cursor glyph usable. On its own it is just an
// input prompt — claude and cursor both draw one permanently — but pointing at
// "1." it only appears while a menu is open.
var (
	menuOption = regexp.MustCompile(`^[❯›»▸]?\s*\d+[.)]\s+\S`)
	menuCursor = regexp.MustCompile(`^[❯›»▸]\s*\d+[.)]\s+\S`)
)

// choiceMenuQuestion reports whether the screen ends inside an open numbered
// choice menu.
//
// Both halves are required: the last visible line has to be one of the options,
// so a menu the agent scrolled past cannot count, and one of the options has to
// carry the selection cursor, so a numbered list the agent merely printed
// cannot either.
func choiceMenuQuestion(lines []string) bool {
	end := lastNonBlankLine(lines)
	if end < 0 || !menuOption.MatchString(strings.TrimSpace(lines[end])) {
		return false
	}
	for _, line := range lines[:end+1] {
		if menuCursor.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// MatchCommonQuestion reports whether pane ends on one of the prompt shapes
// that belong to no provider in particular.
func MatchCommonQuestion(pane string) (string, bool) {
	if question, waiting := MatchQuestion(pane, questionTailLines, commonQuestionPatterns); waiting {
		return question, true
	}
	if choiceMenuQuestion(paneTail(StripANSI(pane), questionTailLines)) {
		return "waiting for a choice from a menu", true
	}
	return "", false
}

// MatchQuestion returns the question implied by the first pattern found in the
// tail of pane, or false when none match.
//
// Only the tail is searched: a pane holds scrollback, and a prompt someone
// answered an hour ago must not keep an agent flagged. tailLines of 0 or less
// searches the whole pane.
func MatchQuestion(pane string, tailLines int, patterns []QuestionPattern) (string, bool) {
	lines := paneTail(StripANSI(pane), tailLines)
	end := lastNonBlankLine(lines)
	for i, line := range lines {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		for _, p := range patterns {
			if p.Contains == "" {
				continue
			}
			if p.AtPaneEnd && i != end {
				continue
			}
			if p.LineStartsWith != "" && !strings.HasPrefix(trimmed, strings.ToLower(p.LineStartsWith)) {
				continue
			}
			if strings.Contains(trimmed, strings.ToLower(p.Contains)) {
				return p.Question, true
			}
		}
	}
	return "", false
}

// lastNonBlankLine returns the index of the last line with content, or -1 when
// there is none. Captured panes are padded to the terminal's height, so the
// last line of the capture is usually empty and the last line the user can see
// is not.
func lastNonBlankLine(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}
