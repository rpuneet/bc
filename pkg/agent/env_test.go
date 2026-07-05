package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/secret"
)

// --- name validation ---

func TestIsValidEnvName(t *testing.T) {
	valid := []string{"FOO", "_FOO", "foo_bar", "A1", "_1", "HTTP_PROXY"}
	for _, n := range valid {
		if !IsValidEnvName(n) {
			t.Errorf("IsValidEnvName(%q) = false, want true", n)
		}
	}
	invalid := []string{"", "1FOO", "FOO-BAR", "FOO BAR", "FOO=1", "a.b", "${X}"}
	for _, n := range invalid {
		if IsValidEnvName(n) {
			t.Errorf("IsValidEnvName(%q) = true, want false", n)
		}
	}
}

// --- store round-trip ---

func TestSQLiteStore_EnvRoundTrip(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	a := &Agent{
		Name:      "env-agent",
		Role:      Role("engineer"),
		State:     StateStopped,
		Workspace: "/ws",
		Env: map[string]string{
			"FOO":     "bar",
			"API_KEY": "${secret:MY_TOKEN}",
		},
		StartedAt: time.Now(),
	}
	if saveErr := store.Save(ctx, a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	got, err := store.Load(ctx, "env-agent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil agent")
	}
	if len(got.Env) != 2 || got.Env["FOO"] != "bar" || got.Env["API_KEY"] != "${secret:MY_TOKEN}" {
		t.Errorf("Env round-trip mismatch: %#v", got.Env)
	}

	// Secret references must be stored verbatim — never resolved.
	if got.Env["API_KEY"] != "${secret:MY_TOKEN}" {
		t.Errorf("secret ref was not stored verbatim: %q", got.Env["API_KEY"])
	}

	// SaveAll path round-trips too.
	a.Env["EXTRA"] = "1"
	if saveAllErr := store.SaveAll(ctx, map[string]*Agent{a.Name: a}); saveAllErr != nil {
		t.Fatalf("SaveAll: %v", saveAllErr)
	}
	got, err = store.Load(ctx, "env-agent")
	if err != nil {
		t.Fatalf("Load after SaveAll: %v", err)
	}
	if len(got.Env) != 3 || got.Env["EXTRA"] != "1" {
		t.Errorf("Env after SaveAll mismatch: %#v", got.Env)
	}
}

func TestSQLiteStore_EnvEmpty(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	a := &Agent{Name: "no-env", Role: Role("engineer"), State: StateStopped, StartedAt: time.Now()}
	if saveErr := store.Save(ctx, a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}
	got, err := store.Load(ctx, "no-env")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Env != nil {
		t.Errorf("expected nil Env for agent saved without env, got %#v", got.Env)
	}
}

// --- merge order ---

func TestMergeUserEnv_ReservedBCKeysWin(t *testing.T) {
	env := map[string]string{
		"BC_AGENT_ID":  "real-agent",
		"BC_WORKSPACE": "/real/ws",
	}
	mergeUserEnv(env, map[string]string{
		"BC_AGENT_ID": "spoofed",
		"BC_EVIL":     "1",
		"FOO":         "bar",
		"PATH_EXTRA":  "/opt/bin",
	}, "test-agent")

	if env["BC_AGENT_ID"] != "real-agent" {
		t.Errorf("BC_AGENT_ID clobbered by user env: %q", env["BC_AGENT_ID"])
	}
	if _, ok := env["BC_EVIL"]; ok {
		t.Error("user-supplied BC_ key must be skipped entirely")
	}
	if env["FOO"] != "bar" || env["PATH_EXTRA"] != "/opt/bin" {
		t.Errorf("non-reserved user env not merged: %#v", env)
	}
}

func TestInjectEnv_UserEnvWinsOverEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "agent.env")
	if err := os.WriteFile(envFile, []byte("FOO=from-file\nONLY_FILE=1\n"), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env := map[string]string{"BC_AGENT_ID": "a1"}
	injectEnv(env, dir, "a1", envFile, map[string]string{"FOO": "from-config"})

	if env["FOO"] != "from-config" {
		t.Errorf("configured env should win over env file: FOO=%q", env["FOO"])
	}
	if env["ONLY_FILE"] != "1" {
		t.Errorf("env file entries should still merge: ONLY_FILE=%q", env["ONLY_FILE"])
	}
}

// --- secret resolution at spawn ---

// seedVault creates a secrets vault under wsPath/.bc and stores the given
// secret. The passphrase is pinned via BC_SECRET_PASSPHRASE so the spawn
// path opens the same vault.
func seedVault(t *testing.T, wsPath, name, value string) {
	t.Helper()
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")
	ss, err := secret.NewStore(wsPath, "test-passphrase")
	if err != nil {
		t.Fatalf("secret.NewStore: %v", err)
	}
	defer func() { _ = ss.Close() }()
	if err := ss.Set(name, value, "test secret"); err != nil {
		t.Fatalf("secret Set: %v", err)
	}
}

