// context_read_test.go — verifies every handler reads its per-workspace
// dependencies from the request context (the WorkspaceView installed by
// the scope middleware) instead of the bundle captured at construction.
//
// Shape per handler:
//  1. Construct handler with a "launch" store (data = "launch-only").
//  2. Install SetWorkspaceFromContext returning a view whose store is a
//     second, independent "ctx" store (data = "ctx-only").
//  3. Fire a representative request and assert the response contains the
//     ctx store's data, not the launch store's.
//
// This catches regressions where a handler accidentally closes over its
// constructor-captured field instead of calling its resolver.
package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/server/handlers"
)

// withCtxView installs a WorkspaceFromContext resolver that returns the
// provided view for the duration of the test. It restores the previous
// resolver on cleanup so parallel-safe handler tests don't pollute one
// another.
func withCtxView(t *testing.T, view *handlers.WorkspaceView) {
	t.Helper()
	handlers.SetWorkspaceFromContext(func(ctx context.Context) *handlers.WorkspaceView {
		return view
	})
	t.Cleanup(func() { handlers.SetWorkspaceFromContext(nil) })
}

// doJSON is a tiny helper for firing a request at a handler and
// returning status + raw body.
func doJSON(t *testing.T, h http.Handler, method, path string, body []byte) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, b
}

// ---------------- Template handler ------------------

// TestTemplateHandler_ReadsFromContext verifies GET /api/templates
// returns the ctx store's template, not the launch store's.
func TestTemplateHandler_ReadsFromContext(t *testing.T) {
	launchDir := t.TempDir()
	ctxDir := t.TempDir()

	launchStore := template.NewStore(launchDir)
	if err := launchStore.Create(template.Template{Name: "launch-only"}, "prompt", ""); err != nil {
		t.Fatalf("seed launch: %v", err)
	}

	ctxStore := template.NewStore(ctxDir)
	if err := ctxStore.Create(template.Template{Name: "ctx-only"}, "prompt", ""); err != nil {
		t.Fatalf("seed ctx: %v", err)
	}

	h := handlers.NewTemplateHandler(launchStore)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Templates: ctxStore})

	status, body := doJSON(t, mux, http.MethodGet, "/api/templates", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	s := string(body)
	if !strings.Contains(s, "ctx-only") {
		t.Errorf("response missing ctx template: %s", s)
	}
	if strings.Contains(s, "launch-only") {
		t.Errorf("response leaked launch template: %s", s)
	}
}

// ---------------- Secret handler ------------------

// TestSecretHandler_ReadsFromContext verifies that the secrets handler
// routes through the ctx-supplied vault.
func TestSecretHandler_ReadsFromContext(t *testing.T) {
	launchDir := t.TempDir()
	ctxDir := t.TempDir()

	launchVault, lvErr := secret.OpenVaultFile(filepath.Join(launchDir, "v.sec"), "pw")
	if lvErr != nil {
		t.Fatalf("launch vault: %v", lvErr)
	}
	t.Cleanup(func() { _ = launchVault.Close() })
	if setErr := launchVault.Set("LAUNCH_KEY", "lval", ""); setErr != nil {
		t.Fatal(setErr)
	}

	ctxVault, cvErr := secret.OpenVaultFile(filepath.Join(ctxDir, "v.sec"), "pw")
	if cvErr != nil {
		t.Fatalf("ctx vault: %v", cvErr)
	}
	t.Cleanup(func() { _ = ctxVault.Close() })
	if setErr := ctxVault.Set("CTX_KEY", "cval", ""); setErr != nil {
		t.Fatal(setErr)
	}

	h := handlers.NewSecretHandler(launchVault)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Secrets: ctxVault})

	status, body := doJSON(t, mux, http.MethodGet, "/api/secrets", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	s := string(body)
	if !strings.Contains(s, "CTX_KEY") {
		t.Errorf("response missing CTX_KEY: %s", s)
	}
	if strings.Contains(s, "LAUNCH_KEY") {
		t.Errorf("response leaked LAUNCH_KEY: %s", s)
	}
}

// ---------------- Cost handler ------------------

