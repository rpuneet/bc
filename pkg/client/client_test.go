package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockServer creates a test server that returns the given response for any request.
func mockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// jsonHandler returns an HTTP handler that responds with the given JSON body and status.
func jsonHandler(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(body) //nolint:errcheck
	}
}

// capturingHandler captures the request method, path, and body, then responds with the given status and body.
func capturingHandler(t *testing.T, wantMethod string, status int, respBody any) (http.HandlerFunc, *capturedRequest) {
	t.Helper()
	cap := &capturedRequest{}
	return func(w http.ResponseWriter, r *http.Request) {
		cap.Method = r.Method
		cap.Path = r.URL.RequestURI()
		if r.Body != nil {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			cap.Body = body
		}
		if wantMethod != "" && r.Method != wantMethod {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respBody != nil {
			json.NewEncoder(w).Encode(respBody) //nolint:errcheck
		}
	}, cap
}

type capturedRequest struct {
	Body   map[string]any
	Method string
	Path   string
}

// --- Client core tests ---

func TestNew(t *testing.T) {
	c := New("http://localhost:9374")
	if c.BaseURL != "http://localhost:9374" {
		t.Errorf("BaseURL = %q, want http://localhost:9374", c.BaseURL)
	}
	if c.Agents == nil {
		t.Error("Agents client is nil")
	}
	if c.Channels == nil {
		t.Error("Channels client is nil")
	}
	if c.Events == nil {
		t.Error("Events client is nil")
	}
	if c.Costs == nil {
		t.Error("Costs client is nil")
	}
	if c.Cron == nil {
		t.Error("Cron client is nil")
	}
	if c.MCP == nil {
		t.Error("MCP client is nil")
	}
	if c.Tools == nil {
		t.Error("Tools client is nil")
	}
	if c.Roles == nil {
		t.Error("Roles client is nil")
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient is nil")
	}
}

func TestNew_EmptyAddr(t *testing.T) {
	// With no env var AND no addr file, should use default.
	// Point HOME at an empty tempdir so the test doesn't see the
	// developer's live ~/.mycel/daemon.addr.
	os.Unsetenv("MYCEL_DAEMON_ADDR") //nolint:errcheck
	t.Setenv("HOME", t.TempDir())
	c := New("")
	if c.BaseURL != DefaultHTTPAddr {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, DefaultHTTPAddr)
	}
}

func TestNew_EnvAddr(t *testing.T) {
	t.Setenv("MYCEL_DAEMON_ADDR", "http://custom:1234")
	c := New("")
	if c.BaseURL != "http://custom:1234" {
		t.Errorf("BaseURL = %q, want http://custom:1234", c.BaseURL)
	}
}

// TestNew_DaemonAddrFile exercises the file-over-default path that `bc up`
// writes. Pins the round-trip: a file containing "http://127.0.0.1:8080\n"
// must resolve to exactly that URL (trailing newline trimmed).
func TestNew_DaemonAddrFile(t *testing.T) {
	os.Unsetenv("MYCEL_DAEMON_ADDR") //nolint:errcheck
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	bcDir := filepath.Join(tmp, ".mycel")
	if err := os.MkdirAll(bcDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := "http://127.0.0.1:8080"
	if err := os.WriteFile(filepath.Join(bcDir, "daemon.addr"), []byte(want+"\n"), 0o600); err != nil {
		t.Fatalf("write addr: %v", err)
	}
	c := New("")
	if c.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q (file should override default)", c.BaseURL, want)
	}
}

// TestNew_EnvWinsOverFile pins the precedence: MYCEL_DAEMON_ADDR must beat
// the addr file so a developer with a stale ~/.mycel/daemon.addr can still
// override via env without deleting the file.
func TestNew_EnvWinsOverFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	bcDir := filepath.Join(tmp, ".mycel")
	if err := os.MkdirAll(bcDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bcDir, "daemon.addr"), []byte("http://stale:1\n"), 0o600); err != nil {
		t.Fatalf("write addr: %v", err)
	}
	t.Setenv("MYCEL_DAEMON_ADDR", "http://env-wins:2")
	c := New("")
	if c.BaseURL != "http://env-wins:2" {
		t.Errorf("BaseURL = %q, want http://env-wins:2 (env must beat file)", c.BaseURL)
	}
}

func TestDefaultSocketPath(t *testing.T) {
	p := DefaultSocketPath()
	if p == "" {
		t.Error("DefaultSocketPath() returned empty string")
	}
	if !strings.Contains(p, "bcd.sock") {
		t.Errorf("DefaultSocketPath() = %q, expected to contain bcd.sock", p)
	}
}

