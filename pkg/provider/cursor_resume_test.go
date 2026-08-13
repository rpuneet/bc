package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorBuildCommandResume(t *testing.T) {
	p := NewCursorProvider()
	tests := []struct { //nolint:govet // test table; field order matches literals
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "fresh start",
			opts: CommandOpts{},
			want: "cursor-agent --trust",
		},
		{
			name: "continue previous",
			opts: CommandOpts{Resume: true},
			want: "cursor-agent --trust --continue",
		},
		{
			name: "resume by id",
			opts: CommandOpts{SessionID: "ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f"},
			want: "cursor-agent --trust --resume ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f",
		},
		{
			name: "session id wins over continue",
			opts: CommandOpts{
				SessionID: "ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f",
				Resume:    true,
			},
			want: "cursor-agent --trust --resume ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f",
		},
		{
			name: "unsafe session id dropped — continue still applies",
			opts: CommandOpts{SessionID: "$(rm -rf /)", Resume: true},
			want: "cursor-agent --trust --continue",
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

func TestCursorParseSessionID(t *testing.T) {
	p := NewCursorProvider()
	const id = "ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f"
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"resume hint", "agent --resume " + id + "\n", id},
		{"json session_id", `{"session_id":"` + id + `","event":"Stop"}`, id},
		{"empty", "no session here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.ParseSessionID(tt.output); got != tt.want {
				t.Errorf("ParseSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
	if !p.SupportsResume() {
		t.Error("SupportsResume() must be true")
	}
}

func TestLatestCursorSessionID(t *testing.T) {
	root := t.TempDir()
	agent := "fast-crane"
	dir := filepath.Join(root, agent, CursorUsageRelDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, CursorUsageFile)
	body := `{"session_id":"old-sess","input_tokens":1,"output_tokens":1}
{"session_id":"ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f","input_tokens":2,"output_tokens":2}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LatestCursorSessionID(root, agent)
	want := "ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f"
	if got != want {
		t.Errorf("LatestCursorSessionID() = %q, want %q", got, want)
	}
	if LatestCursorSessionID(root, "missing") != "" {
		t.Error("missing agent should return empty")
	}
}
