package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// statusStub wraps stubAdapter and records SetStatus calls, exercising the
// commitStatusSetter capability (GitHub's outbound commit-status endpoint).
//
//nolint:govet // fieldalignment: test-only struct, compactness is not a concern
type statusStub struct {
	stubAdapter
	calls []statusCall
	err   error
	mu    sync.Mutex
}

type statusCall struct {
	Owner, Repo, SHA, State, Description, TargetURL, Context string
}

func (s *statusStub) SetStatus(_ context.Context, owner, repo, sha, state, description, targetURL, statusContext string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, statusCall{owner, repo, sha, state, description, targetURL, statusContext})
	return s.err
}

// statusMux registers the apps + gateway routes for /api/apps/{name}/status.
func statusMux(mgr *gateway.Manager) *http.ServeMux {
	h := NewGatewayHandler(mgr, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	NewAppsHandler(h, mgr, nil, nil).Register(mux)
	return mux
}

func buildStatusRequest(t *testing.T, platform string, body map[string]any) *http.Request {
	t.Helper()
	b, _ := json.Marshal(body) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/apps/"+platform+"/status", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestGatewayCommitStatus_OK(t *testing.T) {
	stub := &statusStub{stubAdapter: stubAdapter{name: "github"}}
	mgr := gateway.NewManager()
	mgr.Register(stub)

	mux := statusMux(mgr)
	req := buildStatusRequest(t, "github", map[string]any{
		"owner":       "rpuneet",
		"repo":        "mycel",
		"sha":         "abc123",
		"state":       "success",
		"description": "all checks passed",
		"target_url":  "https://ci.example.com/1",
		"context":     "mycel-ci",
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 SetStatus call, got %d", len(stub.calls))
	}
	got := stub.calls[0]
	want := statusCall{"rpuneet", "mycel", "abc123", "success", "all checks passed", "https://ci.example.com/1", "mycel-ci"}
	if got != want {
		t.Errorf("call = %+v, want %+v", got, want)
	}
}

func TestGatewayCommitStatus_MissingFields(t *testing.T) {
	mgr := gateway.NewManager()
	mgr.Register(&statusStub{stubAdapter: stubAdapter{name: "github"}})
	mux := statusMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildStatusRequest(t, "github", map[string]any{"owner": "rpuneet", "repo": "mycel"}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGatewayCommitStatus_UnknownAdapter(t *testing.T) {
	mgr := gateway.NewManager()
	mux := statusMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildStatusRequest(t, "github", map[string]any{
		"owner": "rpuneet", "repo": "mycel", "sha": "abc123", "state": "success",
	}))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGatewayCommitStatus_UnsupportedAdapter(t *testing.T) {
	mgr := gateway.NewManager()
	mgr.Register(&stubAdapter{name: "telegram"})
	mux := statusMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildStatusRequest(t, "telegram", map[string]any{
		"owner": "rpuneet", "repo": "mycel", "sha": "abc123", "state": "success",
	}))
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGatewayCommitStatus_AdapterError(t *testing.T) {
	stub := &statusStub{stubAdapter: stubAdapter{name: "github"}, err: fmt.Errorf("github: 403 forbidden")}
	mgr := gateway.NewManager()
	mgr.Register(stub)
	mux := statusMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildStatusRequest(t, "github", map[string]any{
		"owner": "rpuneet", "repo": "mycel", "sha": "abc123", "state": "success",
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGatewayCommitStatus_MethodNotAllowed(t *testing.T) {
	mgr := gateway.NewManager()
	mgr.Register(&statusStub{stubAdapter: stubAdapter{name: "github"}})
	mux := statusMux(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/github/status", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /status: status = %d, want 405", rr.Code)
	}
}
