package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/home"
)

func TestAgentFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/_mcp/zen-zebra", "zen-zebra"},
		{"/_mcp/zen-zebra/", "zen-zebra"},
		{"/_mcp/zen-zebra/sse", "zen-zebra"}, // stale pre-rebuild config
		{"/_mcp/agent_1", "agent_1"},
		{"/_mcp/", ""},
		{"/_mcp", ""},
		{"/_mcp/a/b", ""},
		{"/_mcp/../etc", ""},
		{"/_mcp/a%20b", ""},
		{"/api/agents", ""},
	}
	for _, tt := range tests {
		if got := agentFromPath(tt.path); got != tt.want {
			t.Errorf("agentFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestNewRequiresRepo(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New with no home should error")
	}
}

// ── E2E over streamable HTTP ────────────────────────────────────────────────

// sendStub is a gateway adapter that records outbound sends.
//
//nolint:govet // fieldalignment: test-only struct
type sendStub struct {
	name  string
	calls []sendCall
}

type sendCall struct {
	ChannelID string
	Sender    string
	Content   string
}

func (s *sendStub) Name() string                                                { return s.name }
func (s *sendStub) Type() gateway.AdapterType                                   { return gateway.AdapterSocket }
func (s *sendStub) Start(_ context.Context, _ func(gateway.Notification)) error { return nil }
func (s *sendStub) Stop() error                                                 { return nil }
func (s *sendStub) HTTPHandler() http.Handler                                   { return nil }
func (s *sendStub) Channels() []gateway.ChannelInfo                             { return nil }
func (s *sendStub) Status() gateway.AdapterStatus                               { return gateway.AdapterStatus{} }

func (s *sendStub) Send(_ context.Context, channelID, sender, content string) error {
	s.calls = append(s.calls, sendCall{channelID, sender, content})
	return nil
}

// newTestSession spins up the handler on an httptest server and connects an
// SDK client as the given agent. Callers must close the returned session.
func newTestSession(t *testing.T, cfg Config, agentName string) (*sdk.ClientSession, *httptest.Server) {
	t.Helper()
	h, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0"}, nil)
	session, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{
		Endpoint: srv.URL + "/_mcp/" + agentName,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() }) //nolint:errcheck
	return session, srv
}

func testRepo(t *testing.T) *home.Home {
	t.Helper()
	// Isolate global state (~/.mycel prefs + db) per test.
	t.Setenv("MYCEL_HOME", t.TempDir())
	dir := t.TempDir()
	//nolint:gosec // dir is a t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	h, err := home.Open(dir)
	if err != nil {
		t.Fatalf("home.Open: %v", err)
	}
	return h
}

func TestE2E_ListTools(t *testing.T) {
	session, _ := newTestSession(t, Config{Home: testRepo(t)}, "test-agent")

	res, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{
		"whoami": false, "list_agents": false, "list_channels": false,
		"read_channel": false, "send_message": false, "send_file": false,
		"report_status": false, "query_costs": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
		}
		want[tool.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not listed", name)
		}
	}
}

func TestE2E_Whoami(t *testing.T) {
	session, _ := newTestSession(t, Config{Home: testRepo(t)}, "zen-zebra")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami errored: %v", res.Content)
	}
	out := structured(t, res)
	if out["agent"] != "zen-zebra" {
		t.Errorf("agent = %v, want zen-zebra", out["agent"])
	}
	if out["display_name"] != "Zen Zebra" {
		t.Errorf("display_name = %v, want Zen Zebra", out["display_name"])
	}
	// The identity must carry an avatar URL for this agent's AgentCharacter.
	avatarURL, _ := out["avatar_url"].(string)
	if avatarURL == "" || !strings.Contains(avatarURL, "zen-zebra") || !strings.Contains(avatarURL, "avatar") {
		t.Errorf("avatar_url = %q, want a zen-zebra avatar URL", avatarURL)
	}
	// The Slack hint tells the agent how to post as itself.
	slack, ok := out["slack"].(map[string]any)
	if !ok {
		t.Fatalf("slack hint missing or wrong type: %T", out["slack"])
	}
	if slack["username"] != "zen-zebra" {
		t.Errorf("slack.username = %v, want zen-zebra", slack["username"])
	}
	if slack["scope"] != "chat:write.customize" {
		t.Errorf("slack.scope = %v, want chat:write.customize", slack["scope"])
	}
}

