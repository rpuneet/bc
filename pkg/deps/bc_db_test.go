package deps

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordedCall captures a single exec invocation so tests can assert on it.
type recordedCall struct {
	cmd  string
	args []string
}

// mockExec is an execRunner that records every call and replays scripted
// responses. Each response is matched positionally against the call sequence.
type mockExec struct {
	calls     []recordedCall
	responses []mockResponse
	index     int
}

type mockResponse struct {
	err error
	out []byte
}

func (m *mockExec) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, recordedCall{cmd: cmd, args: append([]string(nil), args...)})
	if m.index >= len(m.responses) {
		return nil, nil
	}
	r := m.responses[m.index]
	m.index++
	return r.out, r.err
}

func TestBCDBStatusRunning(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{out: []byte("true\n")}}}
	d := NewBCDBWithRunner(m)

	st, err := d.Status(context.Background())
	if err != nil {
		t.Fatalf("Status err: %v", err)
	}
	if st != StateRunning {
		t.Errorf("state = %v, want running", st)
	}
	if len(m.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(m.calls))
	}
	if m.calls[0].cmd != "docker" || m.calls[0].args[0] != "inspect" {
		t.Errorf("unexpected call: %+v", m.calls[0])
	}
}

func TestBCDBStatusNoContainer(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{err: errors.New("exit 1"), out: []byte("Error: No such object: bc-db\n")}}}
	d := NewBCDBWithRunner(m)

	st, err := d.Status(context.Background())
	if err != nil {
		t.Fatalf("Status err: %v", err)
	}
	if st != StateStopped {
		t.Errorf("state = %v, want stopped", st)
	}
}

func TestBCDBStartNewContainer(t *testing.T) {
	m := &mockExec{responses: []mockResponse{
		// inspect fails (not found)
		{err: errors.New("no such object"), out: []byte("Error: No such object")},
		// docker run succeeds
		{out: []byte("abc123\n")},
	}}
	d := NewBCDBWithRunner(m)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (inspect+run)", len(m.calls))
	}
	second := m.calls[1]
	if second.args[0] != "run" {
		t.Errorf("second call expected docker run, got %v", second.args)
	}
	joined := strings.Join(second.args, " ")
	for _, want := range []string{"--name", bcDBContainer, bcDBImage, "-p", "5432:5432"} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker run args missing %q: %s", want, joined)
		}
	}
}

func TestBCDBStartExistingStopped(t *testing.T) {
	m := &mockExec{responses: []mockResponse{
		{out: []byte("false\n")}, // inspect says exists, stopped
		{out: []byte("bc-db\n")}, // docker start
	}}
	d := NewBCDBWithRunner(m)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(m.calls))
	}
	if m.calls[1].args[0] != "start" {
		t.Errorf("second call should be docker start, got %v", m.calls[1].args)
	}
}

func TestBCDBStartAlreadyRunning(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{out: []byte("true\n")}}}
	d := NewBCDBWithRunner(m)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	if len(m.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (no-op when running)", len(m.calls))
	}
}

func TestBCDBStop(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{out: []byte("bc-db\n")}}}
	d := NewBCDBWithRunner(m)

	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop err: %v", err)
	}
	if m.calls[0].args[0] != "stop" {
		t.Errorf("expected docker stop, got %v", m.calls[0].args)
	}
}

func TestBCDBLogs(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{out: []byte("line1\nline2\nline3\n")}}}
	d := NewBCDBWithRunner(m)

	lines, err := d.Logs(context.Background(), 10)
	if err != nil {
		t.Fatalf("Logs err: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if lines[0] != "line1" || lines[2] != "line3" {
		t.Errorf("lines mismatch: %v", lines)
	}
}

func TestBCCodeServerStartSendsRepoMount(t *testing.T) {
	m := &mockExec{responses: []mockResponse{
		{}, // rm -f (ignored)
		{}, // run succeeds
	}}
	d := NewBCCodeServerWithRunner("/opt/repo", m)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (rm+run)", len(m.calls))
	}
	runArgs := strings.Join(m.calls[1].args, " ")
	if !strings.Contains(runArgs, "/opt/repo:/home/coder/workspace") {
		t.Errorf("run args missing repo mount: %s", runArgs)
	}
	if !strings.Contains(runArgs, "--auth=none") {
		t.Errorf("run args missing --auth=none: %s", runArgs)
	}
}

func TestBCCodeServerStartRequiresRepo(t *testing.T) {
	m := &mockExec{}
	d := NewBCCodeServerWithRunner("", m)
	if err := d.Start(context.Background()); err == nil {
		t.Error("expected Start to fail without a repo root")
	}
	if len(m.calls) != 0 {
		t.Errorf("unexpected calls: %v", m.calls)
	}
}

func TestBCCodeServerSetRepoRoot(t *testing.T) {
	m := &mockExec{responses: []mockResponse{{}, {}}}
	d := NewBCCodeServerWithRunner("/a", m)
	d.SetRepoRoot("/b")
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	runArgs := strings.Join(m.calls[1].args, " ")
	if !strings.Contains(runArgs, "/b:/home/coder/workspace") {
		t.Errorf("run args should use the updated repo: %s", runArgs)
	}
}
