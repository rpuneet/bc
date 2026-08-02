package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/mcp"
)

func writeMCPServers(t *testing.T, homeDir string, servers ...*mcp.ServerConfig) {
	t.Helper()
	store := mcp.NewGlobalStore(filepath.Join(homeDir, "mcps.json"))
	for _, s := range servers {
		if err := store.Add(s); err != nil {
			t.Fatalf("seed mcp server %q: %v", s.Name, err)
		}
	}
}

func TestCheckMCP_EmptyRegistry(t *testing.T) {
	h, _ := newBootstrappedHome(t)
	cat := CheckMCP(context.Background(), h)

	if len(cat.Items) != 1 {
		t.Fatalf("expected exactly one item, got %d: %+v", len(cat.Items), cat.Items)
	}
	if cat.Items[0].Severity != SeverityOK {
		t.Errorf("severity = %s, want ok", cat.Items[0].Severity)
	}
	if cat.Items[0].Message != "no MCP servers configured" {
		t.Errorf("message = %q", cat.Items[0].Message)
	}
}

func TestCheckMCP_StdioCommandFound(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	// "sh" is present on every CI runner / dev machine this test targets.
	present, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not on PATH, cannot test the happy path")
	}
	writeMCPServers(t, homeDir, &mcp.ServerConfig{
		Name: "shell", Transport: mcp.TransportStdio, Command: present, Enabled: true,
	})

	cat := CheckMCP(context.Background(), h)

	ok, warn, fail := cat.Counts()
	if fail != 0 || warn != 0 {
		t.Fatalf("expected no problems, got warn=%d fail=%d items=%+v", warn, fail, cat.Items)
	}
	if ok != 1 {
		t.Errorf("ok = %d, want 1", ok)
	}
	if !strings.Contains(cat.Items[0].Message, "1 of 1 MCP servers OK") {
		t.Errorf("summary message = %q", cat.Items[0].Message)
	}
}

func TestCheckMCP_StdioCommandMissing(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	writeMCPServers(t, homeDir, &mcp.ServerConfig{
		Name: "ghost", Transport: mcp.TransportStdio, Command: "definitely-not-a-real-binary-xyz", Enabled: true,
	})

	cat := CheckMCP(context.Background(), h)

	var found bool
	for _, item := range cat.Items {
		if item.Name == "mcp:ghost" {
			found = true
			if item.Severity != SeverityFail {
				t.Errorf("severity = %s, want fail", item.Severity)
			}
			if !strings.Contains(item.Message, "not found on PATH") {
				t.Errorf("message = %q", item.Message)
			}
			if item.Fix == "" {
				t.Errorf("expected a Fix hint")
			}
		}
	}
	if !found {
		t.Fatalf("expected an item for mcp:ghost, got %+v", cat.Items)
	}
	// Summary item should mention the failing server.
	if !strings.Contains(cat.Items[0].Message, "ghost") {
		t.Errorf("summary should mention the failing server, got %q", cat.Items[0].Message)
	}
}

func TestCheckMCP_DisabledServerSkipped(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	// A disabled server with a bogus command must NOT be flagged — it is
	// never spawned, so its command/URL health is irrelevant.
	writeMCPServers(t, homeDir, &mcp.ServerConfig{
		Name: "disabled-ghost", Transport: mcp.TransportStdio, Command: "no-such-binary-abc", Enabled: false,
	})

	cat := CheckMCP(context.Background(), h)

	_, warn, fail := cat.Counts()
	if warn != 0 || fail != 0 {
		t.Fatalf("disabled server should raise no problems, got %+v", cat.Items)
	}
	for _, item := range cat.Items {
		if item.Name == "mcp:disabled-ghost" {
			t.Errorf("disabled server should not produce a problem item: %+v", item)
		}
	}
	if !strings.Contains(cat.Items[0].Message, "1 of 1 MCP servers OK") {
		t.Errorf("summary = %q", cat.Items[0].Message)
	}
}

func TestCheckMCP_URLUnreachable(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	// A URL nothing listens on — connection should fail fast.
	writeMCPServers(t, homeDir, &mcp.ServerConfig{
		Name: "deadserver", Transport: mcp.TransportSSE, URL: "http://127.0.0.1:1/sse", Enabled: true,
	})

	cat := CheckMCP(context.Background(), h)

	var found bool
	for _, item := range cat.Items {
		if item.Name == "mcp:deadserver" {
			found = true
			if item.Severity != SeverityWarn {
				t.Errorf("severity = %s, want warn", item.Severity)
			}
			if !strings.Contains(item.Message, "unreachable") {
				t.Errorf("message = %q", item.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected an item for mcp:deadserver, got %+v", cat.Items)
	}
}

func TestCheckMCP_URLReachable(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	writeMCPServers(t, homeDir, &mcp.ServerConfig{
		Name: "liveserver", Transport: mcp.TransportSSE, URL: srv.URL, Enabled: true,
	})

	cat := CheckMCP(context.Background(), h)

	_, warn, fail := cat.Counts()
	if warn != 0 || fail != 0 {
		t.Fatalf("expected no problems for a reachable url, got %+v", cat.Items)
	}
}

func TestCheckMCP_MixedRegistry(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	present, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not on PATH")
	}

	writeMCPServers(t, homeDir,
		&mcp.ServerConfig{Name: "a-good-stdio", Transport: mcp.TransportStdio, Command: present, Enabled: true},
		&mcp.ServerConfig{Name: "b-bad-stdio", Transport: mcp.TransportStdio, Command: "no-such-binary-abc", Enabled: true},
		&mcp.ServerConfig{Name: "c-good-url", Transport: mcp.TransportSSE, URL: srv.URL, Enabled: true},
		&mcp.ServerConfig{Name: "d-bad-url", Transport: mcp.TransportSSE, URL: "http://127.0.0.1:1/sse", Enabled: true},
	)

	cat := CheckMCP(context.Background(), h)

	if !strings.Contains(cat.Items[0].Message, "2 of 4 MCP servers OK") {
		t.Errorf("summary = %q", cat.Items[0].Message)
	}
	ok, warn, fail := cat.Counts()
	// Summary item itself is OK, plus 2 problem items (1 warn, 1 fail).
	if ok != 1 || warn != 1 || fail != 1 {
		t.Errorf("counts ok=%d warn=%d fail=%d, want 1/1/1", ok, warn, fail)
	}
}
