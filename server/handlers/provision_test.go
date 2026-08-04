package handlers

import (
	"errors"
	"testing"

	"github.com/rpuneet/mycel/pkg/secret"
)

type stubVault struct {
	byName map[string]*secret.SecretMeta
	err    error
}

func (s stubVault) GetMeta(name string) (*secret.SecretMeta, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byName[name], nil
}

func TestVaultPresenceHas(t *testing.T) {
	t.Parallel()
	v := NewVaultPresence(stubVault{byName: map[string]*secret.SecretMeta{
		"HAVE": {Name: "HAVE"},
	}})
	if !v.Has("HAVE") {
		t.Fatal("HAVE should be present")
	}
	if v.Has("NEED") {
		t.Fatal("NEED must be absent — GetMeta returns (nil, nil) for missing names")
	}
	if p := NewVaultPresence(nil); p != nil {
		t.Fatal("nil store must yield nil SecretPresence")
	}
	if NewVaultPresence(stubVault{err: errors.New("boom")}).Has("HAVE") {
		t.Fatal("store errors must report absent")
	}
}

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