func TestPing_Success(t *testing.T) {
	ts := mockServer(t, jsonHandler(200, map[string]string{"status": "ok"}))
	c := New(ts.URL)

	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v, want nil", err)
	}
}

func TestPing_Unhealthy(t *testing.T) {
	ts := mockServer(t, jsonHandler(503, map[string]string{"status": "error"}))
	c := New(ts.URL)

	err := c.Ping(context.Background())
	if err == nil {
		t.Error("Ping() expected error for 503, got nil")
	}
	if !strings.Contains(err.Error(), "unhealthy") {
		t.Errorf("Ping() error = %q, want to contain 'unhealthy'", err.Error())
	}
}

func TestPing_ConnectionRefused(t *testing.T) {
	c := New("http://127.0.0.1:1") // port 1 — connection refused

	err := c.Ping(context.Background())
	if err == nil {
		t.Error("Ping() expected error for connection refused, got nil")
	}
	if !strings.Contains(err.Error(), "daemon not running") {
		t.Errorf("Ping() error = %q, want to contain 'daemon not running'", err.Error())
	}
}

func TestIsDaemonNotRunning(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "daemon not running", err: newErr("daemon not running: connect failed"), want: true},
		{name: "connection refused", err: newErr("connection refused"), want: true},
		{name: "no such file", err: newErr("no such file"), want: true},
		{name: "other error", err: newErr("timeout"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDaemonNotRunning(tt.err); got != tt.want {
				t.Errorf("IsDaemonNotRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGet_InvalidJSON(t *testing.T) {
	ts := mockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte("not json")) //nolint:errcheck
	})
	c := New(ts.URL)

	var result map[string]any
	err := c.get(context.Background(), "/api/test", &result)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestGet_ServerError(t *testing.T) {
	ts := mockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error")) //nolint:errcheck
	})
	c := New(ts.URL)

	var result map[string]any
	err := c.get(context.Background(), "/api/test", &result)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err.Error())
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Errorf("error = %q, want to contain 'internal error'", err.Error())
	}
}

func TestDo_NoContent(t *testing.T) {
	ts := mockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := New(ts.URL)

	err := c.do(context.Background(), http.MethodPost, "/api/test", nil, nil)
	if err != nil {
		t.Errorf("do() error = %v, want nil for 204", err)
	}
}

func TestDo_ErrorReadBody(t *testing.T) {
	ts := mockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
	})
	c := New(ts.URL)

	err := c.do(context.Background(), http.MethodPost, "/api/test", nil, nil)
	if err == nil {
		t.Error("expected error for 400, got nil")
	}
}

func TestDo_NilResult(t *testing.T) {
	ts := mockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})
	c := New(ts.URL)

	// result=nil means we don't decode
	err := c.do(context.Background(), http.MethodPost, "/api/test", map[string]string{"k": "v"}, nil)
	if err != nil {
		t.Errorf("do() error = %v, want nil", err)
	}
}

func TestPut(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPut, 200, map[string]string{"ok": "true"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	var result map[string]string
	err := c.put(context.Background(), "/api/test", map[string]string{"k": "v"}, &result)
	if err != nil {
		t.Fatalf("put() error = %v", err)
	}
	if cap.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", cap.Method)
	}
}

// --- Agents tests ---

func TestAgents_List(t *testing.T) {
	agents := []AgentInfo{
		{Name: "alice", Role: "engineer", State: "idle"},
		{Name: "bob", Role: "engineer", State: "working"},
	}
	ts := mockServer(t, jsonHandler(200, agents))
	c := New(ts.URL)

	result, err := c.Agents.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d agents, want 2", len(result))
	}
	if result[0].Name != "alice" {
		t.Errorf("agents[0].Name = %q, want alice", result[0].Name)
	}
}

func TestAgents_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "boom"}))
	c := New(ts.URL)

	_, err := c.Agents.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestAgents_ListByRole(t *testing.T) {
	agents := []AgentInfo{
		{Name: "alice", Role: "engineer", State: "idle"},
		{Name: "bob", Role: "manager", State: "working"},
		{Name: "charlie", Role: "engineer", State: "idle"},
	}
	ts := mockServer(t, jsonHandler(200, agents))
	c := New(ts.URL)

	result, err := c.Agents.ListByRole(context.Background(), "engineer")
	if err != nil {
		t.Fatalf("ListByRole() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d agents, want 2", len(result))
	}
}

