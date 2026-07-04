package agent

import (
	"strings"
	"testing"

	pkgdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// TestValidateAgentToolsResolvesGlobalRoles is a regression test for the
// role-validation path reading the wrong store: roles live in the single
// global database, so validating an agent created against a repo with no
// local .bc/roles must still resolve a globally defined role.
func TestValidateAgentToolsResolvesGlobalRoles(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	// Seed the role in the global store (the only place roles live).
	wsDB, driver, err := pkgdb.Global(nil)
	if err != nil {
		t.Fatalf("db.Global: %v", err)
	}
	store, err := workspace.NewRoleStoreFromDB(wsDB.DB, driver)
	if err != nil {
		t.Fatalf("NewRoleStoreFromDB: %v", err)
	}
	if err := store.Save(&workspace.Role{Metadata: workspace.RoleMetadata{Name: "base"}}); err != nil {
		t.Fatalf("save role: %v", err)
	}

	// A repo with no .bc/roles at all — validation must still resolve the
	// global "base" role instead of reporting "role not found".
	repo := t.TempDir()
	issues := validateAgentTools(repo, "base")
	for _, issue := range issues {
		if strings.Contains(issue, "cannot resolve role") {
			t.Fatalf("validateAgentTools resolved roles from the repo-local store: %q", issue)
		}
	}
	if len(issues) != 0 {
		t.Fatalf("validateAgentTools reported unexpected issues: %v", issues)
	}
}

func TestRewriteDockerURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "localhost with port",
			in:   "http://localhost:9374/mcp/sse",
			want: "http://host.docker.internal:9374/mcp/sse",
		},
		{
			name: "127.0.0.1 with port",
			in:   "http://127.0.0.1:9374/mcp/sse",
			want: "http://host.docker.internal:9374/mcp/sse",
		},
		{
			name: "already remote host",
			in:   "http://myserver.example.com:9374/mcp/sse",
			want: "http://myserver.example.com:9374/mcp/sse",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "localhost no port",
			in:   "http://localhost/sse",
			want: "http://host.docker.internal/sse",
		},
		{
			name: "https localhost",
			in:   "https://localhost:8443/mcp",
			want: "https://host.docker.internal:8443/mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDockerURL(tt.in)
			if got != tt.want {
				t.Errorf("rewriteDockerURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestBcSelfURL verifies the bc MCP endpoint is derived from the live
// daemon address per runtime — never from the mcp_servers store, whose
// single static URL can't be right for both tmux and docker at once.
func TestBcSelfURL(t *testing.T) {
	tests := []struct {
		name    string
		bcdAddr string // BC_BCD_ADDR of the daemon process
		runtime string
		agent   string
		want    string
	}{
		{"tmux uses host loopback", "http://127.0.0.1:8080", "tmux", "zeta", "http://127.0.0.1:8080/_mcp/zeta/sse"},
		{"docker rewrites loopback", "http://127.0.0.1:8080", "docker", "zeta", "http://host.docker.internal:8080/_mcp/zeta/sse"},
		{"docker rewrites localhost", "http://localhost:9000", "docker", "a1", "http://host.docker.internal:9000/_mcp/a1/sse"},
		{"empty hostname normalized", "http://:8080", "tmux", "a1", "http://127.0.0.1:8080/_mcp/a1/sse"},
		{"no env falls back to default port tmux", "", "tmux", "a1", "http://127.0.0.1:9374/_mcp/a1/sse"},
		{"no env falls back to default port docker", "", "docker", "a1", "http://host.docker.internal:9374/_mcp/a1/sse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BC_BCD_ADDR", tt.bcdAddr)
			if got := bcSelfURL(tt.runtime, tt.agent); got != tt.want {
				t.Errorf("bcSelfURL(%q, %q) = %q, want %q", tt.runtime, tt.agent, got, tt.want)
			}
		})
	}
}
