package handlers

import "testing"

func TestLeafAgentName(t *testing.T) {
	t.Parallel()
	if got := leafAgentName("solo", "blank", false); got != "solo" {
		t.Fatalf("single: got %q", got)
	}
	if got := leafAgentName("eng", "reviewer", true); got != "eng-reviewer" {
		t.Fatalf("multi: got %q", got)
	}
	if got := leafAgentName("", "reviewer", true); got != "reviewer" {
		t.Fatalf("empty base: got %q", got)
	}
}

func TestLeafTool(t *testing.T) {
	t.Parallel()
	if got := leafTool("cursor", "claude"); got != "cursor" {
		t.Fatalf("req wins: got %q", got)
	}
	if got := leafTool("", "claude"); got != "claude" {
		t.Fatalf("provider fallback: got %q", got)
	}
}

func TestUnionStringSlice(t *testing.T) {
	t.Parallel()
	got := unionStringSlice([]string{"A", "B"}, []string{"B", "C"})
	if len(got) != 3 || got[0] != "A" || got[1] != "B" || got[2] != "C" {
		t.Fatalf("got %v", got)
	}
}