func TestAgents_ListByRole_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "boom"}))
	c := New(ts.URL)

	_, err := c.Agents.ListByRole(context.Background(), "engineer")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestAgents_Get(t *testing.T) {
	agent := AgentInfo{Name: "alice", Role: "engineer", State: "idle"}
	handler, _ := capturingHandler(t, http.MethodGet, 200, agent)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Agents.Get(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "alice" {
		t.Errorf("Name = %q, want alice", result.Name)
	}
}

func TestAgents_Get_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(404, map[string]string{"error": "not found"}))
	c := New(ts.URL)

	_, err := c.Agents.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestAgents_Create(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, AgentInfo{Name: "alice", Role: "engineer"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Agents.Create(context.Background(), CreateAgentReq{Name: "alice", Role: "engineer"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Name != "alice" {
		t.Errorf("Name = %q, want alice", result.Name)
	}
	if cap.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", cap.Method)
	}
}

func TestAgents_Create_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(400, map[string]string{"error": "invalid"}))
	c := New(ts.URL)

	_, err := c.Agents.Create(context.Background(), CreateAgentReq{Name: ""})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestAgents_Start(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, AgentInfo{Name: "alice", State: "running"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Agents.Start(context.Background(), "alice", "tmux", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.State != "running" {
		t.Errorf("State = %q, want running", result.State)
	}
	if !strings.Contains(cap.Path, "/api/agents/alice/start") {
		t.Errorf("path = %q, want to contain /api/agents/alice/start", cap.Path)
	}
}

func TestAgents_Stop(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Agents.Stop(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/agents/alice/stop") {
		t.Errorf("path = %q, want to contain /api/agents/alice/stop", cap.Path)
	}
}

func TestAgents_Delete(t *testing.T) {
	tests := []struct {
		name      string
		wantQuery string
		force     bool
	}{
		{name: "without force", force: false, wantQuery: "/api/agents/alice"},
		{name: "with force", force: true, wantQuery: "/api/agents/alice?force=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
			ts := mockServer(t, handler)
			c := New(ts.URL)

			err := c.Agents.Delete(context.Background(), "alice", tt.force)
			if err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if cap.Path != tt.wantQuery {
				t.Errorf("path = %q, want %q", cap.Path, tt.wantQuery)
			}
		})
	}
}

func TestAgents_Send(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Agents.Send(context.Background(), "alice", "hello")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if cap.Body["message"] != "hello" {
		t.Errorf("body message = %v, want hello", cap.Body["message"])
	}
}

func TestAgents_Rename(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Agents.Rename(context.Background(), "alice", "alice2")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if cap.Body["new_name"] != "alice2" {
		t.Errorf("body new_name = %v, want alice2", cap.Body["new_name"])
	}
}

func TestAgents_Peek(t *testing.T) {
	ts := mockServer(t, jsonHandler(200, map[string]string{"output": "hello world"}))
	c := New(ts.URL)

	output, err := c.Agents.Peek(context.Background(), "alice", 10)
	if err != nil {
		t.Fatalf("Peek() error = %v", err)
	}
	if output != "hello world" {
		t.Errorf("output = %q, want 'hello world'", output)
	}
}

func TestAgents_Sessions(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "sess1", Current: true},
		{ID: "sess2", Current: false},
	}
	ts := mockServer(t, jsonHandler(200, sessions))
	c := New(ts.URL)

	result, err := c.Agents.Sessions(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Sessions() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d sessions, want 2", len(result))
	}
	if !result[0].Current {
		t.Error("sessions[0].Current = false, want true")
	}
}

func TestAgents_Broadcast(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, map[string]int{"sent": 3})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	sent, err := c.Agents.Broadcast(context.Background(), "hello all")
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3", sent)
	}
	if cap.Body["message"] != "hello all" {
		t.Errorf("body message = %v, want 'hello all'", cap.Body["message"])
	}
}

func TestAgents_SendToRole(t *testing.T) {
	resp := SendResultInfo{Matched: []string{"alice", "bob"}, Sent: 2}
	handler, cap := capturingHandler(t, http.MethodPost, 200, resp)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Agents.SendToRole(context.Background(), "engineer", "do work")
	if err != nil {
		t.Fatalf("SendToRole() error = %v", err)
	}
	if result.Sent != 2 {
		t.Errorf("Sent = %d, want 2", result.Sent)
	}
	if cap.Body["role"] != "engineer" {
		t.Errorf("body role = %v, want engineer", cap.Body["role"])
	}
}

