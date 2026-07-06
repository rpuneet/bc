package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPiProviderIdentity(t *testing.T) {
	p := NewPiProvider()
	if p.Name() != "pi" {
		t.Errorf("Name() = %q, want pi", p.Name())
	}
	if p.Binary() != "pi" {
		t.Errorf("Binary() = %q, want pi", p.Binary())
	}
	if p.Command() != "pi" {
		t.Errorf("Command() = %q, want pi", p.Command())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.InstallHint() == "" {
		t.Error("expected non-empty install hint")
	}
}

func TestPiInterfaces(t *testing.T) {
	var p Provider = NewPiProvider()
	if _, ok := p.(ModelLister); !ok {
		t.Error("pi must implement ModelLister")
	}
	if _, ok := p.(DynamicModelLister); !ok {
		t.Error("pi must implement DynamicModelLister")
	}
	if _, ok := p.(EnvContributor); !ok {
		t.Error("pi must implement EnvContributor")
	}
}

func TestSafePiModelName(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"amazon-bedrock/moonshotai.kimi-k2.5", true},
		{"groq/llama-3.3-70b-versatile", true},
		{"groq/llama-3.1-8b-instant", true},
		{"anthropic/claude-sonnet-4-6", true},
		{"google/gemini-2.5-pro", true},
		{"moonshotai.kimi-k2.5", true},    // bare model without provider
		{"llama-3.3-70b-versatile", true}, // bare model with dashes
		{"", false},
		{"$(rm -rf /)", false},
		{"a b c", false},
		{"-model", false},            // leading dash = arg injection
		{"--provider amazon", false}, // spaces
		{"model;cat /etc/passwd", false},
		{"model`whoami`", false},
	}
	for _, tt := range tests {
		if got := SafePiModelName(tt.model); got != tt.want {
			t.Errorf("SafePiModelName(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestPiBuildCommand(t *testing.T) {
	p := NewPiProvider()
	tests := []struct { //nolint:govet // test struct, field order matches literal values
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "no opts — base command",
			opts: CommandOpts{},
			want: "pi",
		},
		{
			name: "bedrock provider/model splits into --provider and --model",
			opts: CommandOpts{Model: "amazon-bedrock/moonshotai.kimi-k2.5"},
			want: "pi --provider amazon-bedrock --model moonshotai.kimi-k2.5",
		},
		{
			name: "groq provider/model splits correctly",
			opts: CommandOpts{Model: "groq/llama-3.3-70b-versatile"},
			want: "pi --provider groq --model llama-3.3-70b-versatile",
		},
		{
			name: "bare model without slash — single --model flag",
			opts: CommandOpts{Model: "llama-3.3-70b-versatile"},
			want: "pi --model llama-3.3-70b-versatile",
		},
		{
			name: "unsafe model is dropped",
			opts: CommandOpts{Model: "$(rm -rf /)"},
			want: "pi",
		},
		{
			name: "leading-dash model is dropped (arg injection prevention)",
			opts: CommandOpts{Model: "-continue"},
			want: "pi",
		},
		{
			name: "session ID appended",
			opts: CommandOpts{SessionID: "abc123"},
			want: "pi --session abc123",
		},
		{
			name: "resume flag",
			opts: CommandOpts{Resume: true},
			want: "pi --continue",
		},
		{
			name: "model + session + resume — all three flags",
			opts: CommandOpts{
				Model:     "groq/llama-3.3-70b-versatile",
				SessionID: "abc123",
				Resume:    true,
			},
			want: "pi --provider groq --model llama-3.3-70b-versatile --session abc123 --continue",
		},
		{
			name: "model with dots in model id",
			opts: CommandOpts{Model: "anthropic/claude-sonnet-4-6"},
			want: "pi --provider anthropic --model claude-sonnet-4-6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.BuildCommand(tt.opts)
			if got != tt.want {
				t.Errorf("BuildCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPiModels(t *testing.T) {
	p := NewPiProvider()
	// Static Models() returns empty — live list comes from ListModels.
	// Mycel must not bake in model choices; the user picks from what pi reports.
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("Models() = %v, want empty static list (use ListModels for live list)", models)
	}
}

func TestPiListModels(t *testing.T) {
	p := NewPiProvider()
	orig := piListModels
	t.Cleanup(func() { piListModels = orig })

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "two-column rows joined with slash",
			output: "groq           llama-3.3-70b-versatile\n" +
				"anthropic      claude-sonnet-4-6\n" +
				"amazon-bedrock moonshotai.kimi-k2.5\n",
			want: []string{
				"groq/llama-3.3-70b-versatile",
				"anthropic/claude-sonnet-4-6",
				"amazon-bedrock/moonshotai.kimi-k2.5",
			},
		},
		{
			name:   "empty output returns empty list",
			output: "",
			want:   []string{},
		},
		{
			name:   "single-column rows are skipped",
			output: "header\ngroq  llama-3.3-70b-versatile\n",
			want:   []string{"groq/llama-3.3-70b-versatile"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := tt.output
			piListModels = func(_ context.Context) (string, error) { return captured, nil }
			got, err := p.ListModels(t.Context())
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ListModels() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ListModels()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPiContributeEnv_NoAWSDir(t *testing.T) {
	p := NewPiProvider()
	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	env := map[string]string{}
	p.ContributeEnv(env)
	if _, ok := env["AWS_PROFILE"]; ok {
		t.Error("should not inject AWS_PROFILE when ~/.aws absent")
	}
}

func TestPiContributeEnv_WithAWSDir(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write a minimal ~/.aws/config with a region
	cfgContent := "[default]\nregion = us-west-2\noutput = json\n"
	if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	p := NewPiProvider()
	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	env := map[string]string{}
	p.ContributeEnv(env)

	if env["AWS_PROFILE"] != "default" {
		t.Errorf("AWS_PROFILE = %q, want default", env["AWS_PROFILE"])
	}
	if env["AWS_REGION"] != "us-west-2" {
		t.Errorf("AWS_REGION = %q, want us-west-2", env["AWS_REGION"])
	}
}

func TestPiContributeEnv_ExistingAWSVarSkipped(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	p := NewPiProvider()
	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	// Pre-existing AWS var must not be overwritten
	env := map[string]string{"AWS_PROFILE": "prod"}
	p.ContributeEnv(env)
	if env["AWS_PROFILE"] != "prod" {
		t.Errorf("AWS_PROFILE = %q, want prod (user-set value should be preserved)", env["AWS_PROFILE"])
	}
}

// TestPiAWSPointerEnv_NoAWSDir verifies no env vars are injected when ~/.aws is absent.
func TestPiAWSPointerEnv_NoAWSDir(t *testing.T) {
	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	// Ensure no AWS_* vars bleed in from the test runner's environment.
	for _, key := range []string{"AWS_PROFILE", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_DEFAULT_REGION"} {
		t.Setenv(key, "")
		t.Cleanup(func() { os.Unsetenv(key) }) //nolint:errcheck // cleanup only
	}

	got := piAWSPointerEnv()
	if got != nil {
		t.Errorf("piAWSPointerEnv() = %v, want nil when ~/.aws absent", got)
	}
}

// TestPiAWSPointerEnv_WithAWSDir verifies AWS_PROFILE and AWS_REGION are injected
// when ~/.aws exists and no AWS_* vars are set in the environment.
func TestPiAWSPointerEnv_WithAWSDir(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgContent := "[default]\nregion = us-east-1\noutput = json\n"
	if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	// Clear any ambient AWS_* from the test environment.
	for _, key := range []string{"AWS_PROFILE", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_DEFAULT_REGION"} {
		os.Unsetenv(key) //nolint:errcheck
	}

	got := piAWSPointerEnv()
	if len(got) == 0 {
		t.Fatal("piAWSPointerEnv() = nil, want non-empty slice when ~/.aws present")
	}
	wantProfile := "AWS_PROFILE=default"
	wantRegion := "AWS_REGION=us-east-1"
	hasProfile, hasRegion := false, false
	for _, e := range got {
		if e == wantProfile {
			hasProfile = true
		}
		if e == wantRegion {
			hasRegion = true
		}
	}
	if !hasProfile {
		t.Errorf("piAWSPointerEnv() missing %q; got %v", wantProfile, got)
	}
	if !hasRegion {
		t.Errorf("piAWSPointerEnv() missing %q; got %v", wantRegion, got)
	}
}

// TestPiAWSPointerEnv_SkipsWhenAWSEnvAlreadySet verifies no injection occurs
// when the daemon process already has an AWS_* variable set.
func TestPiAWSPointerEnv_SkipsWhenAWSEnvAlreadySet(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	// Pre-set an AWS env var — injection must be skipped.
	t.Setenv("AWS_PROFILE", "prod")

	got := piAWSPointerEnv()
	if got != nil {
		t.Errorf("piAWSPointerEnv() = %v, want nil when AWS_PROFILE already set", got)
	}
}

// TestPiContributeEnv_ProcessEnvAWSVarSkipped verifies that ContributeEnv does
// NOT inject AWS vars when any AWS_* variable is already in the daemon's process
// environment (not just in the agent env map). This is the fix for bug #2:
// previously ContributeEnv only checked the agent env map, so a process-level
// AWS_PROFILE=my-profile was silently overwritten with "default".
func TestPiContributeEnv_ProcessEnvAWSVarSkipped(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	p := NewPiProvider()
	orig := piUserHomeDir
	piUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { piUserHomeDir = orig })

	// Set a profile in the process environment; agent env map is empty.
	t.Setenv("AWS_PROFILE", "my-profile")

	env := map[string]string{} // no AWS_ in agent env
	p.ContributeEnv(env)
	if _, ok := env["AWS_PROFILE"]; ok {
		t.Error("ContributeEnv must not inject AWS_PROFILE when AWS_PROFILE is already in os.Environ()")
	}
}

// TestPiListModels_AllUnparseableFallback verifies that when `pi --list-models`
// exits 0 but emits no parseable two-column rows, ListModels returns the static
// fallback (p.Models()) rather than nil (bug #4).
func TestPiListModels_AllUnparseableFallback(t *testing.T) {
	p := NewPiProvider()
	orig := piListModels
	t.Cleanup(func() { piListModels = orig })

	// Output is non-empty but every row has only one column — all skipped.
	piListModels = func(_ context.Context) (string, error) {
		return "MODELS\nNOTE: run pi --setup to configure providers\n", nil
	}
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() unexpected error: %v", err)
	}
	// pi's static Models() is empty, so we expect empty (not nil).
	want := p.Models()
	if len(got) != len(want) {
		t.Errorf("ListModels() = %v (len=%d), want static fallback %v (len=%d)", got, len(got), want, len(want))
	}
}

func TestReadAWSDefaultRegion(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "default profile with region",
			content: "[default]\nregion = eu-central-1\n",
			want:    "eu-central-1",
		},
		{
			name:    "default profile first then other",
			content: "[default]\nregion = ap-southeast-1\n\n[profile staging]\nregion = us-east-1\n",
			want:    "ap-southeast-1",
		},
		{
			name:    "no default profile",
			content: "[profile staging]\nregion = us-east-1\n",
			want:    "",
		},
		{
			name:    "empty config",
			content: "",
			want:    "",
		},
		{
			name:    "default profile without region",
			content: "[default]\noutput = json\n",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := filepath.Join(awsDir, "config")
			if err := os.WriteFile(cfgPath, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got := readAWSDefaultRegion(home)
			if got != tt.want {
				t.Errorf("readAWSDefaultRegion() = %q, want %q", got, tt.want)
			}
		})
	}
}
