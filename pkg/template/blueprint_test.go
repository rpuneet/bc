package template

import (
	"strings"
	"testing"
)

func TestValidateLabel(t *testing.T) {
	cases := []struct {
		label string
		ok    bool
	}{
		{"", true},
		{LabelSingleAgent, true},
		{LabelMultiAgent, true},
		{"other", false},
		{"team", false},
	}
	for _, tc := range cases {
		err := (Template{Name: "x", Label: tc.label}).Validate()
		if tc.ok && err != nil {
			t.Errorf("label %q: unexpected error %v", tc.label, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("label %q: expected error", tc.label)
		}
	}
}

func TestValidateComposesRejectsSelfAndSingleLabel(t *testing.T) {
	if err := (Template{Name: "team", Composes: []string{"team"}}).Validate(); err == nil {
		t.Fatal("expected self-compose error")
	}
	if err := (Template{
		Name:     "team",
		Label:    LabelSingleAgent,
		Composes: []string{"blank"},
	}).Validate(); err == nil {
		t.Fatal("expected single-agent+composes error")
	}
}

func TestExpandLeafIsItself(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Create(Template{Name: "blank", Description: "x", MCPs: []string{"mycel"}, Secrets: []string{"A"}}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	got, err := Expand(s, "blank")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Leaves) != 1 || got.Leaves[0] != "blank" {
		t.Fatalf("leaves = %v", got.Leaves)
	}
	if len(got.Secrets) != 1 || got.Secrets[0] != "A" {
		t.Fatalf("secrets = %v", got.Secrets)
	}
}

func TestExpandFlattensComposition(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	for _, tmpl := range []Template{
		{Name: "eng", Description: "e", MCPs: []string{"mycel"}, Secrets: []string{"GH"}},
		{Name: "test", Description: "t", MCPs: []string{"mycel"}, Secrets: []string{"GH"}},
		{Name: "pm", Description: "p", MCPs: []string{"mycel"}},
		{
			Name:     "engineering-team",
			Label:    LabelMultiAgent,
			Composes: []string{"eng", "test", "pm"},
			Secrets:  []string{"TEAM_TOKEN"},
		},
	} {
		if err := s.Create(tmpl, "", ScopeGlobal); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Expand(s, "engineering-team")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"eng", "test", "pm"}
	if len(got.Leaves) != len(want) {
		t.Fatalf("leaves = %v, want %v", got.Leaves, want)
	}
	for i := range want {
		if got.Leaves[i] != want[i] {
			t.Fatalf("leaves[%d] = %q, want %q", i, got.Leaves[i], want[i])
		}
	}
	for _, sec := range []string{"GH", "TEAM_TOKEN"} {
		found := false
		for _, s := range got.Secrets {
			if s == sec {
				found = true
			}
		}
		if !found {
			t.Errorf("missing secret %q in %v", sec, got.Secrets)
		}
	}
}

func TestExpandDetectsCycle(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Create(Template{Name: "a", Composes: []string{"b"}, Label: LabelMultiAgent}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Template{Name: "b", Composes: []string{"a"}, Label: LabelMultiAgent}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := Expand(s, "a"); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestExpandMissingChild(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Create(Template{Name: "team", Composes: []string{"nope"}, Label: LabelMultiAgent}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	_, err := Expand(s, "team")
	if err == nil {
		t.Fatal("expected missing child error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should name the missing child: %v", err)
	}
}