func TestAgents_SendToPattern(t *testing.T) {
	resp := SendResultInfo{Matched: []string{"alice"}, Sent: 1}
	handler, cap := capturingHandler(t, http.MethodPost, 200, resp)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Agents.SendToPattern(context.Background(), "ali*", "hello")
	if err != nil {
		t.Fatalf("SendToPattern() error = %v", err)
	}
	if result.Sent != 1 {
		t.Errorf("Sent = %d, want 1", result.Sent)
	}
	if cap.Body["pattern"] != "ali*" {
		t.Errorf("body pattern = %v, want ali*", cap.Body["pattern"])
	}
}

func TestAgents_GenerateName(t *testing.T) {
	ts := mockServer(t, jsonHandler(200, map[string]string{"name": "fuzzy-panda"}))
	c := New(ts.URL)

	name, err := c.Agents.GenerateName(context.Background())
	if err != nil {
		t.Fatalf("GenerateName() error = %v", err)
	}
	if name != "fuzzy-panda" {
		t.Errorf("name = %q, want fuzzy-panda", name)
	}
}

func TestAgents_Stats(t *testing.T) {
	records := []*AgentStatsRecord{
		{AgentName: "alice", CPUPct: 25.5, MemUsedMB: 128},
	}
	ts := mockServer(t, jsonHandler(200, records))
	c := New(ts.URL)

	result, err := c.Agents.Stats(context.Background(), "alice", 5)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d records, want 1", len(result))
	}
	if result[0].CPUPct != 25.5 {
		t.Errorf("CPUPct = %v, want 25.5", result[0].CPUPct)
	}
}

func TestAgents_Cost(t *testing.T) {
	summary := AgentCostSummary{AgentID: "alice", TotalCostUSD: 1.23, RequestCount: 10}
	ts := mockServer(t, jsonHandler(200, summary))
	c := New(ts.URL)

	result, err := c.Agents.Cost(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Cost() error = %v", err)
	}
	if result.TotalCostUSD != 1.23 {
		t.Errorf("TotalCostUSD = %v, want 1.23", result.TotalCostUSD)
	}
}

func TestAgents_Report(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Agents.Report(context.Background(), "alice", "working", "doing stuff")
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if cap.Body["state"] != "working" {
		t.Errorf("body state = %v, want working", cap.Body["state"])
	}
}

func TestAgents_Health(t *testing.T) {
	health := []AgentHealthInfo{
		{Name: "alice", Status: "healthy", TmuxAlive: true, StateFresh: true},
	}
	ts := mockServer(t, jsonHandler(200, health))
	c := New(ts.URL)

	result, err := c.Agents.Health(context.Background(), "30s", "")
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0].Status != "healthy" {
		t.Errorf("Status = %q, want healthy", result[0].Status)
	}
}

func TestAgents_Health_WithAgent(t *testing.T) {
	var gotPath string
	ts := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]AgentHealthInfo{}) //nolint:errcheck
	})
	c := New(ts.URL)

	_, err := c.Agents.Health(context.Background(), "30s", "alice")
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !strings.Contains(gotPath, "agent=alice") {
		t.Errorf("path = %q, want to contain agent=alice", gotPath)
	}
}

func TestAgents_StopAll(t *testing.T) {
	handler, _ := capturingHandler(t, http.MethodPost, 200, map[string]int{"stopped": 5})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	stopped, err := c.Agents.StopAll(context.Background())
	if err != nil {
		t.Fatalf("StopAll() error = %v", err)
	}
	if stopped != 5 {
		t.Errorf("stopped = %d, want 5", stopped)
	}
}

// --- Channels tests ---

func TestChannels_List(t *testing.T) {
	channels := []ChannelInfo{
		{Name: "general", MemberCount: 3},
		{Name: "eng", MemberCount: 2},
	}
	ts := mockServer(t, jsonHandler(200, channels))
	c := New(ts.URL)

	result, err := c.Channels.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d channels, want 2", len(result))
	}
}

func TestChannels_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "db error"}))
	c := New(ts.URL)

	_, err := c.Channels.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestChannels_Get(t *testing.T) {
	ch := ChannelInfo{Name: "general", MemberCount: 5}
	ts := mockServer(t, jsonHandler(200, ch))
	c := New(ts.URL)

	result, err := c.Channels.Get(context.Background(), "general")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "general" {
		t.Errorf("Name = %q, want general", result.Name)
	}
}

