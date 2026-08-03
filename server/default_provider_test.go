package server

import (
	"context"
	"os/exec"
	"testing"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
	runtimepkg "github.com/rpuneet/mycel/pkg/runtime"
)

// stubBackend stands in for the docker backend so a manager can be built the
// way newAgentManager builds it on a machine where docker is reachable. Nothing
// here is called: these tests only ask which backend is the default.
type stubBackend struct{}

func (stubBackend) HasSession(context.Context, string) bool { return false }
func (stubBackend) CreateSession(context.Context, string, string) error {
	return nil
}
func (stubBackend) CreateSessionWithCommand(context.Context, string, string, string) error {
	return nil
}
func (stubBackend) CreateSessionWithEnv(context.Context, string, string, string, map[string]string) error {
	return nil
}
func (stubBackend) KillSession(context.Context, string) error           { return nil }
func (stubBackend) RenameSession(context.Context, string, string) error { return nil }
func (stubBackend) SendKeys(context.Context, string, string) error      { return nil }
func (stubBackend) SendKeysWithSubmit(context.Context, string, string, string) error {
	return nil
}
func (stubBackend) Capture(context.Context, string, int) (string, error) { return "", nil }
func (stubBackend) ListSessions(context.Context) ([]runtimepkg.Session, error) {
	return nil, nil
}
func (stubBackend) AttachCmd(context.Context, string) *exec.Cmd { return nil }
func (stubBackend) IsRunning(context.Context) bool              { return true }
func (stubBackend) KillServer(context.Context) error            { return nil }
func (stubBackend) SetEnvironment(context.Context, string, string, string) error {
	return nil
}
func (stubBackend) SessionName(name string) string                 { return name }
func (stubBackend) PipePane(context.Context, string, string) error { return nil }

var _ runtimepkg.Backend = stubBackend{}

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

// runtime.default was ignored whenever docker could be reached: whether the
// docker backend existed decided which backend was the default, so starting
// Docker Desktop moved every new agent into a container. Container creation then
// fails outright for any provider without a prebuilt mycel-agent-<tool> image,
// so the consequence is not a different-but-working runtime, it is agent
// creation that stops working.
func TestApplyDefaultRuntimeHonorsTmuxWhenDockerIsAvailable(t *testing.T) {
	// A manager built the way newAgentManager builds it when docker is
	// reachable: docker registered and default, tmux registered alongside.
	mgr := agentpkg.NewManagerWithRuntime(t.TempDir(), "", stubBackend{}, "docker")
	if got := mgr.DefaultBackend(); got != "docker" {
		t.Fatalf("precondition: default backend = %q, want docker", got)
	}

	applyDefaultRuntime(mgr, "tmux")

	if got := mgr.DefaultBackend(); got != "tmux" {
		t.Errorf("default backend = %q, want tmux — the configured default was ignored", got)
	}
}

// An unavailable runtime must not become the default, or every creation would
// fail against a backend that isn't registered.
func TestApplyDefaultRuntimeIgnoresUnavailableRuntime(t *testing.T) {
	mgr := agentpkg.NewManagerWithRepo(t.TempDir(), "")
	applyDefaultRuntime(mgr, "docker")
	if got := mgr.DefaultBackend(); got != "tmux" {
		t.Errorf("default backend = %q, want tmux — docker was never registered", got)
	}
}

func TestApplyDefaultRuntimeLeavesDefaultWhenUnset(t *testing.T) {
	mgr := agentpkg.NewManagerWithRuntime(t.TempDir(), "", stubBackend{}, "docker")
	applyDefaultRuntime(mgr, "")
	if got := mgr.DefaultBackend(); got != "docker" {
		t.Errorf("default backend = %q, want docker left alone", got)
	}
}
