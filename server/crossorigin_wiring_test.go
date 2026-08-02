package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/server"
)

// postTools issues the write that the vulnerability turned into remote code
// execution: storing a tool whose install_cmd is later handed to sh -c. It
// returns the status and response headers, closing the body before it does.
func postTools(t *testing.T, baseURL, origin string) (int, http.Header) {
	t.Helper()
	body := `{"name":"pwned","type":"cli","command":"sh","install_cmd":"touch /tmp/pwned"}`
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, baseURL+"/api/tools", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup
	return resp.StatusCode, resp.Header.Clone()
}

// TestCrossOriginMutationsAreRejectedEndToEnd exercises the middleware through
// the real chain in the default configuration. The unit tests in
// server/handlers pass whether or not the middleware is ever installed, so
// without this the wiring could be dropped and nothing would notice.
func TestCrossOriginMutationsAreRejectedEndToEnd(t *testing.T) {
	svc := buildTestBundle(t)

	// The shape `mycel up` produces: CORS on, origin "*".
	srv := server.New(server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}, svc, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	status, header := postTools(t, ts.URL, "https://evil.example.com")
	if status != http.StatusForbidden {
		t.Fatalf("POST /api/tools from a foreign origin = %d, want %d", status, http.StatusForbidden)
	}

	// The refusal still carries CORS headers, so the middleware sits inside the
	// CORS wrapper and the browser sees a real status rather than a network
	// error it cannot explain.
	if got := header.Get("Access-Control-Allow-Origin"); got == "" {
		t.Error("403 came back without CORS headers — ordering regression")
	}
}

// TestLocalClientsCanStillMutate is the control: the fix must not break the
// clients that legitimately write. A CLI request carries no Origin at all, and
// the web UI's carries the daemon's own.
func TestLocalClientsCanStillMutate(t *testing.T) {
	svc := buildTestBundle(t)
	srv := server.New(server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}, svc, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// CLI / SDK: no Origin header.
	if status, _ := postTools(t, ts.URL, ""); status == http.StatusForbidden {
		t.Error("a client sending no Origin was rejected — this breaks the CLI")
	}

	// Web UI: same origin as the daemon. ts.URL is the loopback address the
	// test server is listening on, which is exactly what the browser would send.
	if status, _ := postTools(t, ts.URL, ts.URL); status == http.StatusForbidden {
		t.Error("the daemon's own origin was rejected — this breaks the web UI")
	}
}

// TestReadsAreUnaffected keeps the fix scoped: GET is governed by CORS, and
// tightening writes must not start refusing reads.
func TestReadsAreUnaffected(t *testing.T) {
	svc := buildTestBundle(t)
	srv := server.New(server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}, svc, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/tools", nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("GET /api/tools was rejected as cross-origin: %d", resp.StatusCode)
	}
}