func TestChannels_Get_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(404, map[string]string{"error": "not found"}))
	c := New(ts.URL)

	_, err := c.Channels.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestChannels_Create(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, ChannelInfo{Name: "new-chan"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Channels.Create(context.Background(), "new-chan", "a channel")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Name != "new-chan" {
		t.Errorf("Name = %q, want new-chan", result.Name)
	}
	if cap.Body["name"] != "new-chan" {
		t.Errorf("body name = %v, want new-chan", cap.Body["name"])
	}
}

func TestChannels_Update(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPut, 200, ChannelInfo{Name: "general", Description: "updated"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Channels.Update(context.Background(), "general", "updated")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Description != "updated" {
		t.Errorf("Description = %q, want updated", result.Description)
	}
	if cap.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", cap.Method)
	}
}

func TestChannels_Delete(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Channels.Delete(context.Background(), "old-chan")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/channels/old-chan") {
		t.Errorf("path = %q, want to contain /api/channels/old-chan", cap.Path)
	}
}

func TestChannels_AddMember(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Channels.AddMember(context.Background(), "general", "alice")
	if err != nil {
		t.Fatalf("AddMember() error = %v", err)
	}
	if cap.Body["agent_id"] != "alice" {
		t.Errorf("body agent_id = %v, want alice", cap.Body["agent_id"])
	}
}

func TestChannels_RemoveMember(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Channels.RemoveMember(context.Background(), "general", "alice")
	if err != nil {
		t.Fatalf("RemoveMember() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/channels/general/members/alice") {
		t.Errorf("path = %q, want to contain /api/channels/general/members/alice", cap.Path)
	}
}

func TestChannels_Send(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, MessageInfo{Channel: "general", Sender: "alice", Content: "hi"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Channels.Send(context.Background(), "general", "alice", "hi")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Content != "hi" {
		t.Errorf("Content = %q, want hi", result.Content)
	}
	if cap.Body["sender"] != "alice" {
		t.Errorf("body sender = %v, want alice", cap.Body["sender"])
	}
}

func TestChannels_History(t *testing.T) {
	msgs := []MessageInfo{
		{Channel: "general", Sender: "alice", Content: "msg1"},
		{Channel: "general", Sender: "bob", Content: "msg2"},
	}
	ts := mockServer(t, jsonHandler(200, msgs))
	c := New(ts.URL)

	result, err := c.Channels.History(context.Background(), "general", 10, 0, "")
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d messages, want 2", len(result))
	}
}

func TestChannels_History_WithFilter(t *testing.T) {
	var gotPath string
	ts := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]MessageInfo{}) //nolint:errcheck
	})
	c := New(ts.URL)

	_, err := c.Channels.History(context.Background(), "general", 10, 0, "alice")
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if !strings.Contains(gotPath, "agent=alice") {
		t.Errorf("path = %q, want to contain agent=alice", gotPath)
	}
}

func TestChannels_React(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, map[string]bool{"added": true})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	added, err := c.Channels.React(context.Background(), "general", 42, "+1", "alice")
	if err != nil {
		t.Fatalf("React() error = %v", err)
	}
	if !added {
		t.Error("added = false, want true")
	}
	if cap.Body["emoji"] != "+1" {
		t.Errorf("body emoji = %v, want +1", cap.Body["emoji"])
	}
}

func TestChannels_Status(t *testing.T) {
	status := ChannelStatusInfo{ChannelCount: 3, TotalMembers: 10}
	ts := mockServer(t, jsonHandler(200, status))
	c := New(ts.URL)

	result, err := c.Channels.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.ChannelCount != 3 {
		t.Errorf("ChannelCount = %d, want 3", result.ChannelCount)
	}
}

// --- Costs tests ---

func TestCosts_WorkspaceSummary(t *testing.T) {
	summary := CostSummary{TotalCostUSD: 42.5, TotalTokens: 1000}
	ts := mockServer(t, jsonHandler(200, summary))
	c := New(ts.URL)

	result, err := c.Costs.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceSummary() error = %v", err)
	}
	if result.TotalCostUSD != 42.5 {
		t.Errorf("TotalCostUSD = %v, want 42.5", result.TotalCostUSD)
	}
}

func TestCosts_SummaryByAgent(t *testing.T) {
	summaries := []*CostSummary{
		{AgentID: "alice", TotalCostUSD: 10},
		{AgentID: "bob", TotalCostUSD: 20},
	}
	ts := mockServer(t, jsonHandler(200, summaries))
	c := New(ts.URL)

	result, err := c.Costs.SummaryByAgent(context.Background())
	if err != nil {
		t.Fatalf("SummaryByAgent() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d summaries, want 2", len(result))
	}
}

func TestCosts_SummaryByTeam(t *testing.T) {
	summaries := []*CostSummary{{TeamID: "alpha", TotalCostUSD: 15}}
	ts := mockServer(t, jsonHandler(200, summaries))
	c := New(ts.URL)

	result, err := c.Costs.SummaryByTeam(context.Background())
	if err != nil {
		t.Fatalf("SummaryByTeam() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d summaries, want 1", len(result))
	}
}

