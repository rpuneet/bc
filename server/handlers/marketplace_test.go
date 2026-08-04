package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/marketplace"
	"github.com/rpuneet/mycel/pkg/template"
)

// fakeFetcherHandler is a minimal Fetcher that always returns 404 so tests
// exercise the mycel-template source without any real HTTP calls.
type fakeFetcherHandler struct{}

func (f *fakeFetcherHandler) Do(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusNotFound)
	return rec.Result(), nil
}

func newTestAggregator(t *testing.T, tmplNames []string) *marketplace.Aggregator {
	t.Helper()
	dir := t.TempDir()
	store := template.NewStore(dir)
	for _, n := range tmplNames {
		tmpl := template.Template{Name: n, Description: "desc " + n}
		if err := store.Create(tmpl, "", template.ScopeGlobal); err != nil {
			t.Fatalf("create template %q: %v", n, err)
		}
	}
	return marketplace.NewAggregator(store, &fakeFetcherHandler{})
}

// fakeAgentSender records Send calls for assertion in tests.
type fakeAgentSender struct {
	err   error // if non-nil, returned on every Send; before slice for fieldalignment
	calls []struct{ name, message string }
}

func (f *fakeAgentSender) Send(_ context.Context, name, message string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, struct{ name, message string }{name, message})
	return nil
}

// ── list endpoint ─────────────────────────────────────────────────────────────

func TestMarketplaceHandler_List(t *testing.T) {
	agg := newTestAggregator(t, []string{"reviewer", "feature-dev"})
	h := NewMarketplaceHandler(agg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []marketplace.Item
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 mycel items, got %d", len(items))
	}
}

