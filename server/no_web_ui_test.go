// no_web_ui_test.go — a daemon built without the UI bundle registered no root
// handler, so a browser got Go's bare "404 page not found": identical to a dead
// daemon, a wrong port, or a crashed server. It now says which it is.
package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fetched is what a browser or client actually sees: the status, the type it
// was told to expect, and the bytes. Returning these rather than the response
// keeps the body's lifetime inside the helper that opened it.
type fetched struct {
	contentType string
	body        string
	status      int
}

func get(t *testing.T, url string) fetched {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return fetched{
		status:      resp.StatusCode,
		contentType: resp.Header.Get("Content-Type"),
		body:        string(body),
	}
}

func TestWithoutTheUIBundleTheRootPathExplainsItself(t *testing.T) {
	// nil static files is exactly what WebDist returns from a binary built
	// without the UI.
	ts := httptest.NewServer(New(Config{Addr: "127.0.0.1:0"}, Services{}, nil, nil).Handler())
	defer ts.Close()

	got := get(t, ts.URL+"/")

	// 503, not 404: the daemon answered, and this address serves the UI once
	// the binary carries it.
	if got.status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 — the daemon is up, only its UI is missing", got.status)
	}
	if ct := got.contentType; !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want HTML: a person is reading this in a browser", ct)
	}
	// The page has to answer the two questions being asked: is the daemon
	// alive, and what do I run to fix it.
	for _, want := range []string{"daemon is running", "make build-local", "/api/agents"} {
		if !strings.Contains(got.body, want) {
			t.Errorf("page does not mention %q; it is the only thing standing in for the UI", want)
		}
	}
}

func TestWithoutTheUIBundleAPIPathsStillAnswerJSON(t *testing.T) {
	ts := httptest.NewServer(New(Config{Addr: "127.0.0.1:0"}, Services{}, nil, nil).Handler())
	defer ts.Close()

	// An unknown API path must not hand a client an HTML page to parse,
	// whether or not the UI happens to be bundled.
	got := get(t, ts.URL+"/api/definitely-not-a-route")

	if got.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown API route", got.status)
	}
	if ct := got.contentType; !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON for an API path", ct)
	}
	if !strings.Contains(got.body, `"error"`) {
		t.Errorf("body = %q, want a JSON error", got.body)
	}
}

// A route the daemon really has must not be shadowed by the explanatory page.
func TestWithoutTheUIBundleRealRoutesStillWork(t *testing.T) {
	ts := httptest.NewServer(New(Config{Addr: "127.0.0.1:0"}, Services{}, nil, nil).Handler())
	defer ts.Close()

	got := get(t, ts.URL+"/api/health")

	if got.status != http.StatusOK {
		t.Errorf("status = %d, want 200 from /api/health", got.status)
	}
	if !strings.Contains(got.body, "status") {
		t.Errorf("body = %q, want the health payload", got.body)
	}
}
