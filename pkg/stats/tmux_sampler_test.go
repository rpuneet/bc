package stats

import (
	"context"
	"errors"
	"testing"
)

// fakeRunner is an in-memory TmuxProcRunner for tests. It lets a test
// declare a tmux session → pane PIDs mapping and a PID → children
// mapping, then answers queries deterministically without touching
// /usr/bin/tmux or /usr/bin/ps.
type fakeRunner struct {
	panePIDsErr  error
	psStatsErr   error
	listErr      error
	panes        map[string][]int
	children     map[int][]int
	cpuByPID     map[int]float64
	rssByPID     map[int]int64
	childrenErr  map[int]error
	sessions     []string
	panePIDCalls int
	psStatsCalls int
}

func (f *fakeRunner) PanePIDs(_ context.Context, session string) ([]int, error) {
	f.panePIDCalls++
	if f.panePIDsErr != nil {
		return nil, f.panePIDsErr
	}
	return f.panes[session], nil
}

func (f *fakeRunner) ListSessions(_ context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.sessions, nil
}

func (f *fakeRunner) Children(_ context.Context, pid int) ([]int, error) {
	if err, ok := f.childrenErr[pid]; ok {
		return nil, err
	}
	return f.children[pid], nil
}

func (f *fakeRunner) PSStats(_ context.Context, pids []int) (float64, int64, error) {
	f.psStatsCalls++
	if f.psStatsErr != nil {
		return 0, 0, f.psStatsErr
	}
	var cpu float64
	var rss int64
	for _, p := range pids {
		cpu += f.cpuByPID[p]
		rss += f.rssByPID[p]
	}
	return cpu, rss, nil
}

func TestTmuxSampler_SumsPIDTree(t *testing.T) {
	// Tree: pane (1000) → shell (1001) → claude (1002) → claude-worker (1003)
	r := &fakeRunner{
		panes: map[string][]int{"mycel-abc-eng-01": {1000}},
		children: map[int][]int{
			1000: {1001},
			1001: {1002},
			1002: {1003},
		},
		cpuByPID: map[int]float64{1000: 0.1, 1001: 0.2, 1002: 42.5, 1003: 12.0},
		rssByPID: map[int]int64{1000: 1_000_000, 1001: 2_000_000, 1002: 300_000_000, 1003: 50_000_000},
	}
	s := NewTmuxSampler(r)
	got, err := s.Sample(context.Background(), "mycel-abc-eng-01", "eng-01")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	wantCPU := 0.1 + 0.2 + 42.5 + 12.0
	if got.CPUPercent < wantCPU-0.01 || got.CPUPercent > wantCPU+0.01 {
		t.Errorf("CPUPercent = %v, want ~%v", got.CPUPercent, wantCPU)
	}
	wantMem := int64(1_000_000 + 2_000_000 + 300_000_000 + 50_000_000)
	if got.MemBytes != wantMem {
		t.Errorf("MemBytes = %d, want %d", got.MemBytes, wantMem)
	}
	if got.PIDsWalked != 4 {
		t.Errorf("PIDsWalked = %d, want 4", got.PIDsWalked)
	}
}

func TestTmuxSampler_StoppedAgentReturnsZero(t *testing.T) {
	// Session doesn't exist and list-sessions is empty: "agent not running".
	r := &fakeRunner{}
	s := NewTmuxSampler(r)
	got, err := s.Sample(context.Background(), "mycel-abc-ghost", "ghost")
	if err != nil {
		t.Fatalf("Sample: %v, want nil for stopped agent", err)
	}
	if got.CPUPercent != 0 || got.MemBytes != 0 {
		t.Errorf("stopped agent should be 0/0, got %+v", got)
	}
}

func TestTmuxSampler_PanePIDErrorFallsBackToListSessions(t *testing.T) {
	// First PanePIDs call errors (session literal name missed); list-sessions
	// finds the prefix-hashed variant and the retry succeeds.
	r := &fakeRunner{
		sessions: []string{"mycel-abc-eng-01"},
		panes: map[string][]int{
			"mycel-abc-eng-01": {2000},
		},
		children: map[int][]int{2000: {2001}},
		cpuByPID: map[int]float64{2000: 1, 2001: 9},
		rssByPID: map[int]int64{2000: 1024, 2001: 4096},
	}
	// Only the exact session string is indexed, so a lookup with the bare
	// agent name will return nil (empty panes) on the first call, and the
	// sampler should retry after list-sessions.
	s := NewTmuxSampler(r)
	got, err := s.Sample(context.Background(), "eng-01", "eng-01")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if got.CPUPercent != 10 {
		t.Errorf("CPUPercent = %v, want 10", got.CPUPercent)
	}
	if got.MemBytes != 5120 {
		t.Errorf("MemBytes = %d, want 5120", got.MemBytes)
	}
}