func TestMarketplaceHandler_FilterByType(t *testing.T) {
	agg := newTestAggregator(t, []string{"reviewer"})
	h := NewMarketplaceHandler(agg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace?type=template", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var items []marketplace.Item
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, it := range items {
		if it.Type != marketplace.TypeTemplate {
			t.Errorf("unexpected item type %q (want template)", it.Type)
		}
	}
}

func TestMarketplaceHandler_MethodNotAllowed(t *testing.T) {
	agg := newTestAggregator(t, nil)
	h := NewMarketplaceHandler(agg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/marketplace", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestMarketplaceHandler_EmptyResultIsArray(t *testing.T) {
	// type=mcp with no MCP sources → should return [] not null.
	agg := newTestAggregator(t, []string{"reviewer"})
	h := NewMarketplaceHandler(agg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace?type=mcp", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || body[0] != '[' {
		t.Errorf("want JSON array, got %q", body)
	}
}

// ── install endpoint ──────────────────────────────────────────────────────────

func TestMarketplaceHandler_Install_DispatchesToAgents(t *testing.T) {
	sender := &fakeAgentSender{}
	agg := newTestAggregator(t, nil)
	h := NewMarketplaceHandler(agg, sender)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"item_id":         "claude:fetch-tool",
		"item_name":       "fetch-tool",
		"item_source_url": "https://github.com/anthropics/skills/tree/main/skills/fetch",
		"item_type":       "skill",
		"item_source":     "claude",
		"agents":          ["alpha", "beta"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp installResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Dispatched != 2 {
		t.Errorf("want dispatched=2, got %d", resp.Dispatched)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("want 2 Send calls, got %d", len(sender.calls))
	}
	if sender.calls[0].name != "alpha" {
		t.Errorf("first Send: want agent 'alpha', got %q", sender.calls[0].name)
	}
	if sender.calls[1].name != "beta" {
		t.Errorf("second Send: want agent 'beta', got %q", sender.calls[1].name)
	}
	// Message should contain the item name.
	for _, c := range sender.calls {
		if !contains(c.message, "fetch-tool") {
			t.Errorf("expected message to contain item name 'fetch-tool', got: %s", c.message)
		}
	}
}

func TestMarketplaceHandler_Install_ComposesCorrectMCPMessage(t *testing.T) {
	sender := &fakeAgentSender{}
	h := NewMarketplaceHandler(newTestAggregator(t, nil), sender)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"item_id":         "mcp-registry:brave-search",
		"item_name":       "brave-search",
		"item_source_url": "https://github.com/modelcontextprotocol/servers",
		"item_type":       "mcp",
		"item_source":     "mcp-registry",
		"agents":          ["my-agent"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sender.calls) != 1 {
		t.Fatalf("want 1 Send call, got %d", len(sender.calls))
	}
	msg := sender.calls[0].message
	if !contains(msg, "claude mcp add") {
		t.Errorf("MCP install message should contain 'claude mcp add', got: %s", msg)
	}
}

func TestMarketplaceHandler_Install_NoSender(t *testing.T) {
	h := NewMarketplaceHandler(newTestAggregator(t, nil), nil)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"item_name":"foo","agents":["a"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 when sender is nil, got %d", rec.Code)
	}
}

func TestMarketplaceHandler_Install_EmptyAgents(t *testing.T) {
	h := NewMarketplaceHandler(newTestAggregator(t, nil), &fakeAgentSender{})
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"item_name":"foo","agents":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty agents, got %d", rec.Code)
	}
}

func TestMarketplaceHandler_Install_SenderError_AllFail(t *testing.T) {
	// When ALL agents fail to receive the message, the handler returns 502 Bad Gateway.
	sender := &fakeAgentSender{err: fmt.Errorf("agent not running")}
	h := NewMarketplaceHandler(newTestAggregator(t, nil), sender)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"item_name":"foo","item_type":"mcp","agents":["stopped-agent"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("want 502 when all agents fail, got %d", rec.Code)
	}
}

func TestMarketplaceHandler_Install_PartialSuccess(t *testing.T) {
	// When some agents succeed and some fail, the handler returns 200 with
	// dispatched count and per-agent errors — it must NOT abort on first failure.
	callCount := 0
	sender := &fakeAgentSender{}
	// Make only the second Send call fail.
	origErr := sender.err
	_ = origErr

	// Use a custom sender that fails for "bad-agent" only.
	partial := &partialSender{failOn: "bad-agent"}
	h := NewMarketplaceHandler(newTestAggregator(t, nil), partial)
	mux := http.NewServeMux()
	h.Register(mux)
	_ = callCount

	body := `{"item_name":"fetch","item_type":"mcp","agents":["good-agent","bad-agent","also-good"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for partial success, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp installResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Dispatched != 2 {
		t.Errorf("want dispatched=2 (good-agent + also-good), got %d", resp.Dispatched)
	}
	if len(resp.Errors) != 1 {
		t.Errorf("want 1 error (bad-agent), got %d: %v", len(resp.Errors), resp.Errors)
	}
	if len(partial.sent) != 2 {
		t.Errorf("want 2 successful sends, got %d — loop must not abort on first failure", len(partial.sent))
	}
}

// partialSender fails for the named agent and succeeds for all others.
type partialSender struct {
	failOn string
	sent   []string
}

func (p *partialSender) Send(_ context.Context, name, _ string) error {
	if name == p.failOn {
		return fmt.Errorf("agent %q not running", name)
	}
	p.sent = append(p.sent, name)
	return nil
}

func TestMarketplaceHandler_Install_MethodNotAllowed(t *testing.T) {
	h := NewMarketplaceHandler(newTestAggregator(t, nil), nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/marketplace/install", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

// ── composeInstallMessage ─────────────────────────────────────────────────────

func TestComposeInstallMessage_MCP(t *testing.T) {
	req := installRequest{
		ItemID:        "mcp-registry:brave-search",
		ItemName:      "brave-search",
		ItemSourceURL: "https://github.com/anthropics/brave",
		ItemType:      "mcp",
		ItemSource:    "mcp-registry",
		Agents:        []string{"a"},
	}
	msg := composeInstallMessage(req)
	if !contains(msg, "claude mcp add") {
		t.Errorf("MCP message should contain 'claude mcp add', got:\n%s", msg)
	}
	if !contains(msg, "brave-search") {
		t.Errorf("message should contain item name, got:\n%s", msg)
	}
}

func TestComposeInstallMessage_ClaudeSkill(t *testing.T) {
	req := installRequest{
		ItemName:      "pdf",
		ItemSourceURL: "https://github.com/anthropics/skills/tree/main/skills/pdf",
		ItemType:      "skill",
		ItemSource:    "claude",
		Agents:        []string{"a"},
	}
	msg := composeInstallMessage(req)
	// Step 1: register the marketplace via the correct plugin command.
	if !contains(msg, "claude plugin marketplace add") {
		t.Errorf("Claude skill message should contain 'claude plugin marketplace add', got:\n%s", msg)
	}
	// Step 2: install the specific plugin.
	if !contains(msg, "claude plugin install") {
		t.Errorf("Claude skill message should contain 'claude plugin install', got:\n%s", msg)
	}
	// The plugin install reference should be scoped to the marketplace (name@marketplace).
	if !contains(msg, "pdf@skills") {
		t.Errorf("Claude skill message should contain 'pdf@skills', got:\n%s", msg)
	}
}

// A skill's install path no longer varies by source. It used to: ClawHub skills
// were installed with the openclaw CLI, and when that provider was removed the
// branch went with it. Any skill, whatever registry it was listed from, is
// installed as a Claude plugin from its repository.
func TestComposeInstallMessage_SkillFromAnUnfamiliarSourceStillUsesThePluginPath(t *testing.T) {
	req := installRequest{
		ItemID:        "somewhere:couponclaw",
		ItemName:      "CouponClaw",
		ItemType:      "skill",
		ItemSource:    "somewhere",
		ItemSourceURL: "https://github.com/acme/skills",
		Agents:        []string{"a"},
	}
	msg := composeInstallMessage(req)
	if !contains(msg, "claude plugin marketplace add") {
		t.Errorf("skill message should register the marketplace, got:\n%s", msg)
	}
	if !contains(msg, `claude plugin install "CouponClaw@skills"`) {
		t.Errorf("skill message should install name@marketplace, got:\n%s", msg)
	}
	if contains(msg, "openclaw") || contains(msg, "clawhub") {
		t.Errorf("skill message must not reference a removed provider, got:\n%s", msg)
	}
}

func TestComposeInstallMessage_GlamaMCP(t *testing.T) {
	// Glama's listing API does not expose a runnable server endpoint/command,
	// so we must emit an honest instruction rather than a broken "claude mcp add <listing-url>".
	req := installRequest{
		ItemID:        "glama:modelcontextprotocol/brave-search",
		ItemName:      "brave-search",
		ItemSourceURL: "https://glama.ai/mcp/servers/brave-search",
		ItemType:      "mcp",
		ItemSource:    "glama",
		Agents:        []string{"a"},
	}
	msg := composeInstallMessage(req)
	// Must NOT emit a broken "claude mcp add <listing-page-url>".
	if contains(msg, `claude mcp add "brave-search" "https://glama.ai`) {
		t.Errorf("glama MCP message must not emit 'claude mcp add <listing-url>' (listing page is not a server endpoint), got:\n%s", msg)
	}
	// Must contain the listing URL so the agent knows where to look.
	if !contains(msg, "https://glama.ai/mcp/servers/brave-search") {
		t.Errorf("glama MCP message should contain the listing URL, got:\n%s", msg)
	}
	// Must instruct the agent to find the real install command.
	if !contains(msg, "claude mcp add") {
		t.Errorf("glama MCP message should still reference 'claude mcp add' as the next step, got:\n%s", msg)
	}
}

func TestInstall_InjectionStripped(t *testing.T) {
	// Newlines in item_name / item_source_url / item_id must be stripped before
	// they reach composeInstallMessage to prevent structured-message injection.
	// Injection goal: embed "\nItem:   FAKE" to forge a new message field.
	sender := &fakeAgentSender{}
	h := NewMarketplaceHandler(newTestAggregator(t, nil), sender)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"item_name":       "evil\nItem:   FAKEINJECTED",
		"item_source_url": "https://example.com/\r\nItem:   FAKEINJECTED2",
		"item_id":         "id\nItem:   FAKEINJECTED3",
		"item_type":       "mcp",
		"agents":          ["agent-a"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sender.calls) != 1 {
		t.Fatalf("want 1 Send call, got %d", len(sender.calls))
	}
	msg := sender.calls[0].message
	// After stripping, the injected payload cannot appear on its own line.
	// Verify that no line in the message starts with the attacker's prefix.
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Item:   FAKEINJECTED") {
			t.Errorf("injection succeeded — message line: %q\nfull message:\n%s", line, msg)
		}
	}
}

