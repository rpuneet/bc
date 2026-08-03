package server

import (
	"testing"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
)

// providers.default decides which tool `mycel agent create` uses when no --tool
// is given. It used to reach only the "default" badge on the providers page,
// while agent creation used the compiled-in default — so switching the default
// changed a label and nothing else, and every new agent came up on the old tool
// with nothing on screen to explain why.
//
// A setting that silently does nothing is indistinguishable from one that
// works until you check the result, which is why these assert on the manager's
// resolved default rather than on the call succeeding.

func managerWithDefault(t *testing.T, configured string) *agentpkg.Manager {
	t.Helper()
	mgr := agentpkg.NewManagerWithRepo(t.TempDir(), "")
	cfg := &home.Config{}
	cfg.Providers.Default = configured
	applyDefaultProvider(mgr, &home.Home{Config: cfg})
	return mgr
}

func TestApplyDefaultProviderUsesTheConfiguredProvider(t *testing.T) {
	if got := managerWithDefault(t, "cursor").DefaultTool(); got != "cursor" {
		t.Errorf("default tool = %q, want cursor", got)
	}
}

// An unregistered name must not become the default, or every agent creation
// would fail with a provider that cannot be looked up. A provider can leave the
// registry between releases while a config file naming it stays behind, so this
// falls back rather than failing the daemon's boot.
func TestApplyDefaultProviderIgnoresUnknownProvider(t *testing.T) {
	if got := managerWithDefault(t, "not-a-real-provider").DefaultTool(); got != agentpkg.DefaultProvider {
		t.Errorf("default tool = %q, want the built-in %q", got, agentpkg.DefaultProvider)
	}
}

// No configuration at all is the common case and must leave the built-in
// default alone.
func TestApplyDefaultProviderLeavesBuiltInDefault(t *testing.T) {
	if got := managerWithDefault(t, "").DefaultTool(); got != agentpkg.DefaultProvider {
		t.Errorf("default tool = %q, want the built-in %q", got, agentpkg.DefaultProvider)
	}
}

// A nil Config is reachable — the daemon boots with one when no prefs.json
// exists yet — and must not panic on the way to the built-in default.
func TestApplyDefaultProviderSurvivesNilConfig(t *testing.T) {
	mgr := agentpkg.NewManagerWithRepo(t.TempDir(), "")
	applyDefaultProvider(mgr, &home.Home{})
	if got := mgr.DefaultTool(); got != agentpkg.DefaultProvider {
		t.Errorf("default tool = %q, want the built-in %q", got, agentpkg.DefaultProvider)
	}
}
