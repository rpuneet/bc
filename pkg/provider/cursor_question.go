package provider

import "strings"

// cursor-agent's "I am waiting for you" chrome.
//
// cursor has no hook for this and is not shy about it: its binary carries an
// explicit translation table from claude's hook vocabulary to its own, and both
// of the relevant entries are null —
//
//	{PreToolUse: preToolUse, PermissionRequest: null, PostToolUse: postToolUse,
//	 UserPromptSubmit: beforeSubmitPrompt, Stop: stop, …, Notification: null}
//
// None of its twenty-one events fire while it holds a prompt open, so a cursor
// agent waiting on a person reports exactly what a cursor agent thinking hard
// reports: nothing.
//
// What it does do is redraw the placeholder inside its input box to say which
// answer it wants. Those strings are fixed, one per blocking mode, and the
// fragments below are them (verified against cursor-agent 2026.07.23).
//
// Matching the provider's own chrome rather than the shape of a question is
// what keeps this honest, and the chrome is the box rather than the sentence:
// an agent whose work is about prompts puts every fragment below on its screen,
// down to the "→" gutter, and that is not hypothetical — it is what the agent
// writing this file did. Only the input box is cursor speaking.
var cursorQuestionPatterns = []QuestionPattern{
	{
		LineStartsWith: "→",
		Contains:       "answer questions (enter to select",
		Question:       "cursor is asking a question — answer it in the agent's terminal",
	},
	{
		LineStartsWith: "→",
		Contains:       "waiting for decision (y/n/p)",
		Question:       "cursor is waiting for a decision (y/n/p)",
	},
	{
		LineStartsWith: "→",
		Contains:       "approve mode switch (y/n)",
		Question:       "cursor is waiting for approval to switch mode",
	},
	{
		LineStartsWith: "→",
		Contains:       "tell the agent what to do instead",
		Question:       "cursor is waiting to be told what to do instead",
	},
	{
		LineStartsWith: "→",
		Contains:       "describe how to revise the plan",
		Question:       "cursor is waiting for the plan to be revised",
	},
	{
		LineStartsWith: "→",
		Contains:       "edit the image prompt, then press enter",
		Question:       "cursor is waiting for an image prompt",
	},
}

// cursorInputBoxTop is the rule cursor draws above its input box. Anything
// above the last one is the agent's own output.
const cursorInputBoxTop = "▄▄▄▄"

// DetectQuestion implements QuestionDetector.
//
// When cursor's input box is not on screen at all its placeholders cannot
// legitimately appear either, so only the provider-independent shapes are
// tried — which is what covers the prompts cursor puts up before its interface
// exists, like the workspace trust question.
func (p *CursorProvider) DetectQuestion(pane string) (string, bool) {
	if box, ok := cursorPromptBox(pane); ok {
		if question, waiting := MatchQuestion(box, 0, cursorQuestionPatterns); waiting {
			return question, true
		}
	}
	return MatchCommonQuestion(pane)
}

// cursorPromptBox returns the tail of pane from cursor's input box downwards,
// or false when the box is not on screen.
func cursorPromptBox(pane string) (string, bool) {
	lines := paneTail(StripANSI(pane), questionTailLines)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), cursorInputBoxTop) {
			return strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", false
}
