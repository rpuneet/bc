package template

import "testing"

type mapVault map[string]bool

func (m mapVault) Has(name string) bool { return m[name] }

func TestFilterMissing(t *testing.T) {
	t.Parallel()
	vault := mapVault{"HAVE": true}

	got := FilterMissing([]string{"HAVE", "NEED", " HAVE ", "NEED", ""}, vault)
	if len(got) != 1 || got[0] != "NEED" {
		t.Fatalf("got %v, want [NEED]", got)
	}

	if got := FilterMissing(nil, vault); got != nil {
		t.Fatalf("empty declared: got %v", got)
	}

	got = FilterMissing([]string{"A", "B"}, nil)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("nil vault: got %v, want [A B]", got)
	}
}