// TestE2E_Whoami_PublicAvatar verifies avatar_url prefers a configured public
// base (Slack-fetchable) over the loopback endpoint.
func TestE2E_Whoami_PublicAvatar(t *testing.T) {
	t.Setenv("MYCEL_AVATAR_PUBLIC_BASE", "https://bc-infra.com/avatars")
	session, _ := newTestSession(t, Config{Home: testRepo(t)}, "zen-zebra")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	out := structured(t, res)
	if got, want := out["avatar_url"], "https://bc-infra.com/avatars/zen-zebra.png"; got != want {
		t.Errorf("avatar_url = %v, want %v", got, want)
	}
	slack, _ := out["slack"].(map[string]any)
	if got, want := slack["icon_url"], "https://bc-infra.com/avatars/zen-zebra.png"; got != want {
		t.Errorf("slack.icon_url = %v, want %v", got, want)
	}
}

func TestE2E_SendMessage_IdentityEnforced(t *testing.T) {
	stub := &sendStub{name: "slack"}
	mgr := gateway.NewManager()
	mgr.Register(stub)
	// Seed the channel route via an inbound notification so Send can resolve it.
	mgr.HandleNotification("slack", gateway.Notification{
		Channel:   "general",
		ChannelID: "C1234",
		Platform:  "slack",
		Sender:    "bot",
		Content:   "hello",
	})

	session, _ := newTestSession(t, Config{Home: testRepo(t), Gateway: mgr}, "zen-zebra")

	// The client claims to be someone else — the path identity must win.
	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name: "send_message",
		Arguments: map[string]any{
			"channel": "slack:general",
			"message": "hello world",
			"sender":  "root",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("send_message errored: %v", res.Content)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("got %d sends, want 1", len(stub.calls))
	}
	if stub.calls[0].Sender != "zen-zebra" {
		t.Errorf("sender = %q, want zen-zebra (client-supplied sender must be ignored)", stub.calls[0].Sender)
	}
	if stub.calls[0].Content != "hello world" {
		t.Errorf("content = %q", stub.calls[0].Content)
	}
}

func TestE2E_SendMessage_NonGatewayChannel(t *testing.T) {
	session, _ := newTestSession(t, Config{Home: testRepo(t), Gateway: gateway.NewManager()}, "zen-zebra")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "send_message",
		Arguments: map[string]any{"channel": "nope:missing", "message": "x"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("send to unknown channel should be a tool error")
	}
}

func TestE2E_ToolErrorsWhenDependencyMissing(t *testing.T) {
	// No gateway, notify, agents, or costs configured — every dependent tool
	// must degrade to a tool error, never a transport failure.
	session, _ := newTestSession(t, Config{Home: testRepo(t)}, "zen-zebra")

	for _, call := range []*sdk.CallToolParams{
		{Name: "list_channels"},
		{Name: "read_channel", Arguments: map[string]any{"channel": "slack:eng"}},
		{Name: "send_message", Arguments: map[string]any{"channel": "slack:eng", "message": "x"}},
		{Name: "list_agents"},
		{Name: "report_status", Arguments: map[string]any{"task": "testing"}},
		{Name: "query_costs"},
	} {
		res, err := session.CallTool(t.Context(), call)
		if err != nil {
			t.Fatalf("%s: transport error: %v", call.Name, err)
		}
		if !res.IsError {
			t.Errorf("%s: want tool error with missing dependency", call.Name)
		}
	}
}

func TestHTTP_InvalidPaths404(t *testing.T) {
	h, err := New(Config{Home: testRepo(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{"/_mcp/", "/_mcp/a/b/c"} {
		resp, err := http.Get(srv.URL + path) //nolint:noctx // test helper
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("GET %s content-type = %q, want JSON", path, ct)
		}
	}
}

// TestValidateFilePath_SymlinkEscape ensures a symlink inside the repo
// cannot point send_file at a host file outside the allowed roots.
func TestValidateFilePath_SymlinkEscape(t *testing.T) {
	h := testRepo(t)
	cfg := Config{Home: h}

	// A real file inside the repo passes.
	inside := filepath.Join(h.RootDir, "ok.txt")
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFilePath(cfg, inside); err != nil {
		t.Errorf("in-repo file rejected: %v", err)
	}

	// A file outside every allowed root, reachable only via a symlink planted
	// inside the repo — must be rejected. t.TempDir() can live under
	// /tmp (an allowed root) on Linux, so use /etc/passwd as the target.
	const outside = "/etc/passwd"
	link := filepath.Join(h.RootDir, "innocent.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFilePath(cfg, link); err == nil {
		t.Error("symlink escaping the repo root was accepted")
	}

	// A direct path outside the roots is rejected too.
	if _, err := validateFilePath(cfg, outside); err == nil {
		t.Error("path outside repo root was accepted")
	}
}

// structured unmarshals the tool result's structured content into a map.
func structured(t *testing.T, res *sdk.CallToolResult) map[string]any {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return m
}
