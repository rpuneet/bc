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
