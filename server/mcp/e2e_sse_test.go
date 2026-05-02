package mcp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/server/mcp"
)

// sseEvent represents a parsed SSE event from the stream.
type sseEvent struct {
	Event string // "endpoint", "message", or "" (default)
	Data  string
}

// sseClient manages a long-lived SSE connection and parses events.
type sseClient struct {
	events chan sseEvent
	resp   *http.Response
	cancel context.CancelFunc
}

// newSSEClient connects to the SSE endpoint and starts reading events in a goroutine.
func newSSEClient(t *testing.T, url string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("new SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // body kept open for SSE streaming; closed in sseClient.close()
	if err != nil {
		cancel()
		t.Fatalf("GET /sse: %v", err)
	}

	c := &sseClient{
		events: make(chan sseEvent, 16),
		resp:   resp,
		cancel: cancel,
	}

	go c.readLoop()
	return c
}

// readLoop reads SSE events from the response body and sends them on the events channel.
func (c *sseClient) readLoop() {
	defer close(c.events)
	scanner := bufio.NewScanner(c.resp.Body)
	var event, data string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if data != "" || event != "" {
				c.events <- sseEvent{Event: event, Data: data}
				event = ""
				data = ""
			}
		}
	}
}

// close shuts down the SSE connection.
func (c *sseClient) close() {
	c.cancel()
	c.resp.Body.Close() //nolint:errcheck
}

// readEvent reads the next SSE event with a timeout.
func (c *sseClient) readEvent(t *testing.T) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-c.events:
		if !ok {
			t.Fatal("SSE stream closed unexpectedly")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
		return sseEvent{}
	}
}

// postRPC sends a JSON-RPC request to the message endpoint and asserts 202 Accepted.
func postRPC(t *testing.T, url string, id int, method string, params any) {
	t.Helper()

	rawID := json.RawMessage(jsonNum(id))
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		rawParams = b
	}

	req := mcp.Request{JSONRPC: "2.0", ID: &rawID, Method: method, Params: rawParams}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, url,
		strings.NewReader(string(body)),
	)
	if err != nil {
		t.Fatalf("new POST request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("POST %s: %v", method, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s: status = %d, want 202", method, resp.StatusCode)
	}
}

// jsonNum returns a JSON number string for an int.
func jsonNum(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// decodeSSEResponse unmarshals the SSE event data as a JSON-RPC response.
func decodeSSEResponse(t *testing.T, ev sseEvent) mcp.Response {
	t.Helper()
	var resp mcp.Response
	if err := json.Unmarshal([]byte(ev.Data), &resp); err != nil {
		t.Fatalf("decode SSE response: %v\nraw: %s", err, ev.Data)
	}
	return resp
}

// decodeResult re-marshals the response result and unmarshals into dst.
func decodeResult(t *testing.T, resp mcp.Response, dst any) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %s (code %d)", resp.Error.Message, resp.Error.Code)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-marshal result: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("decode result into %T: %v", dst, err)
	}
}

// ─── SSE keepalive test ──────────────────────────────────────────────────────

// TestSSE_Keepalive verifies that the SSE endpoint sends periodic keepalive
// comments to prevent idle connection timeouts. Uses a short ticker override
// via a custom broker to avoid waiting 30 seconds in tests.
func TestSSE_Keepalive(t *testing.T) {
	broker := mcp.NewSSEBroker()
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	scanner := bufio.NewScanner(resp.Body)

	// Read the endpoint event first
	gotEndpoint := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: endpoint") {
			gotEndpoint = true
		}
		if gotEndpoint && line == "" {
			break // finished reading endpoint event
		}
	}
	if !gotEndpoint {
		t.Fatal("did not receive endpoint event")
	}

	// Now wait for a keepalive comment (": keepalive")
	// The ticker fires every 30s, so allow up to 35s.
	deadline := time.After(35 * time.Second)
	keepaliveCh := make(chan bool, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if line == ": keepalive" {
				keepaliveCh <- true
				return
			}
		}
		keepaliveCh <- false
	}()

	select {
	case got := <-keepaliveCh:
		if !got {
			t.Fatal("SSE stream closed without sending keepalive")
		}
	case <-deadline:
		t.Fatal("timed out waiting for SSE keepalive (expected within 35s)")
	}

	cancel()
}

// ─── E2E SSE tests ───────────────────────────────────────────────────────────