func TestCosts_SummaryByModel(t *testing.T) {
	summaries := []*CostSummary{{Model: "gpt-4", TotalCostUSD: 30}}
	ts := mockServer(t, jsonHandler(200, summaries))
	c := New(ts.URL)

	result, err := c.Costs.SummaryByModel(context.Background())
	if err != nil {
		t.Fatalf("SummaryByModel() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d summaries, want 1", len(result))
	}
}

func TestCosts_Daily(t *testing.T) {
	costs := []*DailyCost{
		{Date: "2026-03-22", CostUSD: 5.5},
		{Date: "2026-03-23", CostUSD: 3.2},
	}
	ts := mockServer(t, jsonHandler(200, costs))
	c := New(ts.URL)

	result, err := c.Costs.Daily(context.Background(), 7)
	if err != nil {
		t.Fatalf("Daily() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d costs, want 2", len(result))
	}
}

func TestCosts_ListBudgets(t *testing.T) {
	budgets := []*CostBudget{{Scope: "workspace", LimitUSD: 100}}
	ts := mockServer(t, jsonHandler(200, budgets))
	c := New(ts.URL)

	result, err := c.Costs.ListBudgets(context.Background())
	if err != nil {
		t.Fatalf("ListBudgets() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d budgets, want 1", len(result))
	}
}

func TestCosts_CheckBudget(t *testing.T) {
	status := CostBudgetStatus{
		Budget:       &CostBudget{Scope: "workspace", LimitUSD: 100},
		CurrentSpend: 42.5,
		PercentUsed:  42.5,
	}
	ts := mockServer(t, jsonHandler(200, status))
	c := New(ts.URL)

	result, err := c.Costs.CheckBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CheckBudget() error = %v", err)
	}
	if result.CurrentSpend != 42.5 {
		t.Errorf("CurrentSpend = %v, want 42.5", result.CurrentSpend)
	}
}

func TestCosts_SetBudget(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, CostBudget{Scope: "workspace", LimitUSD: 100})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Costs.SetBudget(context.Background(), &SetBudgetReq{
		Scope:    "workspace",
		LimitUSD: 100,
		AlertAt:  80,
	})
	if err != nil {
		t.Fatalf("SetBudget() error = %v", err)
	}
	if result.LimitUSD != 100 {
		t.Errorf("LimitUSD = %v, want 100", result.LimitUSD)
	}
	if cap.Body["scope"] != "workspace" {
		t.Errorf("body scope = %v, want workspace", cap.Body["scope"])
	}
}

func TestCosts_DeleteBudget(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Costs.DeleteBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("DeleteBudget() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/costs/budgets/workspace") {
		t.Errorf("path = %q, want to contain /api/costs/budgets/workspace", cap.Path)
	}
}

func TestCosts_ProjectCost(t *testing.T) {
	proj := CostProjection{ProjectedCost: 150, DaysAnalyzed: 7, DailyAvgCost: 5}
	ts := mockServer(t, jsonHandler(200, proj))
	c := New(ts.URL)

	result, err := c.Costs.ProjectCost(context.Background(), 7, 30)
	if err != nil {
		t.Fatalf("ProjectCost() error = %v", err)
	}
	if result.ProjectedCost != 150 {
		t.Errorf("ProjectedCost = %v, want 150", result.ProjectedCost)
	}
}

func TestCosts_AgentSummary(t *testing.T) {
	detail := AgentCostDetail{
		Summary: &CostSummary{AgentID: "alice", TotalCostUSD: 5},
		Daily:   []*AgentDailyCost{{AgentID: "alice", Date: "2026-03-23", CostUSD: 2.5}},
	}
	ts := mockServer(t, jsonHandler(200, detail))
	c := New(ts.URL)

	result, err := c.Costs.AgentSummary(context.Background(), "alice")
	if err != nil {
		t.Fatalf("AgentSummary() error = %v", err)
	}
	if result.Summary.TotalCostUSD != 5 {
		t.Errorf("TotalCostUSD = %v, want 5", result.Summary.TotalCostUSD)
	}
	if len(result.Daily) != 1 {
		t.Errorf("got %d daily, want 1", len(result.Daily))
	}
}

func TestCosts_Sync(t *testing.T) {
	ts := mockServer(t, jsonHandler(200, map[string]int{"imported": 42}))
	c := New(ts.URL)

	imported, err := c.Costs.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if imported != 42 {
		t.Errorf("imported = %d, want 42", imported)
	}
}

