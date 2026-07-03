package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server/mcp"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// makeWorkspace creates a minimal workspace directory suitable for tests.
func makeWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// workspace.Load requires a git repo
	if out, err := exec.CommandContext(context.Background(), "git", "init", dir).CombinedOutput(); err != nil { //nolint:gosec // dir is a t.TempDir(), not user input
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("MYCEL_HOME", t.TempDir())
	stateDir, sdErr := workspace.GlobalStateDir(dir)
	if sdErr != nil {
		t.Fatal(sdErr)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "roles"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatal(err)
	}
	// Minimal JSON config satisfying workspace.Load validation:
	// version = 2 and at least one provider defined.
	cfg := `{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`
	if err := os.WriteFile(filepath.Join(stateDir, workspace.PreferencesFileName), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newTestServer creates a Server backed by a temporary workspace.
func newTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	wsDir := makeWorkspace(t)

	ws, err := workspace.Load(wsDir)
	if err != nil {
		t.Fatalf("failed to load workspace: %v", err)
	}

	srv, err := mcp.New(mcp.Config{Workspace: ws, Version: "test"})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// rpc sends a JSON-RPC request to Handle and decodes the result into dst.
func rpc(t *testing.T, srv *mcp.Server, method string, params any, dst any) {
	t.Helper()
	id := json.RawMessage(`1`)
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		rawParams = b
	}
	req := mcp.Request{JSONRPC: "2.0", ID: &id, Method: method, Params: rawParams}
	resp := srv.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error for %s: %s (code %d)", method, resp.Error.Message, resp.Error.Code)
	}
	if dst == nil {
		return
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-marshal result: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal result into %T: %v", dst, err)
	}
}

// rpcErr sends a request and asserts it returns an error with the given code.
func rpcErr(t *testing.T, srv *mcp.Server, method string, params any, wantCode int) {
	t.Helper()
	id := json.RawMessage(`1`)
	var rawParams json.RawMessage
	if params != nil {
		b, _ := json.Marshal(params)
		rawParams = b
	}
	req := mcp.Request{JSONRPC: "2.0", ID: &id, Method: method, Params: rawParams}
	resp := srv.Handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatalf("expected error for %s but got result", method)
	}
	if resp.Error.Code != wantCode {
		t.Fatalf("want error code %d, got %d (%s)", wantCode, resp.Error.Code, resp.Error.Message)
	}
}

// ─── protocol / dispatch ─────────────────────────────────────────────────────

func TestHandle_UnknownMethod(t *testing.T) {
	srv := newTestServer(t)
	rpcErr(t, srv, "no_such_method", nil, mcp.ErrMethodNotFound)
}

func TestHandle_Notification_NoResponse(t *testing.T) {
	srv := newTestServer(t)
	// "initialized" is a notification (no ID) — Handle should return empty Response
	req := mcp.Request{JSONRPC: "2.0", Method: "initialized"}
	resp := srv.Handle(context.Background(), req)
	if resp.Error != nil || resp.Result != nil {
		t.Fatal("notification should produce empty response")
	}
}

// ─── initialize ──────────────────────────────────────────────────────────────

func TestInitialize(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Capabilities struct {
			Resources json.RawMessage `json:"resources"`
			Tools     json.RawMessage `json:"tools"`
		} `json:"capabilities"`
	}
	rpc(t, srv, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client"},
	}, &result)

	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocolVersion = %q, want 2024-11-05", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "bc" {
		t.Errorf("serverInfo.name = %q, want bc", result.ServerInfo.Name)
	}
	if result.ServerInfo.Version != "test" {
		t.Errorf("serverInfo.version = %q, want test", result.ServerInfo.Version)
	}
	if result.Capabilities.Resources == nil {
		t.Error("capabilities.resources should be present")
	}
	if result.Capabilities.Tools == nil {
		t.Error("capabilities.tools should be present")
	}
}

// ─── resources/list ──────────────────────────────────────────────────────────

func TestResourcesList(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Resources []mcp.Resource `json:"resources"`
	}
	rpc(t, srv, "resources/list", nil, &result)

	wantURIs := []string{
		"bc://workspace/status",
		"bc://agents",
		"bc://channels",
		"bc://costs",
		"bc://roles",
		"bc://tools",
	}
	got := make(map[string]bool, len(result.Resources))
	for _, r := range result.Resources {
		got[r.URI] = true
	}
	for _, uri := range wantURIs {
		if !got[uri] {
			t.Errorf("resources/list missing URI %q", uri)
		}
	}
	if len(result.Resources) != len(wantURIs) {
		t.Errorf("resources/list returned %d resources, want %d", len(result.Resources), len(wantURIs))
	}
}

