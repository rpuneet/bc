package provider

import (
	"strings"
	"testing"
)

// A cursor agent mid-turn, captured verbatim from a tmux pane. Every fixture
// below is the bottom of a real cursor screen, because that is the only part a
// question detector looks at and hand-typed approximations of it are how the
// first attempt at pane reading passed its tests and matched nothing in
// production (see TestDetectFailureSeesThroughTerminalColour).
const cursorWorkingPane = `  $ npm run dev 14m 55s in current dir
    … 23 output lines hidden · ctrl+o to expand
    ✓ Running next.config.ts took 27ms

 ⠰⠰ Waiting  26.02k tokens
    Tip: Use /config to customize Cursor settings and behavior.
 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Add a follow-up                                  ctrl+c to stop
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  2 tasks
  GPT-5.6 Sol 272K Medium · 23.6% · 20 files edited      Run Everything
  ~/.mycel/agents/eager-fox/worktree · 3b4b167

`

// The same screen with cursor's input box in a mode that blocks on a person.
const cursorAskingPane = `  I found two candidate fixes and need you to pick one.

 ⠰⠰ Waiting for you  26.02k tokens
 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Answer questions (Enter to select/next, Esc to skip)
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  2 tasks
  GPT-5.6 Sol 272K Medium · 23.6% · 20 files edited      Run Everything
  ~/.mycel/agents/eager-fox/worktree · 3b4b167

`

const cursorDecisionPane = `  Ready to apply the migration.

 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Waiting for decision (y/n/p)...
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  Auto · 32.7% · 1 file edited                           Run Everything
  ~/.mycel/agents/keen-lemur/worktree · main

`

func TestCursorDetectQuestion(t *testing.T) {
	p := &CursorProvider{}

	question, waiting := p.DetectQuestion(cursorAskingPane)
	if !waiting {
		t.Fatal("cursor's ask-question placeholder must be read as waiting on a person")
	}
	if !strings.Contains(question, "cursor is asking a question") {
		t.Errorf("question = %q, want it to say what is being waited on", question)
	}

	if _, waiting := p.DetectQuestion(cursorDecisionPane); !waiting {
		t.Error("cursor's decision placeholder must be read as waiting on a person")
	}
}

// The whole capability is worth less than nothing if it flags working agents:
// a state that cries wolf is a state people stop looking at.
func TestCursorDetectQuestionIgnoresWorkingAgents(t *testing.T) {
	p := &CursorProvider{}

	falsePositives := map[string]string{
		// The bottom of a real, busy cursor screen. Note the status line
		// already says "Waiting" while the agent is working — a detector
		// matching that word alone would flag every agent in the fleet.
		"cursor mid-turn": cursorWorkingPane,

		// An agent whose work is about prompts. Every fragment the detector
		// looks for is on screen, in the agent's own output rather than in
		// cursor's input box.
		"agent writing about prompts": `  Added the pane detector patterns:

    → Answer questions (Enter to select/next, Esc to skip)
    → Waiting for decision (y/n/p)
    Do you want to proceed? (y/n)

 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Add a follow-up                                  ctrl+c to stop
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  ~/.mycel/agents/eager-fox/worktree · 3b4b167

`,

		// A diff, which is where "(y/n)" turns up in ordinary work.
		"a diff containing a prompt": `  +	fmt.Print("Overwrite the file? (y/n) ")
  +	if answer := readLine(); answer != "y" {

 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Add a follow-up                                  ctrl+c to stop
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  ~/.mycel/agents/eager-fox/worktree · main

`,

		// A numbered list the agent wrote, with no selection cursor on it.
		"a numbered list in the output": `  Three options:

  1. Rewrite the parser
  2. Patch the caller
  3. Leave it alone
`,

		// A prompt from an earlier, already-answered turn, scrolled up.
		"an answered prompt in scrollback": `  Do you want to proceed? (y/n)
  y
  Applied 3 files.
`,
		"empty pane": "",
	}

	for name, pane := range falsePositives {
		t.Run(name, func(t *testing.T) {
			if question, waiting := p.DetectQuestion(pane); waiting {
				t.Errorf("a working agent was reported as waiting on %q", question)
			}
		})
	}
}

// The provider-independent shapes, for a CLI that prints its question and
// blocks on a read rather than drawing a screen around it.
func TestMatchCommonQuestion(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want bool
	}{
		{"yes/no at the prompt", "Building…\nOverwrite config.toml? (y/n) ", true},
		{"bracketed yes/no", "Detected 2 worktrees.\nRemove them? [y/N]", true},
		{"proceed", "This will drop the table.\nDo you want to proceed?", true},
		{"press enter", "Review the plan above.\nPress Enter to continue", true},
		{"open menu", "Which fix?\n❯ 1. Rewrite the parser\n  2. Patch the caller", true},
		{"menu with the cursor above the end", "❯ 1. Rewrite the parser\n  2. Patch the caller\n  3. Leave it", true},
		{"numbered list, no cursor", "Options:\n  1. Rewrite the parser\n  2. Patch the caller", false},
		{"bare prompt cursor", "Done.\n❯ ", false},
		{"prompt followed by more output", "Do you want to proceed? (y/n)\ny\nDone in 4s.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, waiting := MatchCommonQuestion(tt.pane); waiting != tt.want {
				t.Errorf("waiting = %v, want %v", waiting, tt.want)
			}
		})
	}
}

// Panes are never plain text — the same lesson failure detection learned the
// hard way.
func TestDetectQuestionSeesThroughTerminalColor(t *testing.T) {
	pane := "\x1b[2K \x1b[38;2;80;80;80m▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄\x1b[0m\n" +
		"\x1b[2K  \x1b[38;2;102;102;102m→\x1b[0m Answer questions (Enter to select/next, Esc to skip)\x1b[39m\n" +
		"\x1b[2K  ~/.mycel/agents/eager-fox/worktree · main\n"

	if _, waiting := (&CursorProvider{}).DetectQuestion(pane); !waiting {
		t.Fatal("a colored prompt must be detected — panes are never plain text")
	}
}

// Ensure CursorProvider satisfies the capability the monitor looks for.
var _ QuestionDetector = (*CursorProvider)(nil)
