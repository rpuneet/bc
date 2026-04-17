package deps

import (
	"context"
	"errors"
)

// BCBrowser is a deprecated dependency kept in the registry for
// discoverability. The Claude Code browser plugin supersedes it.
type BCBrowser struct{}

// NewBCBrowser constructs the deprecated stub.
func NewBCBrowser() *BCBrowser { return &BCBrowser{} }

// ID implements Dependency.
func (*BCBrowser) ID() string { return "bc-browser" }

// DisplayName implements Dependency.
func (*BCBrowser) DisplayName() string { return "bc-browser" }

// Description implements Dependency.
func (*BCBrowser) Description() string {
	return "Playwright browser service — deprecated, superseded by the Claude Code browser plugin"
}

// Deprecated implements Dependency.
func (*BCBrowser) Deprecated() bool { return true }

// Status always reports stopped.
func (*BCBrowser) Status(_ context.Context) (State, error) { return StateStopped, nil }

// Start refuses — the dependency is deprecated.
func (*BCBrowser) Start(_ context.Context) error {
	return errors.New("deprecated; see docs/deps/bc-browser.md")
}

// Stop is a no-op.
func (*BCBrowser) Stop(_ context.Context) error { return nil }

// Logs returns an empty slice.
func (*BCBrowser) Logs(_ context.Context, _ int) ([]string, error) { return nil, nil }