func TestTmuxSampler_NilRunnerReturnsError(t *testing.T) {
	var s *TmuxSampler
	_, err := s.Sample(context.Background(), "x", "x")
	if err == nil {
		t.Error("nil sampler should error")
	}
	s2 := &TmuxSampler{}
	_, err = s2.Sample(context.Background(), "x", "x")
	if err == nil {
		t.Error("sampler with nil runner should error")
	}
}

func TestTmuxSampler_ChildrenErrorPartialWalk(t *testing.T) {
	// Simulate pgrep failing for an intermediate PID — sampler keeps what it has.
	r := &fakeRunner{
		panes:    map[string][]int{"mycel-sess": {3000}},
		children: map[int][]int{3000: {3001}},
		childrenErr: map[int]error{
			3001: errors.New("pgrep crashed"),
		},
		cpuByPID: map[int]float64{3000: 0.5, 3001: 1.5},
		rssByPID: map[int]int64{3000: 500, 3001: 1500},
	}
	s := NewTmuxSampler(r)
	got, err := s.Sample(context.Background(), "mycel-sess", "sess")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	// We get pane + its first child, but not the second level (pgrep failed).
	if got.CPUPercent != 2.0 {
		t.Errorf("CPUPercent = %v, want 2.0", got.CPUPercent)
	}
	if got.PIDsWalked != 2 {
		t.Errorf("PIDsWalked = %d, want 2", got.PIDsWalked)
	}
}

func TestTmuxSampler_EmptyPIDsSkipsPSCall(t *testing.T) {
	// List-sessions returns a match but it has no panes — treat as stopped.
	r := &fakeRunner{
		sessions: []string{"mycel-abc-eng-01"},
		panes:    map[string][]int{}, // session exists but no panes
	}
	s := NewTmuxSampler(r)
	got, err := s.Sample(context.Background(), "mycel-abc-eng-01", "eng-01")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if got.CPUPercent != 0 || got.MemBytes != 0 {
		t.Errorf("no-pane session should be 0/0, got %+v", got)
	}
	if r.psStatsCalls != 0 {
		t.Errorf("expected no PS calls when there are no panes, got %d", r.psStatsCalls)
	}
}

func TestTmuxSampler_MaxDepthPreventsInfiniteLoop(t *testing.T) {
	// A runner that claims every PID has itself as a child; maxDepth must stop us.
	r := &fakeRunner{
		panes: map[string][]int{"sess": {5000}},
		children: map[int][]int{
			5000: {5001},
			5001: {5002},
			5002: {5003},
			5003: {5004},
			5004: {5005},
			5005: {5006},
			5006: {5007}, // would be depth 7, beyond maxDepth=6
		},
		cpuByPID: map[int]float64{5000: 1, 5001: 1, 5002: 1, 5003: 1, 5004: 1, 5005: 1, 5006: 1, 5007: 1},
	}
	s := NewTmuxSampler(r)
	s.maxDepth = 3
	got, err := s.Sample(context.Background(), "sess", "sess")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	// pane + 3 levels of descendants = 4 PIDs
	if got.PIDsWalked != 4 {
		t.Errorf("PIDsWalked = %d, want 4 (maxDepth=3)", got.PIDsWalked)
	}
}

func TestParsePIDList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []int
	}{
		{"empty", "", nil},
		{"single", "12345\n", []int{12345}},
		{"multiple", "100\n200\n300\n", []int{100, 200, 300}},
		{"blank lines", "100\n\n200\n", []int{100, 200}},
		{"with trailing fields", "100 extra\n200\t%CPU\n", []int{100, 200}},
		{"non-numeric ignored", "PID\n100\n", []int{100}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePIDList([]byte(tc.in))
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}
