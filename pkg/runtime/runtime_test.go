package runtime_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/rpuneet/mycel/pkg/runtime"
	"github.com/rpuneet/mycel/pkg/tmux"
)

// ---------------------------------------------------------------------------
// Test helper process — standard Go pattern for mocking exec.Command.
// ---------------------------------------------------------------------------

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if s := os.Getenv("MOCK_STDOUT"); s != "" {
		_, _ = fmt.Fprint(os.Stdout, s)
	}
	if s := os.Getenv("MOCK_STDERR"); s != "" {
		_, _ = fmt.Fprint(os.Stderr, s)
	}
	exitCode := 0
	if v := os.Getenv("MOCK_EXIT_CODE"); v != "" {
		var err error
		exitCode, err = strconv.Atoi(v)
		if err != nil {
			os.Exit(2)
		}
	}
	os.Exit(exitCode)
}

// mockCmd creates a mock execCommand function that returns a subprocess
// with the given stdout, stderr, and exit code.
func mockCmd(stdout, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		cs := make([]string, 0, 3+len(args))
		cs = append(cs, "-test.run=TestHelperProcess", "--", name)
		cs = append(cs, args...)
		cmd := exec.CommandContext(context.Background(), os.Args[0], cs...) //nolint:gosec
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_STDOUT=" + stdout,
			"MOCK_STDERR=" + stderr,
			fmt.Sprintf("MOCK_EXIT_CODE=%d", exitCode),
		}
		return cmd
	}
}

// newMockBackend creates a TmuxBackend with a mocked tmux.Manager.
func newMockBackend(prefix, stdout, stderr string, exitCode int) *runtime.TmuxBackend {
	mgr := tmux.NewManager(prefix).WithExecCommand(mockCmd(stdout, stderr, exitCode))
	return runtime.NewTmuxBackend(mgr)
}

// ---------------------------------------------------------------------------
// Session struct tests
// ---------------------------------------------------------------------------

func TestSessionStruct(t *testing.T) {
	s := runtime.Session{
		Name:      "test",
		Created:   "2024-01-01",
		Directory: "/tmp",
		Attached:  true,
	}
	if s.Name != "test" {
		t.Errorf("Name = %q, want %q", s.Name, "test")
	}
	if s.Created != "2024-01-01" {
		t.Errorf("Created = %q, want %q", s.Created, "2024-01-01")
	}
	if s.Directory != "/tmp" {
		t.Errorf("Directory = %q, want %q", s.Directory, "/tmp")
	}
	if !s.Attached {
		t.Error("Attached should be true")
	}
}

func TestSessionStructZeroValue(t *testing.T) {
	var s runtime.Session
	if s.Name != "" {
		t.Errorf("zero value Name = %q, want empty", s.Name)
	}
	if s.Attached {
		t.Error("zero value Attached should be false")
	}
}

// ---------------------------------------------------------------------------
// NewTmuxBackend and TmuxManager accessor tests
// ---------------------------------------------------------------------------

func TestNewTmuxBackend(t *testing.T) {
	mgr := tmux.NewManager("test-")
	backend := runtime.NewTmuxBackend(mgr)

	if backend == nil {
		t.Fatal("NewTmuxBackend returned nil")
	}
}

func TestTmuxBackendImplementsBackend(t *testing.T) {
	mgr := tmux.NewManager("test-")
	backend := runtime.NewTmuxBackend(mgr)

	var _ runtime.Backend = backend
}

func TestTmuxManager(t *testing.T) {
	mgr := tmux.NewManager("test-")
	backend := runtime.NewTmuxBackend(mgr)

	if backend.TmuxManager() != mgr {
		t.Error("TmuxManager() should return the underlying tmux manager")
	}
}

// ---------------------------------------------------------------------------
// SessionName tests
// ---------------------------------------------------------------------------

