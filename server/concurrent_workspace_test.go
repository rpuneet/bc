// concurrent_workspace_test.go — concurrency hammer over multi-workspace
// bcd. Boots 5 workspaces, fires 100 concurrent requests spread across
// them, and asserts:
//
//  1. No deadlocks (test completes under the test timeout).
//  2. No data leakage between workspaces on the templates route (which
//     has full per-workspace isolation via file-based store).
//  3. No 5xx responses — concurrent scoped dispatch must be race-safe.
//
// Race detector ("go test -race") will surface map / slice / pointer
// races inside the WorkspaceManager, Registry, scope middleware, or
// hub. Run this file under -race in CI.
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	bccost "github.com/rpuneet/mycel/pkg/cost"
	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// wsSetup is a single workspace's test state.
type wsSetup struct {
	id   string
	path string
}

// concurrentHarness holds a bcd bound to N workspaces.
type concurrentHarness struct {
	ts   *httptest.Server
	mgr  *server.WorkspaceManager
	list []wsSetup
}

func (h *concurrentHarness) close() {
	h.ts.Close()
	_ = h.mgr.Close() //nolint:errcheck
}

// newConcurrentHarness registers n workspaces and boots a bcd against them.
func newConcurrentHarness(t *testing.T, n int) *concurrentHarness {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	list := make([]wsSetup, n)
	root := t.TempDir()
	for i := 0; i < n; i++ {
		p := filepath.Join(root, fmt.Sprintf("ws%d", i))
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		gitInitDir(t, p)
		if _, err := workspace.Init(p); err != nil {
			t.Fatalf("init: %v", err)
		}
		list[i] = wsSetup{id: workspace.ComputeWorkspaceID(p), path: p}
	}

	// All workspaces share the single global mycel.db, opened lazily by
	// BuildWorkspaceServices; release it after the test.
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

	reg, regErr := workspace.LoadRegistry()
	if regErr != nil {
		t.Fatalf("load registry: %v", regErr)
	}
	for i, w := range list {
		alias := fmt.Sprintf("ws%d", i)
		if err := reg.RegisterWithAlias(w.path, alias, alias); err != nil {
			t.Fatalf("register %s: %v", alias, err)
		}
	}
	if err := reg.SetActive(list[0].path); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	costsGlobal, cgErr := bccost.OpenGlobalStore(filepath.Join(home, ".bc", "costs.db"))
	if cgErr != nil {
		t.Fatalf("open global costs: %v", cgErr)
	}
	t.Cleanup(func() { _ = costsGlobal.Close() })

	globals := &server.Globals{
		Registry:    reg,
		GlobalHub:   hub,
		CostsGlobal: costsGlobal,
	}

	ctx := context.Background()
	mgr := server.NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*server.WorkspaceServices, error) {
		gitInitDir(t, w.RootDir)
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	// Eager-load every workspace so the concurrency hammer exercises
	// dispatch, not first-time load races.
	for _, w := range list {
		if _, err := mgr.Load(ctx, w.id); err != nil {
			t.Fatalf("load %s: %v", w.id, err)
		}
	}

	srv := server.NewWithManager(server.Config{Addr: "127.0.0.1:0", CORS: true}, mgr, globals, nil)
	ts := httptest.NewServer(srv.Handler())

	return &concurrentHarness{ts: ts, mgr: mgr, list: list}
}

// TestConcurrent_ScopedDispatch fires 100 parallel requests spread
// across five workspaces and verifies there are no 5xx responses and
// no cross-contamination on per-workspace data (templates).
func TestConcurrent_ScopedDispatch(t *testing.T) {
	const wsCount = 5
	const totalReqs = 100
	h := newConcurrentHarness(t, wsCount)
	defer h.close()

	// Pre-seed each workspace with a distinct template so we can assert
	// list responses surface only that workspace's entry.
	for i, w := range h.list {
		body := map[string]any{
			"name":          fmt.Sprintf("tpl-ws%d", i),
			"system_prompt": "probe",
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.ts.URL+"/api/templates?workspace="+w.id, bytes.NewReader(b))
		if err != nil {
			t.Fatalf("build seed req: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		_ = resp.Body.Close()
	}

	var wg sync.WaitGroup
	var errCount atomic.Int32
	var crossLeaks atomic.Int32

	for n := 0; n < totalReqs; n++ {
		wg.Add(1)
		idx := n % wsCount
		target := h.list[idx]
		wantName := fmt.Sprintf("tpl-ws%d", idx)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.ts.URL+"/api/templates?workspace="+target.id, nil)
			if err != nil {
				errCount.Add(1)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errCount.Add(1)
				return
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode >= 500 {
				errCount.Add(1)
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			body := string(raw)

			// The ws-scoped entry we expect must be present.
			if !containsTemplateName(body, wantName) {
				errCount.Add(1)
				return
			}

			// Cross-contamination check: any OTHER workspace's template
			// name showing up at workspace-scope level is a leak.
			for j := 0; j < wsCount; j++ {
				if j == idx {
					continue
				}
				other := fmt.Sprintf("tpl-ws%d", j)
				if hasWorkspaceScopedTemplate(body, other) {
					crossLeaks.Add(1)
					return
				}
			}
		}()
	}

	wg.Wait()

	if errCount.Load() > 0 {
		t.Errorf("concurrent requests produced %d errors", errCount.Load())
	}
	if crossLeaks.Load() > 0 {
		t.Errorf("cross-workspace leaks: %d", crossLeaks.Load())
	}
}

// containsTemplateName returns true if the response body mentions the
// given template name in any scope.
func containsTemplateName(body, name string) bool {
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return false
	}
	for _, e := range list {
		if e["name"] == name {
			return true
		}
	}
	return false
}

// hasWorkspaceScopedTemplate returns true only when a template entry
// with the given name is reported with scope="workspace". Global-scope
// leaks would legitimately surface under other workspaces and are
// intentional (global templates are visible to all workspaces).
func hasWorkspaceScopedTemplate(body, name string) bool {
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return false
	}
	for _, e := range list {
		if e["name"] == name && e["scope"] == "workspace" {
			return true
		}
	}
	return false
}