// --- Cron tests ---

func TestCron_List(t *testing.T) {
	jobs := []CronJob{
		{Name: "backup", Schedule: "0 0 * * *", Enabled: true},
		{Name: "cleanup", Schedule: "0 */6 * * *", Enabled: false},
	}
	ts := mockServer(t, jsonHandler(200, jobs))
	c := New(ts.URL)

	result, err := c.Cron.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d jobs, want 2", len(result))
	}
}

func TestCron_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "db error"}))
	c := New(ts.URL)

	_, err := c.Cron.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCron_Get(t *testing.T) {
	job := CronJob{Name: "backup", Schedule: "0 0 * * *", Enabled: true}
	ts := mockServer(t, jsonHandler(200, job))
	c := New(ts.URL)

	result, err := c.Cron.Get(context.Background(), "backup")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "backup" {
		t.Errorf("Name = %q, want backup", result.Name)
	}
}

func TestCron_Add(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, CronJob{Name: "backup", Schedule: "0 0 * * *"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.Cron.Add(context.Background(), &CronJob{Name: "backup", Schedule: "0 0 * * *"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Name != "backup" {
		t.Errorf("Name = %q, want backup", result.Name)
	}
	if cap.Body["name"] != "backup" {
		t.Errorf("body name = %v, want backup", cap.Body["name"])
	}
}

func TestCron_Delete(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Cron.Delete(context.Background(), "backup")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/cron/backup") {
		t.Errorf("path = %q, want to contain /api/cron/backup", cap.Path)
	}
}

func TestCron_Enable(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Cron.Enable(context.Background(), "backup")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/cron/backup/enable") {
		t.Errorf("path = %q, want to contain /api/cron/backup/enable", cap.Path)
	}
}

func TestCron_Disable(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Cron.Disable(context.Background(), "backup")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/cron/backup/disable") {
		t.Errorf("path = %q, want to contain /api/cron/backup/disable", cap.Path)
	}
}

func TestCron_Run(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Cron.Run(context.Background(), "backup")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/cron/backup/run") {
		t.Errorf("path = %q, want to contain /api/cron/backup/run", cap.Path)
	}
}

func TestCron_Logs(t *testing.T) {
	entries := []CronLogEntry{
		{Status: "success", DurationMS: 1200},
		{Status: "failed", DurationMS: 500},
	}
	ts := mockServer(t, jsonHandler(200, entries))
	c := New(ts.URL)

	result, err := c.Cron.Logs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d entries, want 2", len(result))
	}
}

// --- Events tests ---

func TestEvents_List(t *testing.T) {
	events := []EventInfo{
		{Type: "agent.started", Agent: "alice"},
		{Type: "agent.stopped", Agent: "bob"},
	}
	ts := mockServer(t, jsonHandler(200, events))
	c := New(ts.URL)

	result, err := c.Events.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d events, want 2", len(result))
	}
}

func TestEvents_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "db error"}))
	c := New(ts.URL)

	_, err := c.Events.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestEvents_ListByAgent(t *testing.T) {
	events := []EventInfo{
		{Type: "agent.started", Agent: "alice"},
	}
	ts := mockServer(t, jsonHandler(200, events))
	c := New(ts.URL)

	result, err := c.Events.ListByAgent(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ListByAgent() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d events, want 1", len(result))
	}
}

func TestEvents_Append(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.Events.Append(context.Background(), EventInfo{Type: "test.event", Agent: "alice"})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if cap.Body["type"] != "test.event" {
		t.Errorf("body type = %v, want test.event", cap.Body["type"])
	}
}

func TestEvents_Tail(t *testing.T) {
	events := []EventInfo{
		{Type: "agent.started", Agent: "alice"},
	}
	var gotPath string
	ts := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(events) //nolint:errcheck
	})
	c := New(ts.URL)

	result, err := c.Events.Tail(context.Background(), 5)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d events, want 1", len(result))
	}
	if !strings.Contains(gotPath, "tail=5") {
		t.Errorf("path = %q, want to contain tail=5", gotPath)
	}
}

// --- MCP tests ---

func TestMCP_List(t *testing.T) {
	configs := []*MCPServerConfig{
		{Name: "github", Transport: "stdio", Enabled: true},
	}
	ts := mockServer(t, jsonHandler(200, configs))
	c := New(ts.URL)

	result, err := c.MCP.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d configs, want 1", len(result))
	}
	if result[0].Name != "github" {
		t.Errorf("Name = %q, want github", result[0].Name)
	}
}