func TestTmuxBackendSessionName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		input  string
		want   string
	}{
		{
			name:   "standard prefix",
			prefix: "mycel-",
			input:  "agent-1",
			want:   "mycel-agent-1",
		},
		{
			name:   "empty prefix",
			prefix: "",
			input:  "agent-1",
			want:   "agent-1",
		},
		{
			name:   "custom prefix",
			prefix: "myapp-",
			input:  "worker",
			want:   "myapp-worker",
		},
		{
			name:   "empty name",
			prefix: "mycel-",
			input:  "",
			want:   "mycel-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := tmux.NewManager(tt.prefix)
			backend := runtime.NewTmuxBackend(mgr)

			got := backend.SessionName(tt.input)
			if got != tt.want {
				t.Errorf("SessionName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HasSession tests
// ---------------------------------------------------------------------------

func TestTmuxBackendHasSession(t *testing.T) {
	tests := []struct {
		name     string
		want     bool
		exitCode int
	}{
		{
			name:     "session exists",
			exitCode: 0,
			want:     true,
		},
		{
			name:     "session does not exist",
			exitCode: 1,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			got := backend.HasSession(ctx, "test-agent")
			if got != tt.want {
				t.Errorf("HasSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateSession tests
// ---------------------------------------------------------------------------

func TestTmuxBackendCreateSession(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		stderr   string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			dir:      "/tmp/workdir",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "success with empty dir",
			dir:      "",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "failure",
			dir:      "/tmp/workdir",
			exitCode: 1,
			stderr:   "duplicate session",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", tt.stderr, tt.exitCode)
			ctx := context.Background()

			err := backend.CreateSession(ctx, "agent-1", tt.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateSessionWithCommand tests
// ---------------------------------------------------------------------------

func TestTmuxBackendCreateSessionWithCommand(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		wantErr  bool
	}{
		{
			name:     "success",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "failure",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.CreateSessionWithCommand(ctx, "agent-1", "/tmp", "echo hello")
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSessionWithCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateSessionWithEnv tests
// ---------------------------------------------------------------------------

func TestTmuxBackendCreateSessionWithEnv(t *testing.T) {
	tests := []struct {
		env      map[string]string
		name     string
		exitCode int
		wantErr  bool
	}{
		{
			name:     "success with env",
			env:      map[string]string{"FOO": "bar", "BAZ": "qux"},
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "success with nil env",
			env:      nil,
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "invalid env var name",
			env:      map[string]string{"INVALID-KEY": "val"},
			exitCode: 0,
			wantErr:  true,
		},
		{
			name:     "env var with leading digit",
			env:      map[string]string{"1BAD": "val"},
			exitCode: 0,
			wantErr:  true,
		},
		{
			name:     "failure from tmux",
			env:      map[string]string{"OK_KEY": "val"},
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.CreateSessionWithEnv(ctx, "agent-1", "/tmp", "echo hi", tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSessionWithEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KillSession tests
// ---------------------------------------------------------------------------

func TestTmuxBackendKillSession(t *testing.T) {
	tests := []struct {
		name     string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "session not found",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.KillSession(ctx, "agent-1")
			if (err != nil) != tt.wantErr {
				t.Errorf("KillSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RenameSession tests
// ---------------------------------------------------------------------------

func TestTmuxBackendRenameSession(t *testing.T) {
	tests := []struct {
		name     string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "failure",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.RenameSession(ctx, "old-name", "new-name")
			if (err != nil) != tt.wantErr {
				t.Errorf("RenameSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SendKeys tests
// ---------------------------------------------------------------------------

func TestTmuxBackendSendKeys(t *testing.T) {
	tests := []struct {
		name     string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "failure",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.SendKeys(ctx, "agent-1", "echo hello")
			if (err != nil) != tt.wantErr {
				t.Errorf("SendKeys() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SendKeysWithSubmit tests
// ---------------------------------------------------------------------------

func TestTmuxBackendSendKeysWithSubmit(t *testing.T) {
	tests := []struct {
		name      string
		keys      string
		submitKey string
		wantErr   bool
		exitCode  int
	}{
		{
			name:      "success with Enter",
			keys:      "echo hello",
			submitKey: "Enter",
			exitCode:  0,
			wantErr:   false,
		},
		{
			name:      "success with empty submit key",
			keys:      "partial input",
			submitKey: "",
			exitCode:  0,
			wantErr:   false,
		},
		{
			name:      "success with custom submit key",
			keys:      "some text",
			submitKey: "C-m",
			exitCode:  0,
			wantErr:   false,
		},
		{
			name:      "failure",
			keys:      "echo hello",
			submitKey: "Enter",
			exitCode:  1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.SendKeysWithSubmit(ctx, "agent-1", tt.keys, tt.submitKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendKeysWithSubmit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Capture tests
// ---------------------------------------------------------------------------

func TestTmuxBackendCapture(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		wantErr  bool
		exitCode int
		lines    int
	}{
		{
			name:     "success",
			stdout:   "captured output line 1\ncaptured output line 2\n",
			exitCode: 0,
			lines:    100,
			wantErr:  false,
		},
		{
			name:     "success with zero lines",
			stdout:   "all output\n",
			exitCode: 0,
			lines:    0,
			wantErr:  false,
		},
		{
			name:     "failure",
			stdout:   "",
			exitCode: 1,
			lines:    50,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", tt.stdout, "", tt.exitCode)
			ctx := context.Background()

			output, err := backend.Capture(ctx, "agent-1", tt.lines)
			if (err != nil) != tt.wantErr {
				t.Errorf("Capture() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && output != tt.stdout {
				t.Errorf("Capture() = %q, want %q", output, tt.stdout)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListSessions tests
// ---------------------------------------------------------------------------

func TestTmuxBackendListSessions(t *testing.T) {
	tests := []struct {
		wantFirst *runtime.Session
		name      string
		stdout    string
		exitCode  int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "multiple sessions",
			stdout:    "mycel-agent-1|Mon Jan 1 00:00:00 2024|0|1|/tmp/a\nmycel-agent-2|Tue Jan 2 00:00:00 2024|1|2|/tmp/b\n",
			exitCode:  0,
			wantCount: 2,
			wantErr:   false,
			wantFirst: &runtime.Session{
				Name:      "agent-1",
				Created:   "Mon Jan 1 00:00:00 2024",
				Directory: "/tmp/a",
				Attached:  false,
			},
		},
		{
			name:      "no sessions (empty output)",
			stdout:    "",
			exitCode:  1, // tmux exits 1 when no server
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "sessions with different prefix filtered out",
			stdout:    "other-agent|Mon Jan 1|0|1|/tmp\nmycel-mine|Tue Jan 2|1|1|/home\n",
			exitCode:  0,
			wantCount: 1,
			wantErr:   false,
			wantFirst: &runtime.Session{
				Name:      "mine",
				Created:   "Tue Jan 2",
				Directory: "/home",
				Attached:  true,
			},
		},
		{
			name:      "malformed line skipped",
			stdout:    "mycel-good|date|0|1|/tmp\ntoo|few|fields\n",
			exitCode:  0,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", tt.stdout, "", tt.exitCode)
			ctx := context.Background()

			sessions, err := backend.ListSessions(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListSessions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(sessions) != tt.wantCount {
				t.Errorf("ListSessions() returned %d sessions, want %d", len(sessions), tt.wantCount)
			}
			if tt.wantFirst != nil && len(sessions) > 0 {
				got := sessions[0]
				if got.Name != tt.wantFirst.Name {
					t.Errorf("first session Name = %q, want %q", got.Name, tt.wantFirst.Name)
				}
				if got.Created != tt.wantFirst.Created {
					t.Errorf("first session Created = %q, want %q", got.Created, tt.wantFirst.Created)
				}
				if got.Directory != tt.wantFirst.Directory {
					t.Errorf("first session Directory = %q, want %q", got.Directory, tt.wantFirst.Directory)
				}
				if got.Attached != tt.wantFirst.Attached {
					t.Errorf("first session Attached = %v, want %v", got.Attached, tt.wantFirst.Attached)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AttachCmd tests
// ---------------------------------------------------------------------------

func TestTmuxBackendAttachCmd(t *testing.T) {
	backend := newMockBackend("mycel-", "", "", 0)
	ctx := context.Background()

	cmd := backend.AttachCmd(ctx, "agent-1")
	if cmd == nil {
		t.Fatal("AttachCmd() returned nil")
	}
}

// ---------------------------------------------------------------------------
// IsRunning tests
// ---------------------------------------------------------------------------

func TestTmuxBackendIsRunning(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		want     bool
		exitCode int
	}{
		{
			name:     "running",
			exitCode: 0,
			want:     true,
		},
		{
			name:     "not running - no server",
			exitCode: 1,
			stderr:   "no server running on /tmp/tmux-1000/default",
			want:     false,
		},
		{
			name:     "not running - other error",
			exitCode: 1,
			stderr:   "some other error",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", tt.stderr, tt.exitCode)
			ctx := context.Background()

			got := backend.IsRunning(ctx)
			if got != tt.want {
				t.Errorf("IsRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KillServer tests
// ---------------------------------------------------------------------------

func TestTmuxBackendKillServer(t *testing.T) {
	tests := []struct {
		name     string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "failure",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.KillServer(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("KillServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SetEnvironment tests
// ---------------------------------------------------------------------------

func TestTmuxBackendSetEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "success",
			key:      "MY_VAR",
			value:    "my_value",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "invalid key",
			key:      "BAD-KEY",
			value:    "val",
			exitCode: 0,
			wantErr:  true,
		},
		{
			name:     "key with leading digit",
			key:      "1BAD",
			value:    "val",
			exitCode: 0,
			wantErr:  true,
		},
		{
			name:     "underscore key",
			key:      "_VALID",
			value:    "val",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "tmux failure",
			key:      "GOOD_KEY",
			value:    "val",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.SetEnvironment(ctx, "agent-1", tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetEnvironment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PipePane tests
// ---------------------------------------------------------------------------

func TestTmuxBackendPipePane(t *testing.T) {
	tests := []struct {
		name     string
		logPath  string
		wantErr  bool
		exitCode int
	}{
		{
			name:     "start pipe",
			logPath:  "/tmp/agent.log",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "stop pipe",
			logPath:  "",
			exitCode: 0,
			wantErr:  false,
		},
		{
			name:     "start pipe failure",
			logPath:  "/tmp/agent.log",
			exitCode: 1,
			wantErr:  true,
		},
		{
			name:     "stop pipe failure",
			logPath:  "",
			exitCode: 1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockBackend("mycel-", "", "", tt.exitCode)
			ctx := context.Background()

			err := backend.PipePane(ctx, "agent-1", tt.logPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("PipePane() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Backend interface compliance with mock implementation
// ---------------------------------------------------------------------------

// mockBackend is a simple mock implementation of Backend for testing
// code that depends on the Backend interface.
type mockBackend struct {
	sendKeysErr           error
	captureErr            error
	pipePaneErr           error
	createSessionErr      error
	createSessionCmdErr   error
	createSessionEnvErr   error
	killSessionErr        error
	renameSessionErr      error
	setEnvironmentErr     error
	killServerErr         error
	sendKeysWithSubmitErr error
	listSessionsErr       error
	attachCmdResult       *exec.Cmd
	captureResult         string
	sessionNameResult     string
	listSessionsResult    []runtime.Session
	hasSessionResult      bool
	isRunningResult       bool
}

func (m *mockBackend) HasSession(_ context.Context, _ string) bool { return m.hasSessionResult }
func (m *mockBackend) CreateSession(_ context.Context, _, _ string) error {
	return m.createSessionErr
}
func (m *mockBackend) CreateSessionWithCommand(_ context.Context, _, _, _ string) error {
	return m.createSessionCmdErr
}
func (m *mockBackend) CreateSessionWithEnv(_ context.Context, _, _, _ string, _ map[string]string) error {
	return m.createSessionEnvErr
}
func (m *mockBackend) KillSession(_ context.Context, _ string) error { return m.killSessionErr }
func (m *mockBackend) RenameSession(_ context.Context, _, _ string) error {
	return m.renameSessionErr
}
func (m *mockBackend) SendKeys(_ context.Context, _, _ string) error { return m.sendKeysErr }
func (m *mockBackend) SendKeysWithSubmit(_ context.Context, _, _, _ string) error {
	return m.sendKeysWithSubmitErr
}
func (m *mockBackend) Capture(_ context.Context, _ string, _ int) (string, error) {
	return m.captureResult, m.captureErr
}
func (m *mockBackend) ListSessions(_ context.Context) ([]runtime.Session, error) {
	return m.listSessionsResult, m.listSessionsErr
}
func (m *mockBackend) AttachCmd(_ context.Context, _ string) *exec.Cmd { return m.attachCmdResult }
func (m *mockBackend) IsRunning(_ context.Context) bool                { return m.isRunningResult }
func (m *mockBackend) KillServer(_ context.Context) error              { return m.killServerErr }
func (m *mockBackend) SetEnvironment(_ context.Context, _, _, _ string) error {
	return m.setEnvironmentErr
}
func (m *mockBackend) SessionName(name string) string {
	if m.sessionNameResult != "" {
		return m.sessionNameResult
	}
	return "mock-" + name
}
func (m *mockBackend) PipePane(_ context.Context, _, _ string) error { return m.pipePaneErr }

func TestMockBackendImplementsInterface(t *testing.T) {
	var _ runtime.Backend = &mockBackend{}
}

func TestMockBackendUsage(t *testing.T) {
	t.Run("mock returns configured values", func(t *testing.T) {
		mock := &mockBackend{
			hasSessionResult:   true,
			captureResult:      "hello world",
			isRunningResult:    true,
			sessionNameResult:  "custom-name",
			listSessionsResult: []runtime.Session{{Name: "s1"}},
		}

		ctx := context.Background()

		if !mock.HasSession(ctx, "test") {
			t.Error("expected HasSession to return true")
		}
		output, err := mock.Capture(ctx, "test", 100)
		if err != nil || output != "hello world" {
			t.Error("expected Capture to return configured values")
		}
		if !mock.IsRunning(ctx) {
			t.Error("expected IsRunning to return true")
		}
		if mock.SessionName("x") != "custom-name" {
			t.Error("expected SessionName to return configured value")
		}
		sessions, err := mock.ListSessions(ctx)
		if err != nil || len(sessions) != 1 || sessions[0].Name != "s1" {
			t.Error("expected ListSessions to return configured values")
		}
	})

	t.Run("mock returns errors", func(t *testing.T) {
		mockErr := fmt.Errorf("mock error")
		mock := &mockBackend{
			createSessionErr:      mockErr,
			createSessionCmdErr:   mockErr,
			createSessionEnvErr:   mockErr,
			killSessionErr:        mockErr,
			renameSessionErr:      mockErr,
			sendKeysErr:           mockErr,
			sendKeysWithSubmitErr: mockErr,
			captureErr:            mockErr,
			listSessionsErr:       mockErr,
			killServerErr:         mockErr,
			setEnvironmentErr:     mockErr,
			pipePaneErr:           mockErr,
		}

		ctx := context.Background()

		if err := mock.CreateSession(ctx, "a", "/tmp"); err != mockErr {
			t.Error("expected CreateSession error")
		}
		if err := mock.CreateSessionWithCommand(ctx, "a", "/tmp", "cmd"); err != mockErr {
			t.Error("expected CreateSessionWithCommand error")
		}
		if err := mock.CreateSessionWithEnv(ctx, "a", "/tmp", "cmd", nil); err != mockErr {
			t.Error("expected CreateSessionWithEnv error")
		}
		if err := mock.KillSession(ctx, "a"); err != mockErr {
			t.Error("expected KillSession error")
		}
		if err := mock.RenameSession(ctx, "a", "b"); err != mockErr {
			t.Error("expected RenameSession error")
		}
		if err := mock.SendKeys(ctx, "a", "keys"); err != mockErr {
			t.Error("expected SendKeys error")
		}
		if err := mock.SendKeysWithSubmit(ctx, "a", "keys", "Enter"); err != mockErr {
			t.Error("expected SendKeysWithSubmit error")
		}
		if _, err := mock.Capture(ctx, "a", 100); err != mockErr {
			t.Error("expected Capture error")
		}
		if _, err := mock.ListSessions(ctx); err != mockErr {
			t.Error("expected ListSessions error")
		}
		if err := mock.KillServer(ctx); err != mockErr {
			t.Error("expected KillServer error")
		}
		if err := mock.SetEnvironment(ctx, "a", "k", "v"); err != mockErr {
			t.Error("expected SetEnvironment error")
		}
		if err := mock.PipePane(ctx, "a", "/tmp/log"); err != mockErr {
			t.Error("expected PipePane error")
		}
	})
}

// ---------------------------------------------------------------------------
// ListSessions conversion test — verifies tmux.Session to runtime.Session mapping
// ---------------------------------------------------------------------------

func TestListSessionsConversion(t *testing.T) {
	// Mock tmux output with attached and detached sessions
	sessionOutput := "mycel-root|Mon Jan 1 10:00:00 2024|1|3|/home/user/project\n" +
		"mycel-eng-01|Mon Jan 1 10:05:00 2024|0|1|/home/user/project/.bc/worktrees/eng-01\n"

	backend := newMockBackend("mycel-", sessionOutput, "", 0)
	ctx := context.Background()

	sessions, err := backend.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Check first session (attached)
	if sessions[0].Name != "root" {
		t.Errorf("sessions[0].Name = %q, want %q", sessions[0].Name, "root")
	}
	if !sessions[0].Attached {
		t.Error("sessions[0].Attached should be true")
	}
	if sessions[0].Directory != "/home/user/project" {
		t.Errorf("sessions[0].Directory = %q, want %q", sessions[0].Directory, "/home/user/project")
	}

	// Check second session (detached)
	if sessions[1].Name != "eng-01" {
		t.Errorf("sessions[1].Name = %q, want %q", sessions[1].Name, "eng-01")
	}
	if sessions[1].Attached {
		t.Error("sessions[1].Attached should be false")
	}
}
