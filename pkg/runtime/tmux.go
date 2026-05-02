package runtime

import (
	"context"
	"os/exec"

	"github.com/rpuneet/mycel/pkg/tmux"
)

// TmuxBackend wraps a tmux.Manager to implement the Backend interface.
type TmuxBackend struct {
	mgr *tmux.Manager
}

// Ensure TmuxBackend implements Backend.
var _ Backend = (*TmuxBackend)(nil)

// NewTmuxBackend creates a Backend backed by a tmux.Manager.
func NewTmuxBackend(mgr *tmux.Manager) *TmuxBackend {
	return &TmuxBackend{mgr: mgr}
}

// TmuxManager returns the underlying tmux.Manager.
// Use this when tmux-specific operations are needed.
func (t *TmuxBackend) TmuxManager() *tmux.Manager {
	return t.mgr
}

func (t *TmuxBackend) HasSession(ctx context.Context, name string) bool {
	return t.mgr.HasSession(ctx, name)
}

func (t *TmuxBackend) CreateSession(ctx context.Context, name, dir string) error {
	return t.mgr.CreateSession(ctx, name, dir)
}

func (t *TmuxBackend) CreateSessionWithCommand(ctx context.Context, name, dir, command string) error {
	return t.mgr.CreateSessionWithCommand(ctx, name, dir, command)
}

func (t *TmuxBackend) CreateSessionWithEnv(ctx context.Context, name, dir, command string, env map[string]string) error {
	return t.mgr.CreateSessionWithEnv(ctx, name, dir, command, env)
}

func (t *TmuxBackend) KillSession(ctx context.Context, name string) error {
	return t.mgr.KillSession(ctx, name)
}

func (t *TmuxBackend) RenameSession(ctx context.Context, oldName, newName string) error {
	return t.mgr.RenameSession(ctx, oldName, newName)
}

func (t *TmuxBackend) SendKeys(ctx context.Context, name, keys string) error {
	return t.mgr.SendKeys(ctx, name, keys)
}

func (t *TmuxBackend) SendKeysWithSubmit(ctx context.Context, name, keys, submitKey string) error {
	return t.mgr.SendKeysWithSubmit(ctx, name, keys, submitKey)
}

func (t *TmuxBackend) Capture(ctx context.Context, name string, lines int) (string, error) {
	return t.mgr.Capture(ctx, name, lines)
}

func (t *TmuxBackend) ListSessions(ctx context.Context) ([]Session, error) {
	tmuxSessions, err := t.mgr.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, len(tmuxSessions))
	for i, s := range tmuxSessions {
		sessions[i] = Session{
			Name:      s.Name,
			Created:   s.Created,
			Directory: s.Directory,
			Attached:  s.Attached,
		}
	}
	return sessions, nil
}

func (t *TmuxBackend) AttachCmd(ctx context.Context, name string) *exec.Cmd {
	return t.mgr.AttachCmd(ctx, name)
}

func (t *TmuxBackend) IsRunning(ctx context.Context) bool {
	return t.mgr.IsRunning(ctx)
}

func (t *TmuxBackend) KillServer(ctx context.Context) error {
	return t.mgr.KillServer(ctx)
}

func (t *TmuxBackend) SetEnvironment(ctx context.Context, name, key, value string) error {
	return t.mgr.SetEnvironment(ctx, name, key, value)
}

func (t *TmuxBackend) SessionName(name string) string {
	return t.mgr.SessionName(name)
}

func (t *TmuxBackend) PipePane(ctx context.Context, name, logPath string) error {
	return t.mgr.PipePane(ctx, name, logPath)
}
