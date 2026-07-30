package server

import (
	"context"
	"sync"
	"testing"

	bcapp "github.com/rpuneet/mycel/pkg/app"
	bcworkspace "github.com/rpuneet/mycel/pkg/workspace"
)

// TestBuildGatewayManagerFromApps proves the data-driven loop: a config
// with two instances — one buildable, one referencing an unknown app —
// yields one registered adapter and one degraded entry.
func TestBuildGatewayManagerFromApps(t *testing.T) {
	cfg := bcworkspace.DefaultConfig()
	cfg.Apps = map[string]bcapp.InstanceConfig{
		"webhook:ci": {App: "webhook", Enabled: true},
		"bogus":      {App: "no-such-app", Enabled: true},
		"rss:off":    {App: "rss", Enabled: false, Config: map[string]string{"url": "https://x/feed"}},
	}
	ws := &bcworkspace.Workspace{Config: &cfg, RootDir: t.TempDir()}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	degraded := map[string]string{}

	m := buildGatewayManager(ctx, ws, nil, nil, degraded, &wg)
	if m == nil {
		t.Fatal("manager is nil")
	}

	if m.GetAdapter("webhook:ci") == nil {
		t.Error("webhook:ci adapter not registered")
	}
	if m.GetAdapter("rss:off") != nil {
		t.Error("disabled instance must not be registered")
	}
	if reason, ok := degraded["app:bogus"]; !ok || reason == "" {
		t.Errorf("degraded[app:bogus] = %q, want unknown-app reason", reason)
	}
	if _, ok := degraded["app:webhook:ci"]; ok {
		t.Error("healthy instance must not be degraded")
	}

	cancel()
	wg.Wait()
}

// TestBuildGatewayManagerBuildFailureDegrades proves a Build error
// (missing required secret with no vault) degrades the instance instead
// of failing boot.
func TestBuildGatewayManagerBuildFailureDegrades(t *testing.T) {
	cfg := bcworkspace.DefaultConfig()
	cfg.Apps = map[string]bcapp.InstanceConfig{
		// slack requires bot_token; no vault is wired, so Build must fail.
		"slack": {App: "slack", Enabled: true},
	}
	ws := &bcworkspace.Workspace{Config: &cfg, RootDir: t.TempDir()}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	degraded := map[string]string{}

	m := buildGatewayManager(ctx, ws, nil, nil, degraded, &wg)
	if m.GetAdapter("slack") != nil {
		t.Error("slack adapter must not be registered without its secret")
	}
	if reason, ok := degraded["app:slack"]; !ok || reason == "" {
		t.Errorf("degraded[app:slack] = %q, want build-failure reason", reason)
	}

	cancel()
	wg.Wait()
}