// ─── resources/read ──────────────────────────────────────────────────────────

func readResource(t *testing.T, srv *mcp.Server, uri string) string {
	t.Helper()
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	rpc(t, srv, "resources/read", map[string]string{"uri": uri}, &result)
	if len(result.Contents) != 1 {
		t.Fatalf("%s: expected 1 content item, got %d", uri, len(result.Contents))
	}
	c := result.Contents[0]
	if c.URI != uri {
		t.Errorf("%s: content URI mismatch: got %q", uri, c.URI)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("%s: mimeType = %q, want application/json", uri, c.MIMEType)
	}
	return c.Text
}

func TestResourceRead_WorkspaceStatus(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://workspace/status")

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("workspace/status: invalid JSON: %v", err)
	}
	for _, key := range []string{"name", "path", "state_dir", "agents_dir"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("workspace/status: missing key %q", key)
		}
	}
}

func TestResourceRead_Agents_EmptyWorkspace(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://agents")

	var agents []any
	if err := json.Unmarshal([]byte(text), &agents); err != nil {
		t.Fatalf("bc://agents: invalid JSON: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents in fresh workspace, got %d", len(agents))
	}
}

func TestResourceRead_Channels_EmptyWorkspace(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://channels")

	var channels []any
	if err := json.Unmarshal([]byte(text), &channels); err != nil {
		t.Fatalf("bc://channels: invalid JSON: %v", err)
	}
	// Fresh workspace creates 3 default channels: general, engineering, all
	if len(channels) != 0 {
		t.Errorf("expected 0 channels in fresh workspace (no defaults), got %d", len(channels))
	}
}

func TestResourceRead_Costs(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://costs")

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("bc://costs: invalid JSON: %v", err)
	}
	if _, ok := payload["workspace"]; !ok {
		t.Error("bc://costs: missing 'workspace' key")
	}
}

func TestResourceRead_Roles(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://roles")

	// Fresh workspace has no role files — verify valid JSON array.
	var roles []any
	if err := json.Unmarshal([]byte(text), &roles); err != nil {
		t.Fatalf("bc://roles: invalid JSON: %v", err)
	}
}

func TestResourceRead_Tools(t *testing.T) {
	srv := newTestServer(t)
	text := readResource(t, srv, "bc://tools")

	var tools []struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal([]byte(text), &tools); err != nil {
		t.Fatalf("bc://tools: invalid JSON: %v", err)
	}
	if len(tools) == 0 {
		t.Error("bc://tools: expected at least one tool")
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"claude", "gemini", "cursor"} {
		if !names[want] {
			t.Errorf("bc://tools: missing tool %q", want)
		}
	}
}

func TestResourceRead_UnknownURI(t *testing.T) {
	srv := newTestServer(t)
	rpcErr(t, srv, "resources/read",
		map[string]string{"uri": "bc://no_such_resource"},
		mcp.ErrInvalidParams)
}

func TestResourceRead_InvalidParams(t *testing.T) {
	srv := newTestServer(t)
	rpcErr(t, srv, "resources/read", "not an object", mcp.ErrInvalidParams)
}

// ─── tools/list ──────────────────────────────────────────────────────────────

func TestToolsList(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Tools []mcp.Tool `json:"tools"`
	}
	rpc(t, srv, "tools/list", nil, &result)

	// send_message, list_channels, and read_channel re-added on gateway/notify.
	wantNames := []string{"send_message", "list_channels", "read_channel", "send_file", "whoami", "list_agents"}
	got := make(map[string]bool)
	for _, tool := range result.Tools {
		got[tool.Name] = true
		if tool.InputSchema == nil {
			t.Errorf("tool %q missing inputSchema", tool.Name)
		}
	}
	for _, name := range wantNames {
		if !got[name] {
			t.Errorf("tools/list missing tool %q", name)
		}
	}
}

// ─── tools/call ──────────────────────────────────────────────────────────────

func TestToolCall_UnknownTool(t *testing.T) {
	srv := newTestServer(t)
	rpcErr(t, srv, "tools/call",
		map[string]any{"name": "no_such_tool", "arguments": map[string]any{}},
		mcp.ErrInvalidParams)
}

