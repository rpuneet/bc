package deps

import (
	"context"
	"errors"
)

// Browser is a deprecated dependency kept in the registry for
// discoverability. The Claude Code browser plugin supersedes it.
type Browser struct{}

// NewBrowser constructs the deprecated stub.
func NewBrowser() *Browser { return &Browser{} }

// ID implements Dependency.
func (*Browser) ID() string { return "mycel-browser" }

// DisplayName implements Dependency.
func (*Browser) DisplayName() string { return "mycel-browser" }

// Description implements Dependency.
func (*Browser) Description() string {
	return "Playwright browser service — deprecated, superseded by the Claude Code browser plugin"
}

// Deprecated implements Dependency.
func (*Browser) Deprecated() bool { return true }

// Status always reports stopped.
func (*Browser) Status(_ context.Context) (State, error) { return StateStopped, nil }

// Start refuses — the dependency is deprecated.
func (*Browser) Start(_ context.Context) error {
	return errors.New("deprecated; see docs/deps/mycel-browser.md")
}

// Stop is a no-op.
func (*Browser) Stop(_ context.Context) error { return nil }

// Logs returns an empty slice.
func (*Browser) Logs(_ context.Context, _ int) ([]string, error) { return nil, nil }
