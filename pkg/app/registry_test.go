package app

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// fakePlugin is a minimal Plugin for registry tests.
type fakePlugin struct {
	id string
}

func (p fakePlugin) Describe() Descriptor {
	return Descriptor{ID: p.id, Label: p.id, Auth: AuthNone}
}

func (p fakePlugin) Build(_ Instance, _ Env) (gateway.NotificationAdapter, error) {
	return nil, nil
}

func TestRegistryRegisterGet(t *testing.T) {
	r := NewRegistry()
	r.Register(fakePlugin{id: "slack"})

	p, ok := r.Get("slack")
	if !ok {
		t.Fatal("Get(slack) not found")
	}
	if p.Describe().ID != "slack" {
		t.Errorf("Describe().ID = %q, want slack", p.Describe().ID)
	}

	if _, ok := r.Get("nope"); ok {
		t.Error("Get(nope) found, want missing")
	}
}

func TestRegistryRegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	first := fakePlugin{id: "slack"}
	second := fakePlugin{id: "slack"}
	r.Register(first)
	r.Register(second)

	if got := len(r.List()); got != 1 {
		t.Errorf("List() len = %d, want 1", got)
	}
}

func TestRegistryListSorted(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"webhook", "slack", "telegram", "rss"} {
		r.Register(fakePlugin{id: id})
	}

	want := []string{"rss", "slack", "telegram", "webhook"}
	list := r.List()
	if len(list) != len(want) {
		t.Fatalf("List() len = %d, want %d", len(list), len(want))
	}
	for i, p := range list {
		if p.Describe().ID != want[i] {
			t.Errorf("List()[%d] = %q, want %q", i, p.Describe().ID, want[i])
		}
	}
}

func TestDefaultRegistryHelpers(t *testing.T) {
	// The default registry is package-global; use an ID no real plugin claims.
	Register(fakePlugin{id: "test-fake"})
	defer delete(DefaultRegistry.plugins, "test-fake")

	if _, ok := Get("test-fake"); !ok {
		t.Fatal("Get(test-fake) not found after Register")
	}
	found := false
	for _, p := range List() {
		if p.Describe().ID == "test-fake" {
			found = true
		}
	}
	if !found {
		t.Error("List() missing test-fake")
	}
}