func TestInjectEnv_ResolvesSecretRefs(t *testing.T) {
	ws := t.TempDir()
	seedVault(t, ws, "MY_TOKEN", "s3cr3t-value")

	env := map[string]string{"BC_AGENT_ID": "a1"}
	injectEnv(env, ws, "a1", "", map[string]string{
		"API_KEY": "${secret:MY_TOKEN}",
		"PLAIN":   "not-a-ref",
	})

	if env["API_KEY"] != "s3cr3t-value" {
		t.Errorf("secret ref not resolved at spawn: API_KEY=%q", env["API_KEY"])
	}
	if env["PLAIN"] != "not-a-ref" {
		t.Errorf("plain value mangled: %q", env["PLAIN"])
	}
}

// --- spawn paths ---

// gitInit creates a git repo with one commit so worktree creation works.
func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test"},
		{"config", "user.name", "test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec,noctx // test helper
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

// TestCreateAgent_StoresAndInjectsEnv verifies the fresh-create path: the
// configured env map is persisted verbatim on the agent (secret refs
// unresolved) while the session env receives resolved values, with BC_*
// system vars protected.
func TestCreateAgent_StoresAndInjectsEnv(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	repo := gitInit(t)
	seedVault(t, repo, "SPAWN_TOKEN", "resolved-at-spawn")

	be := newMockBackend("docker")
	m := newMockManager(t, "docker", map[string]*mockBackend{"docker": be})

	a, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{
		Name:      "env-create",
		Role:      Role("engineer"),
		Workspace: repo,
		Env: map[string]string{
			"FOO":         "bar",
			"API_KEY":     "${secret:SPAWN_TOKEN}",
			"BC_AGENT_ID": "spoofed",
		},
	})
	if err != nil {
		t.Fatalf("SpawnAgentWithOptions: %v", err)
	}

	// Stored on the agent verbatim (reference, not resolved value).
	if a.Env["API_KEY"] != "${secret:SPAWN_TOKEN}" {
		t.Errorf("agent.Env must keep the secret reference, got %q", a.Env["API_KEY"])
	}

	_, sessEnv := be.lastSession()
	if sessEnv == nil {
		t.Fatal("CreateSessionWithEnv not called")
	}
	if sessEnv["FOO"] != "bar" {
		t.Errorf("session env missing configured var: FOO=%q", sessEnv["FOO"])
	}
	if sessEnv["API_KEY"] != "resolved-at-spawn" {
		t.Errorf("secret ref not resolved in session env: API_KEY=%q", sessEnv["API_KEY"])
	}
	if sessEnv["BC_AGENT_ID"] != "env-create" {
		t.Errorf("BC_AGENT_ID must not be clobbered: %q", sessEnv["BC_AGENT_ID"])
	}

	// Persisted: round-trips through the store with the reference intact.
	stored, err := m.store.Load(context.Background(), "env-create")
	if err != nil || stored == nil {
		t.Fatalf("store.Load: %v (agent=%v)", err, stored)
	}
	if stored.Env["API_KEY"] != "${secret:SPAWN_TOKEN}" || stored.Env["FOO"] != "bar" {
		t.Errorf("persisted env mismatch: %#v", stored.Env)
	}
}

// TestStartAgent_ReinjectsStoredEnv verifies the restart path re-injects the
// stored env map (with secret refs resolved fresh) into the new session.
func TestStartAgent_ReinjectsStoredEnv(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	// Restart resolves the worktree manager from the agent's repo, which
	// must be a real git repo for worktreeManagerFor to anchor there.
	ws := gitInit(t)
	seedVault(t, ws, "ROTATED", "rotated-value")

	// Fake worktree that passes the .git existence checks.
	wtDir := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wtDir, 0750); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: /nowhere"), 0600); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	be := newMockBackend("docker")
	m := newMockManager(t, "docker", map[string]*mockBackend{"docker": be})
	m.agents["env-restart"] = &Agent{
		Name:           "env-restart",
		Role:           Role("engineer"),
		State:          StateStopped,
		Workspace:      ws,
		WorktreeDir:    wtDir,
		RuntimeBackend: "docker",
		Env: map[string]string{
			"FOO":     "persisted",
			"API_KEY": "${secret:ROTATED}",
		},
		StartedAt: time.Now(),
	}

	if _, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{
		Name:      "env-restart",
		Role:      Role("engineer"),
		Workspace: ws,
	}); err != nil {
		t.Fatalf("restart SpawnAgentWithOptions: %v", err)
	}

	_, sessEnv := be.lastSession()
	if sessEnv == nil {
		t.Fatal("CreateSessionWithEnv not called on restart")
	}
	if sessEnv["FOO"] != "persisted" {
		t.Errorf("restart did not re-inject stored env: FOO=%q", sessEnv["FOO"])
	}
	if sessEnv["API_KEY"] != "rotated-value" {
		t.Errorf("restart must resolve secret refs fresh: API_KEY=%q", sessEnv["API_KEY"])
	}
	if sessEnv["BC_AGENT_ID"] != "env-restart" {
		t.Errorf("BC_AGENT_ID wrong on restart: %q", sessEnv["BC_AGENT_ID"])
	}
}