func TestToolCall_InvalidParams(t *testing.T) {
	srv := newTestServer(t)
	rpcErr(t, srv, "tools/call", "not an object", mcp.ErrInvalidParams)
}

func TestToolCall_Whoami(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpc(t, srv, "tools/call", map[string]any{
		"name":      "whoami",
		"arguments": map[string]any{},
	}, &result)

	if result.IsError {
		t.Fatal("whoami returned isError=true")
	}
	if len(result.Content) == 0 {
		t.Error("whoami returned no content")
	}
}

func TestToolCall_ListAgents(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpc(t, srv, "tools/call", map[string]any{
		"name":      "list_agents",
		"arguments": map[string]any{},
	}, &result)

	if result.IsError {
		t.Fatal("list_agents returned isError=true")
	}
	if len(result.Content) == 0 {
		t.Error("list_agents returned no content")
	}
}

// TestToolCall_SendFile_MissingFile verifies send_file returns an error when
// the file path does not exist (replaces deleted send_message/list_channels/read_channel tests).
func TestToolCall_SendFile_MissingFile(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpc(t, srv, "tools/call", map[string]any{
		"name": "send_file",
		"arguments": map[string]any{
			"channel":   "slack:general",
			"file_path": "/nonexistent/file.png",
		},
	}, &result)

	if !result.IsError {
		t.Fatal("expected isError=true when file does not exist")
	}
}

// ─── tool input length caps ──────────────────────────────────────────────────

// TestToolCall_InputLengthCaps verifies that MCP tool handlers reject
// oversized string fields. The /_mcp/* routes are exempt from the global
// MaxBodySize middleware, so per-tool caps are the only line of defense
// (see server/handlers/helpers.go MaxBodySize).
func TestToolCall_InputLengthCaps(t *testing.T) {
	srv := newTestServer(t)

	const (
		maxMessage = 64 * 1024
		maxChannel = 256
		maxSender  = 256
		maxRole    = 256
		maxPath    = 4 * 1024
		maxComment = 64 * 1024
	)

	tests := []struct {
		args    map[string]any
		name    string
		tool    string
		wantErr bool
	}{
		// send_message
		{
			name: "send_message under message cap",
			tool: "send_message",
			args: map[string]any{
				"channel": "slack:eng",
				"message": strings.Repeat("a", maxMessage),
			},
			// no gateway configured in test workspace → returns isError=true with
			// "no gateway configured" message; that's NOT a length-cap failure.
			wantErr: true,
		},
		{
			name: "send_message message over cap",
			tool: "send_message",
			args: map[string]any{
				"channel": "slack:eng",
				"message": strings.Repeat("a", maxMessage+1),
			},
			wantErr: true,
		},
		{
			name: "send_message channel over cap",
			tool: "send_message",
			args: map[string]any{
				"channel": strings.Repeat("c", maxChannel+1),
				"message": "hi",
			},
			wantErr: true,
		},
		{
			name: "send_message sender over cap",
			tool: "send_message",
			args: map[string]any{
				"channel": "slack:eng",
				"message": "hi",
				"sender":  strings.Repeat("s", maxSender+1),
			},
			wantErr: true,
		},
		// read_channel
		{
			name: "read_channel channel over cap",
			tool: "read_channel",
			args: map[string]any{
				"channel": strings.Repeat("c", maxChannel+1),
			},
			wantErr: true,
		},
		// list_agents
		{
			name: "list_agents role over cap",
			tool: "list_agents",
			args: map[string]any{
				"role": strings.Repeat("r", maxRole+1),
			},
			wantErr: true,
		},
		// send_file
		{
			name: "send_file channel over cap",
			tool: "send_file",
			args: map[string]any{
				"channel":   strings.Repeat("c", maxChannel+1),
				"file_path": "/tmp/x.png",
			},
			wantErr: true,
		},
		{
			name: "send_file file_path over cap",
			tool: "send_file",
			args: map[string]any{
				"channel":   "slack:eng",
				"file_path": strings.Repeat("p", maxPath+1),
			},
			wantErr: true,
		},
		{
			name: "send_file comment over cap",
			tool: "send_file",
			args: map[string]any{
				"channel":   "slack:eng",
				"file_path": "/tmp/x.png",
				"comment":   strings.Repeat("c", maxComment+1),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result struct {
				Content []mcp.ToolContent `json:"content"`
				IsError bool              `json:"isError"`
			}
			rpc(t, srv, "tools/call", map[string]any{
				"name":      tc.tool,
				"arguments": tc.args,
			}, &result)
			if result.IsError != tc.wantErr {
				t.Fatalf("isError = %v, want %v (content=%+v)", result.IsError, tc.wantErr, result.Content)
			}
		})
	}
}

