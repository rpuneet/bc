package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/deps"
)

// fakeDep is a handler-side test double implementing deps.Dependency.
type fakeDep struct {
	startErr   error
	stopErr    error
	id         string
	state      deps.State
	deprecated bool
}

func (f *fakeDep) ID() string          { return f.id }
func (f *fakeDep) DisplayName() string { return f.id }
func (f *fakeDep) Description() string { return "fake" }
func (f *fakeDep) Status(_ context.Context) (deps.State, error) {
	if f.state == "" {
		return deps.StateStopped, nil
	}
	return f.state, nil
}
func (f *fakeDep) Start(_ context.Context) error                   { return f.startErr }
func (f *fakeDep) Stop(_ context.Context) error                    { return f.stopErr }
func (f *fakeDep) Logs(_ context.Context, _ int) ([]string, error) { return []string{"log-a"}, nil }
func (f *fakeDep) Deprecated() bool                                { return f.deprecated }

func newDepsMux(t *testing.T, reg *deps.Registry) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	NewDepsHandler(reg).Register(mux)
	return mux
}

func newLoopbackRequest(method, url string) *http.Request {
	req := httptest.NewRequest(method, url, nil)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

func TestDepsListEmpty(t *testing.T) {
	reg := deps.NewRegistry()
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodGet, "/api/deps")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Deps []depView `json:"deps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Deps) != 0 {
		t.Errorf("deps = %d, want 0", len(body.Deps))
	}
}

func TestDepsListWithEntries(t *testing.T) {
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha", state: deps.StateRunning})
	reg.Register(&fakeDep{id: "beta", deprecated: true})
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodGet, "/api/deps")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Deps []depView `json:"deps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Deps) != 2 {
		t.Fatalf("deps = %d, want 2", len(body.Deps))
	}
	if body.Deps[0].ID != "alpha" || body.Deps[0].State != string(deps.StateRunning) {
		t.Errorf("alpha mismatch: %+v", body.Deps[0])
	}
	if body.Deps[1].ID != "beta" || !body.Deps[1].Deprecated {
		t.Errorf("beta mismatch: %+v", body.Deps[1])
	}
}

func TestDepsStartDeprecatedReturns409(t *testing.T) {
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "legacy", deprecated: true, startErr: errors.New("deprecated")})
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodPost, "/api/deps/legacy/start")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestDepsStartNonLoopbackReturns403(t *testing.T) {
	t.Setenv("MYCEL_REMOTE", "")
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha"})
	mux := newDepsMux(t, reg)

	req := httptest.NewRequest(http.MethodPost, "/api/deps/alpha/start", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDepsStartRemoteAllowedWithEnv(t *testing.T) {
	t.Setenv("MYCEL_REMOTE", "1")
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha"})
	mux := newDepsMux(t, reg)

	req := httptest.NewRequest(http.MethodPost, "/api/deps/alpha/start", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (MYCEL_REMOTE=1 allows non-loopback)", rec.Code)
	}
}

func TestDepsStartOK(t *testing.T) {
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha", state: deps.StateRunning})
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodPost, "/api/deps/alpha/start")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
}

func TestDepsStopOK(t *testing.T) {
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha"})
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodPost, "/api/deps/alpha/stop")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}
}

func TestDepsStatusEndpoint(t *testing.T) {
	reg := deps.NewRegistry()
	reg.Register(&fakeDep{id: "alpha", state: deps.StateRunning})
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodGet, "/api/deps/alpha/status")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body depView
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.State != string(deps.StateRunning) {
		t.Errorf("state = %q, want running", body.State)
	}
}

func TestDepsNotFound(t *testing.T) {
	reg := deps.NewRegistry()
	mux := newDepsMux(t, reg)

	req := newLoopbackRequest(http.MethodGet, "/api/deps/missing")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
