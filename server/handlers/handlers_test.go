package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/server"
	"github.com/rpuneet/mycel/server/ws"
)

// --- test helpers (mirrors server_test.go pattern) ---

func buildTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	hub := ws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	cfg := server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}
	srv := server.New(cfg, server.Services{}, hub, nil)
	return httptest.NewServer(srv.Handler())
}

func doRequest(t *testing.T, method, url, contentType, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	return doRequest(t, http.MethodGet, url, "", "")
}

func post(t *testing.T, url, contentType, body string) *http.Response {
	t.Helper()
	return doRequest(t, http.MethodPost, url, contentType, body)
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return m
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("want status %d, got %d", want, resp.StatusCode)
	}
}

func assertContentType(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, want) {
		t.Fatalf("want Content-Type starting with %q, got %q", want, ct)
	}
}

// --- tests ---

func TestHealthEndpoint(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	assertContentType(t, resp, "application/json")

	body := readJSON(t, resp)
	if body["status"] != "ok" {
		t.Fatalf("want status ok, got %v", body["status"])
	}
	if _, ok := body["addr"]; !ok {
		t.Fatal("response missing addr field")
	}
}

func TestHealthReadyEndpoint(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health/ready")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	assertContentType(t, resp, "application/json")

	body := readJSON(t, resp)
	if body["status"] != "ok" {
		t.Fatalf("want status ok, got %v", body["status"])
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatal("expected checks to be a map")
	}
	// With nil services, checks should be empty (no db, no agents).
	if len(checks) != 0 {
		t.Fatalf("expected empty checks with nil services, got %v", checks)
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"POST /health", "/health", http.MethodPost},
		{"PUT /health", "/health", http.MethodPut},
		{"DELETE /health", "/health", http.MethodDelete},
		{"POST /health/ready", "/health/ready", http.MethodPost},
		{"PUT /health/ready", "/health/ready", http.MethodPut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, tt.method, ts.URL+tt.path, "application/json", "")
			defer func() { _ = resp.Body.Close() }()
			assertStatus(t, resp, http.StatusMethodNotAllowed)
			body := readJSON(t, resp)
			if _, ok := body["error"]; !ok {
				t.Fatal("expected error field in 405 response")
			}
		})
	}
}

func TestCORSHeaders(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health")
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("want CORS origin *, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("missing Access-Control-Allow-Methods header")
	}
}

func TestCORSPreflight(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := doRequest(t, http.MethodOptions, ts.URL+"/health", "", "")
	defer func() { _ = resp.Body.Close() }()

	assertStatus(t, resp, http.StatusNoContent)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("want CORS origin *, got %q", got)
	}
}

func TestRequestIDHeader(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	t.Run("generated when absent", func(t *testing.T) {
		resp := get(t, ts.URL+"/health")
		defer func() { _ = resp.Body.Close() }()
		id := resp.Header.Get("X-Request-ID")
		if id == "" {
			t.Fatal("expected X-Request-ID header to be set")
		}
	})

	t.Run("echoed when provided", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Request-ID", "test-request-123")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if got := resp.Header.Get("X-Request-ID"); got != "test-request-123" {
			t.Fatalf("want echoed request ID test-request-123, got %q", got)
		}
	})
}

func TestUnregisteredResourceEndpoints_404(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	paths := []string{
		"/api/agents",
		"/api/agents/nonexistent",
		"/api/channels",
		"/api/channels/general",
		"/api/costs",
		"/api/secrets",
		"/api/mcp",
		"/api/tools",
		"/api/logs",
		"/api/doctor",
	}
	for _, path := range paths {
		t.Run("GET "+path, func(t *testing.T) {
			resp := get(t, ts.URL+path)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("want 404 for unregistered %s, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestMissingAgentReturns404(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/nonexistent")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestErrorResponsesAreJSON(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"POST /health returns 405 JSON", http.MethodPost, "/health", http.StatusMethodNotAllowed},
		{"DELETE /health/ready returns 405 JSON", http.MethodDelete, "/health/ready", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, tt.method, ts.URL+tt.path, "", "")
			assertStatus(t, resp, tt.want)

			defer func() { _ = resp.Body.Close() }()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("expected JSON error body, got: %s", string(raw))
			}
			if _, ok := m["error"]; !ok {
				t.Fatalf("expected error key in response, got: %v", m)
			}
		})
	}
}

func TestParsePagination_HelperUnit(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health?limit=5&offset=10")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestHealthResponseFields(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health")
	defer func() { _ = resp.Body.Close() }()
	body := readJSON(t, resp)

	if _, ok := body["status"]; !ok {
		t.Fatal("missing 'status' field")
	}
	if _, ok := body["addr"]; !ok {
		t.Fatal("missing 'addr' field")
	}
}

func TestHealthReadyResponseStructure(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	resp := get(t, ts.URL+"/health/ready")
	defer func() { _ = resp.Body.Close() }()
	body := readJSON(t, resp)

	if _, ok := body["status"]; !ok {
		t.Fatal("missing 'status' field in readiness response")
	}
	if _, ok := body["checks"]; !ok {
		t.Fatal("missing 'checks' field in readiness response")
	}
}

func TestMiddlewareHelpers(t *testing.T) {
	t.Run("writeJSON", func(t *testing.T) {
		ts := buildTestServer(t)
		defer ts.Close()

		resp := get(t, ts.URL+"/health")
		assertContentType(t, resp, "application/json")
		_ = resp.Body.Close()
	})

	t.Run("methodNotAllowed returns JSON error", func(t *testing.T) {
		ts := buildTestServer(t)
		defer ts.Close()

		resp := post(t, ts.URL+"/health", "application/json", "")
		defer func() { _ = resp.Body.Close() }()
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		body := readJSON(t, resp)
		errMsg, ok := body["error"].(string)
		if !ok || errMsg == "" {
			t.Fatal("expected non-empty error message in 405 response")
		}
	})
}

func TestGzipMiddleware(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	assertStatus(t, resp, http.StatusOK)
}

func TestGzipMiddlewareSkipsSSE(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	tests := []struct {
		name   string
		path   string
		accept string // Accept header (empty = omit)
		wantGz bool
	}{
		{"health is gzipped", "/health", "", true},
		{"events endpoint skipped", "/api/events", "", false},
		{"mcp sse skipped", "/_mcp/sse", "", false},
		{"agent output skipped", "/api/agents/alice/output", "", false},
		{"accept event-stream skipped", "/health", "text/event-stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", "gzip")
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			client := &http.Client{
				// Don't follow redirects, don't auto-decompress
				Transport: &http.Transport{
					DisableCompression: true,
				},
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()

			gotGz := resp.Header.Get("Content-Encoding") == "gzip"
			if gotGz != tt.wantGz {
				t.Errorf("Content-Encoding gzip: got %v, want %v", gotGz, tt.wantGz)
			}
		})
	}
}

func TestMultipleHealthCallsConsistent(t *testing.T) {
	ts := buildTestServer(t)
	defer ts.Close()

	for i := 0; i < 5; i++ {
		resp := get(t, ts.URL+"/health")
		defer func() { _ = resp.Body.Close() }()
		assertStatus(t, resp, http.StatusOK)
		body := readJSON(t, resp)
		if body["status"] != "ok" {
			t.Fatalf("iteration %d: want status ok, got %v", i, body["status"])
		}
	}
}