// TestToolCall_InputLengthCaps_ErrorMessage verifies the exact error text
// indicates which field exceeded its cap, so callers can debug.
func TestToolCall_InputLengthCaps_ErrorMessage(t *testing.T) {
	srv := newTestServer(t)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpc(t, srv, "tools/call", map[string]any{
		"name": "send_message",
		"arguments": map[string]any{
			"channel": "slack:eng",
			"message": strings.Repeat("a", 64*1024+1),
		},
	}, &result)

	if !result.IsError {
		t.Fatal("expected isError=true for oversize message")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
	got := result.Content[0].Text
	if !strings.Contains(got, "message too long") {
		t.Errorf("error message %q does not mention which field exceeded cap", got)
	}
}

// ─── stdio transport ─────────────────────────────────────────────────────────

func TestStdio_RoundTrip(t *testing.T) {
	srv := newTestServer(t)

	id := json.RawMessage(`42`)
	req := mcp.Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{}}`),
	}
	reqBytes, _ := json.Marshal(req)

	in := bytes.NewReader(append(reqBytes, '\n'))
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := srv.ServeStdioRW(ctx, in, &out)
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("ServeStdioRW: %v", err)
	}

	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("no output from ServeStdioRW")
	}
	var resp mcp.Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v\nraw: %s", err, line)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %s", resp.Error.Message)
	}
}

func TestStdio_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	in := strings.NewReader("not valid json\n")
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = srv.ServeStdioRW(ctx, in, &out)

	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("no output for invalid JSON input")
	}
	var resp mcp.Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("expected JSON error response, got: %s", line)
	}
	if resp.Error == nil {
		t.Fatal("expected error response for invalid JSON")
	}
	if resp.Error.Code != mcp.ErrParse {
		t.Errorf("want ErrParse (%d), got %d", mcp.ErrParse, resp.Error.Code)
	}
}

func TestStdio_MultipleRequests(t *testing.T) {
	srv := newTestServer(t)

	// Send two requests back-to-back
	id1 := json.RawMessage(`1`)
	id2 := json.RawMessage(`2`)
	r1, _ := json.Marshal(mcp.Request{JSONRPC: "2.0", ID: &id1, Method: "tools/list"})
	r2, _ := json.Marshal(mcp.Request{JSONRPC: "2.0", ID: &id2, Method: "resources/list"})

	input := string(r1) + "\n" + string(r2) + "\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = srv.ServeStdioRW(ctx, in, &out)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		var resp mcp.Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i+1, err)
		}
		if resp.Error != nil {
			t.Errorf("line %d: unexpected error: %s", i+1, resp.Error.Message)
		}
	}
}

// ─── SSE transport ───────────────────────────────────────────────────────────

func TestSSE_LocalhostAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{":8811", "127.0.0.1:8811"},
		{":0", "127.0.0.1:0"},
		{"127.0.0.1:8811", "127.0.0.1:8811"},
		{"0.0.0.0:8811", "0.0.0.0:8811"}, // explicit all-interface preserved
	}
	for _, tc := range tests {
		got := mcp.LocalhostAddr(tc.input)
		if got != tc.want {
			t.Errorf("LocalhostAddr(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSSE_Health(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","server":"bc-mcp","version":"test"}`) //nolint:errcheck // writing to response
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSSE_Message_ResourcesList(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	id := json.RawMessage(`7`)
	mcpReq := mcp.Request{JSONRPC: "2.0", ID: &id, Method: "resources/list"}
	body, _ := json.Marshal(mcpReq)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/message", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /message: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", resp.StatusCode)
	}
}

func TestSSE_Message_WrongMethod(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/message", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /message: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestSSE_Broker_SendReceive(t *testing.T) {
	broker := mcp.NewSSEBroker()

	// Mount a fake SSE endpoint using httptest
	mux := http.NewServeMux()
	srv := newTestServer(t)
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// POST a notification (no ID) — broker should not send, server returns 202
	notif := mcp.Request{JSONRPC: "2.0", Method: "initialized"}
	body, _ := json.Marshal(notif)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/message", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST notification: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("notification: status = %d, want 202", resp.StatusCode)
	}
}