func TestMCP_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "boom"}))
	c := New(ts.URL)

	_, err := c.MCP.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestMCP_Get(t *testing.T) {
	cfg := MCPServerConfig{Name: "github", Transport: "stdio", Enabled: true}
	ts := mockServer(t, jsonHandler(200, cfg))
	c := New(ts.URL)

	result, err := c.MCP.Get(context.Background(), "github")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "github" {
		t.Errorf("Name = %q, want github", result.Name)
	}
}

func TestMCP_Add(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 200, MCPServerConfig{Name: "slack", Transport: "sse"})
	ts := mockServer(t, handler)
	c := New(ts.URL)

	result, err := c.MCP.Add(context.Background(), &MCPServerConfig{Name: "slack", Transport: "sse"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Name != "slack" {
		t.Errorf("Name = %q, want slack", result.Name)
	}
	if cap.Body["name"] != "slack" {
		t.Errorf("body name = %v, want slack", cap.Body["name"])
	}
}

func TestMCP_Remove(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodDelete, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.MCP.Remove(context.Background(), "slack")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/mcp/slack") {
		t.Errorf("path = %q, want to contain /api/mcp/slack", cap.Path)
	}
}

func TestMCP_Enable(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.MCP.Enable(context.Background(), "github")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/mcp/github/enable") {
		t.Errorf("path = %q, want to contain /api/mcp/github/enable", cap.Path)
	}
}

func TestMCP_Disable(t *testing.T) {
	handler, cap := capturingHandler(t, http.MethodPost, 204, nil)
	ts := mockServer(t, handler)
	c := New(ts.URL)

	err := c.MCP.Disable(context.Background(), "github")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !strings.Contains(cap.Path, "/api/mcp/github/disable") {
		t.Errorf("path = %q, want to contain /api/mcp/github/disable", cap.Path)
	}
}

// --- Roles tests ---

func TestRoles_List(t *testing.T) {
	roles := map[string]*RoleInfo{
		"engineer": {Name: "engineer", Prompt: "you are an engineer"},
		"manager":  {Name: "manager", Prompt: "you are a manager"},
	}
	ts := mockServer(t, jsonHandler(200, roles))
	c := New(ts.URL)

	result, err := c.Roles.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d roles, want 2", len(result))
	}
	if result["engineer"].Prompt != "you are an engineer" {
		t.Errorf("engineer prompt = %q, want 'you are an engineer'", result["engineer"].Prompt)
	}
}

func TestRoles_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "boom"}))
	c := New(ts.URL)

	_, err := c.Roles.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRoles_Get(t *testing.T) {
	roles := map[string]*RoleInfo{
		"engineer": {Name: "engineer", Prompt: "you are an engineer"},
		"manager":  {Name: "manager", Prompt: "you are a manager"},
	}
	ts := mockServer(t, jsonHandler(200, roles))
	c := New(ts.URL)

	result, err := c.Roles.Get(context.Background(), "engineer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "engineer" {
		t.Errorf("Name = %q, want engineer", result.Name)
	}
}

func TestRoles_Get_NotFound(t *testing.T) {
	roles := map[string]*RoleInfo{
		"engineer": {Name: "engineer"},
	}
	ts := mockServer(t, jsonHandler(200, roles))
	c := New(ts.URL)

	_, err := c.Roles.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected error for missing role, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// --- Tools tests ---

func TestTools_List(t *testing.T) {
	tools := []*ToolInfo{
		{Name: "claude", Enabled: true, Builtin: true},
		{Name: "gemini", Enabled: false},
	}
	ts := mockServer(t, jsonHandler(200, tools))
	c := New(ts.URL)

	result, err := c.Tools.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d tools, want 2", len(result))
	}
	if !result[0].Builtin {
		t.Error("tools[0].Builtin = false, want true")
	}
}

func TestTools_List_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(500, map[string]string{"error": "boom"}))
	c := New(ts.URL)

	_, err := c.Tools.List(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTools_Get(t *testing.T) {
	tool := ToolInfo{Name: "claude", Command: "claude", Enabled: true}
	ts := mockServer(t, jsonHandler(200, tool))
	c := New(ts.URL)

	result, err := c.Tools.Get(context.Background(), "claude")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Name != "claude" {
		t.Errorf("Name = %q, want claude", result.Name)
	}
}

func TestTools_Get_Error(t *testing.T) {
	ts := mockServer(t, jsonHandler(404, map[string]string{"error": "not found"}))
	c := New(ts.URL)

	_, err := c.Tools.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// newErr creates a simple error for testing.
func newErr(msg string) error {
	return &simpleError{msg: msg}
}

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
}