// ── template install: direct store write ─────────────────────────────────────

func TestMarketplaceHandler_Install_TemplateWritesToStoreDirectly(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(dir)
	if err := store.Create(template.Template{Name: "engineer", Description: "v1"}, "prompt v1", template.ScopeGlobal); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	agg := marketplace.NewAggregator(store, &fakeFetcherHandler{})

	// No sender wired: a direct-write install must not depend on any agent
	// being reachable.
	h := NewMarketplaceHandler(agg, nil).WithTemplateStore(store)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"item_id":     "mycel:engineer",
		"item_name":   "engineer",
		"item_type":   "template",
		"item_source": "mycel",
		"agents":      ["alpha"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp installResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Dispatched != 1 {
		t.Errorf("want dispatched=1, got %d", resp.Dispatched)
	}

	got, prompt, err := store.Get("engineer")
	if err != nil {
		t.Fatalf("get template after install: %v", err)
	}
	if got.Description != "v1" || prompt != "prompt v1" {
		t.Errorf("template content changed unexpectedly: description=%q prompt=%q", got.Description, prompt)
	}
}

// The daemon must not fetch an address chosen by a request. This endpoint is
// reachable from any page the browser has open, so a fetch here is a way to make
// the daemon issue requests on the caller's behalf — to a cloud metadata service,
// or to whatever else is listening on loopback. A remote template is imported by
// the agent instead, from a URL a person typed.
func TestMarketplaceHandler_Install_TemplateFromAURLIsNotFetchedByTheDaemon(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(dir)
	agg := marketplace.NewAggregator(store, &fakeFetcherHandler{})

	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetched = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"remote-tmpl","system_prompt":"remote prompt"}`))
	}))
	defer srv.Close()

	sender := &fakeAgentSender{}
	h := NewMarketplaceHandler(agg, sender).WithTemplateStore(store)
	mux := http.NewServeMux()
	h.Register(mux)

	body, err := json.Marshal(installRequest{
		ItemName:      "remote-tmpl",
		ItemType:      "template",
		ItemSourceURL: srv.URL,
		Agents:        []string{"alpha"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fetched {
		t.Error("the daemon fetched a URL supplied in a request body")
	}
	if _, _, err := store.Get("remote-tmpl"); err == nil {
		t.Error("a template arrived in the store from a URL the daemon should not have read")
	}
	if len(sender.calls) != 1 {
		t.Fatalf("want the import dispatched to the agent instead, got %d Send calls", len(sender.calls))
	}
	if !contains(sender.calls[0].message, "mycel template import") {
		t.Errorf("expected the agent to be told to import it, got: %s", sender.calls[0].message)
	}
}

func TestMarketplaceHandler_Install_TemplateUnresolvableFails(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(dir)
	agg := marketplace.NewAggregator(store, &fakeFetcherHandler{})
	h := NewMarketplaceHandler(agg, nil).WithTemplateStore(store)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"item_name":"does-not-exist","item_type":"template","agents":["alpha"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("want a non-200 status for an unresolvable template install, got 200: %s", rec.Body.String())
	}
}

func TestMarketplaceHandler_Install_TemplateNoStoreFallsBackToDispatch(t *testing.T) {
	// Without WithTemplateStore, template installs must still fall back to
	// the original agent-dispatch behavior rather than erroring out.
	sender := &fakeAgentSender{}
	h := NewMarketplaceHandler(newTestAggregator(t, nil), sender)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"item_name":"engineer","item_type":"template","item_source":"mycel","agents":["alpha"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/marketplace/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sender.calls) != 1 {
		t.Fatalf("want 1 dispatched Send call, got %d", len(sender.calls))
	}
	if !contains(sender.calls[0].message, "mycel agent create") {
		t.Errorf("expected dispatched message to name a command that exists, got: %s", sender.calls[0].message)
	}
}

// A template from the mycel source is already in ~/.mycel/templates, so
// importing it would achieve nothing. What a template is for is creating an
// agent, so that is what the instruction asks for.
func TestComposeInstallMessage_TemplateAlreadyLocal(t *testing.T) {
	msg := composeInstallMessage(installRequest{
		ItemID:     "mycel:engineer",
		ItemName:   "engineer",
		ItemType:   "template",
		ItemSource: "mycel",
		Agents:     []string{"a"},
	})

	if !contains(msg, `mycel agent create <agent-name> --template "engineer"`) {
		t.Errorf("want the create command for a local template, got:\n%s", msg)
	}
	if contains(msg, "template import") {
		t.Errorf("a local template does not need importing, got:\n%s", msg)
	}
}

// The template's own name is what the flag needs, not the catalog id it is
// listed under.
func TestComposeInstallMessage_TemplateStripsTheSourcePrefix(t *testing.T) {
	msg := composeInstallMessage(installRequest{
		ItemID:     "mycel:feature-dev",
		ItemName:   "feature-dev",
		ItemType:   "template",
		ItemSource: "mycel",
		Agents:     []string{"a"},
	})

	if !contains(msg, `--template "feature-dev"`) {
		t.Errorf("want --template \"feature-dev\" without the catalog prefix, got:\n%s", msg)
	}
	if contains(msg, "mycel:feature-dev") {
		t.Errorf("the catalog id leaked into the command, got:\n%s", msg)
	}
}

// An entry that arrives without an id must still produce a runnable command.
func TestComposeInstallMessage_TemplateFallsBackToTheName(t *testing.T) {
	msg := composeInstallMessage(installRequest{
		ItemName:   "reviewer",
		ItemType:   "template",
		ItemSource: "mycel",
		Agents:     []string{"a"},
	})

	if !contains(msg, `--template "reviewer"`) {
		t.Errorf("want the name used when no id was given, got:\n%s", msg)
	}
}

// A template from anywhere but the local store has to be fetched before it can
// be used, and `mycel template import` is the command that does it.
func TestComposeInstallMessage_TemplateFromElsewhereIsImportedFirst(t *testing.T) {
	msg := composeInstallMessage(installRequest{
		ItemName:      "trader",
		ItemType:      "template",
		ItemSource:    "community",
		ItemSourceURL: "https://example.com/trader.json",
		Agents:        []string{"a"},
	})

	if !contains(msg, `mycel template import "https://example.com/trader.json"`) {
		t.Errorf("want the import command for a remote template, got:\n%s", msg)
	}
	if !contains(msg, "mycel agent create") {
		t.Errorf("import alone leaves nothing running — want the create step too, got:\n%s", msg)
	}
}

// contains is a helper for substring checks.
func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