func TestSSE_E2E_InitializeRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	// First event must be the endpoint event.
	ev := client.readEvent(t)
	if ev.Event != "endpoint" {
		t.Fatalf("first event type = %q, want endpoint", ev.Event)
	}

	// The endpoint data tells us where to POST.
	messageURL := ts.URL + ev.Data

	// POST initialize request.
	postRPC(t, messageURL, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "e2e-test"},
	})

	// Read initialize response from SSE stream.
	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

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
	decodeResult(t, resp, &result)

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

func TestSSE_E2E_ResourcesList(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	postRPC(t, messageURL, 1, "resources/list", nil)

	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

	var result struct {
		Resources []mcp.Resource `json:"resources"`
	}
	decodeResult(t, resp, &result)

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
		t.Errorf("got %d resources, want %d", len(result.Resources), len(wantURIs))
	}
}

func TestSSE_E2E_ResourcesRead(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	postRPC(t, messageURL, 1, "resources/read", map[string]string{"uri": "bc://agents"})

	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	decodeResult(t, resp, &result)

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Contents))
	}
	c := result.Contents[0]
	if c.URI != "bc://agents" {
		t.Errorf("content URI = %q, want bc://agents", c.URI)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("mimeType = %q, want application/json", c.MIMEType)
	}
	// Verify the text is valid JSON.
	if !json.Valid([]byte(c.Text)) {
		t.Errorf("content text is not valid JSON: %s", c.Text)
	}
}

func TestSSE_E2E_ToolsList(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	postRPC(t, messageURL, 1, "tools/list", nil)

	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

	var result struct {
		Tools []mcp.Tool `json:"tools"`
	}
	decodeResult(t, resp, &result)

	// send_message, list_channels, and read_channel re-added on gateway/notify.
	wantNames := []string{"send_message", "list_channels", "read_channel", "send_file", "whoami", "list_agents"}
	got := make(map[string]bool)
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	for _, name := range wantNames {
		if !got[name] {
			t.Errorf("tools/list missing tool %q", name)
		}
	}
	if len(result.Tools) != len(wantNames) {
		t.Errorf("got %d tools, want %d", len(result.Tools), len(wantNames))
	}
}

// TestSSE_E2E_ToolsCall_Whoami exercises the whoami tool via SSE.
func TestSSE_E2E_ToolsCall_Whoami(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	postRPC(t, messageURL, 1, "tools/call", map[string]any{
		"name":      "whoami",
		"arguments": map[string]any{},
	})

	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	decodeResult(t, resp, &result)

	if result.IsError {
		t.Error("whoami returned isError=true")
	}
	if len(result.Content) == 0 {
		t.Error("whoami returned no content")
	}
}

func TestSSE_E2E_ErrorHandling(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	// Send a request with an unknown method.
	postRPC(t, messageURL, 1, "nonexistent/method", nil)

	respEv := client.readEvent(t)
	resp := decodeSSEResponse(t, respEv)

	if resp.Error == nil {
		t.Fatal("expected error response for unknown method")
	}
	if resp.Error.Code != mcp.ErrMethodNotFound {
		t.Errorf("error code = %d, want %d (ErrMethodNotFound)", resp.Error.Code, mcp.ErrMethodNotFound)
	}
}

func TestSSE_E2E_MultipleRequests(t *testing.T) {
	srv := newTestServer(t)
	broker := mcp.NewSSEBroker()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", broker.SSEHandler())
	mux.HandleFunc("/message", srv.HandleSSEMessage(context.Background(), broker))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newSSEClient(t, ts.URL+"/sse")
	defer client.close()

	ev := client.readEvent(t)
	messageURL := ts.URL + ev.Data

	// Send multiple requests in sequence and verify each response arrives on the SSE stream.
	postRPC(t, messageURL, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
	})
	resp1 := decodeSSEResponse(t, client.readEvent(t))
	if resp1.Error != nil {
		t.Errorf("initialize: unexpected error: %s", resp1.Error.Message)
	}

	postRPC(t, messageURL, 2, "tools/list", nil)
	resp2 := decodeSSEResponse(t, client.readEvent(t))
	if resp2.Error != nil {
		t.Errorf("tools/list: unexpected error: %s", resp2.Error.Message)
	}

	postRPC(t, messageURL, 3, "resources/list", nil)
	resp3 := decodeSSEResponse(t, client.readEvent(t))
	if resp3.Error != nil {
		t.Errorf("resources/list: unexpected error: %s", resp3.Error.Message)
	}
}
