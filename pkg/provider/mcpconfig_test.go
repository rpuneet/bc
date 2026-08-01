package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// forceCLIUnavailable makes the claude CLI lookup fail for the test so
// ReadMCPs exercises the .mcp.json fallback deterministically.
func forceCLIUnavailable(t *testing.T) {
	t.Helper()
	orig := claudeLookPath
	claudeLookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { claudeLookPath = orig })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const mcpJSONFixture = `{
	"mcpServers": {
		"remote": {"url": "http://localhost:9374/mcp/sse", "type": "sse"},
		"local": {"command": "npx", "args": ["-y", "some-server"]}
	}
}`

func TestClaudeCommands(t *testing.T) {
	cmds := NewClaudeProvider().Commands()
	if len(cmds) != 8 {
		t.Fatalf("len(Commands()) = %d, want 8", len(cmds))
	}
	if cmds[0].Name != "mcp add" || cmds[0].Args != "<name> <command|url>" {
		t.Errorf("first command = %+v, want mcp add", cmds[0])
	}
	for _, c := range cmds {
		if c.Name == "" || c.Command == "" || c.Description == "" {
			t.Errorf("command %+v has empty required field", c)
		}
	}
}

func TestParseClaudeMCPList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []MCPServerInfo
	}{
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "sse and stdio lines",
			output: "mycel: sse http://localhost:9374/mcp/sse\nfiles: stdio npx -y file-server\n",
			want: []MCPServerInfo{
				{Name: "mycel", Transport: "sse", URL: "http://localhost:9374/mcp/sse", Enabled: true},
				{Name: "files", Transport: "stdio", Command: "npx -y file-server", Enabled: true},
			},
		},
		{
			name:   "bare command defaults to stdio",
			output: "tool: /usr/local/bin/tool --serve",
			want: []MCPServerInfo{
				{Name: "tool", Transport: "stdio", Command: "/usr/local/bin/tool --serve", Enabled: true},
			},
		},
		{
			name:   "unparseable lines skipped",
			output: "no separator here",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClaudeMCPList(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d servers %+v, want %d", len(got), got, len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("server[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClaudeReadMCPsFileFallback(t *testing.T) {
	forceCLIUnavailable(t)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mcp.json"), mcpJSONFixture)

	got := NewClaudeProvider().ReadMCPs(context.Background(), root)
	if len(got) != 2 {
		t.Fatalf("got %d servers %+v, want 2", len(got), got)
	}
	// readMCPJSONFile sorts by name: local < remote.
	if got[0].Name != "local" || got[0].Transport != "stdio" || got[0].Command != "npx -y some-server" {
		t.Errorf("local = %+v", got[0])
	}
	if got[1].Name != "remote" || got[1].Transport != "sse" || got[1].URL != "http://localhost:9374/mcp/sse" {
		t.Errorf("remote = %+v", got[1])
	}
}

func TestClaudeReadMCPsNoRepo(t *testing.T) {
	forceCLIUnavailable(t)

	got := NewClaudeProvider().ReadMCPs(context.Background(), "")
	if got == nil || len(got) != 0 {
		t.Errorf("ReadMCPs with empty rootDir = %+v, want empty non-nil", got)
	}
}

func TestCursorReadMCPs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".cursor", "mcp.json"), mcpJSONFixture)

	p := NewCursorProvider()
	got := p.ReadMCPs(context.Background(), root)
	if len(got) != 2 {
		t.Fatalf("got %d servers %+v, want 2", len(got), got)
	}
	if got[0].Name != "local" || got[1].Name != "remote" {
		t.Errorf("servers not sorted by name: %+v", got)
	}

	if empty := p.ReadMCPs(context.Background(), ""); len(empty) != 0 {
		t.Errorf("empty rootDir = %+v, want empty", empty)
	}
	if missing := p.ReadMCPs(context.Background(), t.TempDir()); missing == nil || len(missing) != 0 {
		t.Errorf("missing file = %+v, want empty non-nil", missing)
	}
}

func TestReadMCPJSONFileMalformed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".mcp.json")
	writeFile(t, path, "{not json")

	if got := readMCPJSONFile(path); got == nil || len(got) != 0 {
		t.Errorf("malformed file = %+v, want empty non-nil", got)
	}
}
