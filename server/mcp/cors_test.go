package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/bc/server/mcp"
)

// ─── #2960: MCP SSE CORS wildcard ────────────────────────────────────────────

// TestSSE_CORSOrigin_DefaultsToWildcard preserves the historic loopback
// behavior: callers that don't set an explicit origin get "*".
func TestSSE_CORSOrigin_DefaultsToWildcard(t *testing.T) {
	b := mcp.NewSSEBroker()

	srv := httptest.NewServer(b.SSEHandler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestSSE_CORSOrigin_RespectsConfiguredOrigin is the regression test for
// #2960: when the broker has a configured origin, the SSE response must
// echo that origin instead of the wildcard.
func TestSSE_CORSOrigin_RespectsConfiguredOrigin(t *testing.T) {
	b := mcp.NewSSEBroker()
	b.SetCORSOrigin("https://app.example.com")

	srv := httptest.NewServer(b.SSEHandler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got == "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q — wildcard must NOT leak when an origin is configured (#2960)", got)
	}
	if got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want https://app.example.com", got)
	}
}