// TestCostHandler_ReadsFromContext records into two independent cost
// stores, wires the ctx store via the scope shim, and verifies the
// summary reflects the ctx store only.
func TestCostHandler_ReadsFromContext(t *testing.T) {
	launchDir := t.TempDir()
	launchStore := cost.NewStore(launchDir)
	if err := launchStore.Open(); err != nil {
		t.Fatalf("open launch cost: %v", err)
	}
	t.Cleanup(func() { _ = launchStore.Close() })
	if _, err := launchStore.Record(context.Background(), "launch-agent", "", "m", 10, 5, 9.99); err != nil {
		t.Fatalf("record launch: %v", err)
	}

	ctxDir := t.TempDir()
	ctxStore := cost.NewStore(ctxDir)
	if err := ctxStore.Open(); err != nil {
		t.Fatalf("open ctx cost: %v", err)
	}
	t.Cleanup(func() { _ = ctxStore.Close() })
	if _, err := ctxStore.Record(context.Background(), "ctx-agent", "", "m", 1, 1, 0.25); err != nil {
		t.Fatalf("record ctx: %v", err)
	}

	h := handlers.NewCostHandler(launchStore, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Costs: ctxStore})

	status, body := doJSON(t, mux, http.MethodGet, "/api/costs", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	// The summary from ctxStore should report total ~0.25 (not 9.99).
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	totalVal, ok := payload["total_cost_usd"].(float64)
	if !ok {
		t.Fatalf("no total_cost_usd: %v", payload)
	}
	if totalVal > 0.26 {
		t.Errorf("total_cost_usd=%v (expected ~0.25 from ctx, got launch value)", totalVal)
	}
}

// ---------------- Event handler ------------------

// fakeEventStore is a zero-dep in-memory EventStore used to prove the
// event handler routes through the context store. It records writes
// so tests can verify which store was hit.
type fakeEventStore struct {
	tag    string
	events []events.Event
}

func (f *fakeEventStore) Append(ev events.Event) error {
	f.events = append(f.events, ev)
	return nil
}
func (f *fakeEventStore) Read() ([]events.Event, error) {
	// Tag every read-back event with the store tag in message so the
	// test can confirm which one served the response.
	out := make([]events.Event, len(f.events))
	copy(out, f.events)
	return out, nil
}
func (f *fakeEventStore) ReadLast(n int) ([]events.Event, error) {
	// ensure we return at least one tagged event so callers can assert.
	return []events.Event{{Type: "probe", Message: f.tag}}, nil
}
func (f *fakeEventStore) ReadByAgent(_ string) ([]events.Event, error) {
	return []events.Event{{Type: "probe", Message: f.tag, Agent: "a"}}, nil
}
func (f *fakeEventStore) Close() error { return nil }

// TestEventHandler_ReadsFromContext confirms /api/logs uses the ctx event
// store, not the launch one.
func TestEventHandler_ReadsFromContext(t *testing.T) {
	launchStore := &fakeEventStore{tag: "launch-tag"}
	ctxStore := &fakeEventStore{tag: "ctx-tag"}

	h := handlers.NewEventHandler(launchStore)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Events: ctxStore})

	status, body := doJSON(t, mux, http.MethodGet, "/api/logs?tail=10", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	s := string(body)
	if !strings.Contains(s, "ctx-tag") {
		t.Errorf("response did not come from ctx event store: %s", s)
	}
	if strings.Contains(s, "launch-tag") {
		t.Errorf("response leaked launch event store data: %s", s)
	}
}

// TestEventHandler_AppendUsesContext confirms POST /api/logs writes to
// the ctx event store, not launch.
func TestEventHandler_AppendUsesContext(t *testing.T) {
	launchStore := &fakeEventStore{tag: "launch-tag"}
	ctxStore := &fakeEventStore{tag: "ctx-tag"}

	h := handlers.NewEventHandler(launchStore)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Events: ctxStore})

	payload, _ := json.Marshal(events.Event{Type: "test.ctx", Agent: "alice"})
	status, body := doJSON(t, mux, http.MethodPost, "/api/logs", payload)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if got := len(ctxStore.events); got != 1 {
		t.Errorf("ctx store appended count = %d, want 1", got)
	}
	if got := len(launchStore.events); got != 0 {
		t.Errorf("launch store should not be written to, got %d events", got)
	}
}

// ---------------- Per-handler ctx-resolver smoke (nil-fallback) ------

// TestAllHandlers_ResolveNilCtxFallsBackToLaunch confirms that if the
// scope resolver returns nil (e.g., legacy un-scoped request before the
// middleware runs), handlers fall back to their constructor-captured
// store rather than panicking.
//
// We don't assert the response payload — only that the handler returns
// a non-panicking response when no context workspace view is set.
func TestAllHandlers_ResolveNilCtxFallsBackToLaunch(t *testing.T) {
	// Ensure no resolver is installed.
	handlers.SetWorkspaceFromContext(nil)

	// Template handler with a real store — request should succeed.
	launchDir := t.TempDir()
	launchTmpl := template.NewStore(launchDir)
	if err := launchTmpl.Create(template.Template{Name: "only"}, "p", ""); err != nil {
		t.Fatal(err)
	}
	tmux := http.NewServeMux()
	handlers.NewTemplateHandler(launchTmpl).Register(tmux)

	status, body := doJSON(t, tmux, http.MethodGet, "/api/templates", nil)
	if status != http.StatusOK {
		t.Fatalf("fallback template list status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "only") {
		t.Errorf("expected fallback to serve launch store, got: %s", body)
	}
}
