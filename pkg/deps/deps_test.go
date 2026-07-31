package deps

import (
	"context"
	"errors"
	"testing"
)

// stubDep is a tiny Dependency implementation used to drive registry tests.
type stubDep struct {
	id         string
	deprecated bool
}

func (s *stubDep) ID() string                                      { return s.id }
func (s *stubDep) DisplayName() string                             { return s.id }
func (s *stubDep) Description() string                             { return "stub" }
func (s *stubDep) Status(_ context.Context) (State, error)         { return StateStopped, nil }
func (s *stubDep) Start(_ context.Context) error                   { return nil }
func (s *stubDep) Stop(_ context.Context) error                    { return nil }
func (s *stubDep) Logs(_ context.Context, _ int) ([]string, error) { return nil, nil }
func (s *stubDep) Deprecated() bool                                { return s.deprecated }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubDep{id: "alpha"})
	r.Register(&stubDep{id: "beta"})

	got, err := r.Get("alpha")
	if err != nil {
		t.Fatalf("Get(alpha) error: %v", err)
	}
	if got.ID() != "alpha" {
		t.Errorf("ID = %q, want alpha", got.ID())
	}

	// nil register is a no-op, not a panic.
	r.Register(nil)
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubDep{id: "beta"})
	r.Register(&stubDep{id: "alpha"})
	r.Register(&stubDep{id: "gamma"})

	out := r.List()
	if len(out) != 3 {
		t.Fatalf("List len = %d, want 3", len(out))
	}
	// Sorted alphabetically.
	want := []string{"alpha", "beta", "gamma"}
	for i, d := range out {
		if d.ID() != want[i] {
			t.Errorf("List[%d] = %q, want %q", i, d.ID(), want[i])
		}
	}
}

func TestRegistryRegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubDep{id: "x", deprecated: false})
	r.Register(&stubDep{id: "x", deprecated: true})

	got, err := r.Get("x")
	if err != nil {
		t.Fatalf("Get(x) error: %v", err)
	}
	if !got.Deprecated() {
		t.Error("expected second registration to overwrite first")
	}
}

func TestBrowserDeprecated(t *testing.T) {
	b := NewBrowser()
	if !b.Deprecated() {
		t.Error("mycel-browser should be deprecated")
	}
	if err := b.Start(context.Background()); err == nil {
		t.Error("mycel-browser Start should fail")
	}
	st, err := b.Status(context.Background())
	if err != nil {
		t.Errorf("Status err: %v", err)
	}
	if st != StateStopped {
		t.Errorf("Status = %v, want stopped", st)
	}
}
