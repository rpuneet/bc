package mcp

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// stubProvider is a minimal provider.Provider that "runs" as /bin/true so
// spawn_agent tests exercise a real Manager.SpawnAgentWithOptions call
// without depending on any real AI CLI being installed.
type stubProvider struct{ name string }

// The stub command sleeps rather than exiting immediately: tests that send
// keystrokes to the spawned session (send_to_agent) need it to still be
// alive by the time the tool call reaches SendKeys.
const stubProviderCmd = "sleep 100"

func (p stubProvider) Name() string                               { return p.name }
func (p stubProvider) Description() string                        { return "test stub" }
func (p stubProvider) Command() string                            { return stubProviderCmd }
func (p stubProvider) Binary() string                             { return "sleep" }
func (p stubProvider) InstallHint() string                        { return "" }
func (p stubProvider) BuildCommand(_ provider.CommandOpts) string { return stubProviderCmd }
func (p stubProvider) IsInstalled(_ context.Context) bool         { return true }
func (p stubProvider) Version(_ context.Context) string           { return "stub" }

func init() {
	// Registered once for the whole test binary — harmless to share since
	// tests pick a unique agent/repo dir per case.
	provider.DefaultRegistry.Register(stubProvider{name: "mcp-test-tool"})
}

// withRoleHierarchy sets agent.RoleHierarchy[parent] for the duration of the
// test and restores the previous value on cleanup. RoleHierarchy is a
// package-level map read by agent.CanCreateRole — the same mechanism the
// daemon uses (via ParentID) to gate --parent-based agent creation, and
// what spawn_agent inherits by always passing the caller as parent.
func withRoleHierarchy(t *testing.T, parent agent.Role, children ...agent.Role) {
	t.Helper()
	prev, had := agent.RoleHierarchy[parent]
	agent.RoleHierarchy[parent] = children
	t.Cleanup(func() {
		if had {
			agent.RoleHierarchy[parent] = prev
		} else {
			delete(agent.RoleHierarchy, parent)
		}
	})
}

// errText extracts the text of a tool error result for readable test
// failure messages.
func errText(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		return "<no content>"
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		return "<non-text content>"
	}
	return tc.Text
}

// newOrchestrationSvc builds a real Manager+AgentService rooted at a fresh
// repo, for tests that need actual spawn/send/stop behavior rather than
// gateway/notify stubs.
func newOrchestrationSvc(t *testing.T) (*agent.Manager, *agent.AgentService, *home.Home) {
	t.Helper()
	h := testRepo(t)
	mgr := agent.NewManagerWithRepo(h.AgentsDir(), h.RootDir)
	svc := agent.NewAgentService(mgr, nil, nil)
	return mgr, svc, h
}

// ─── spawn_agent ────────────────────────────────────────────────────────────

func TestE2E_SpawnAgent_Success(t *testing.T) {
	withRoleHierarchy(t, agent.Role("manager"), agent.Role("engineer"))
	mgr, svc, h := newOrchestrationSvc(t)

	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "boss", Role: agent.Role("manager"), Repo: h.RootDir,
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "boss")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name: "spawn_agent",
		Arguments: map[string]any{
			"name":     "worker-1",
			"role":     "engineer",
			"task":     "implement the thing",
			"provider": "mcp-test-tool",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("spawn_agent errored: %v", errText(t, res))
	}
	out := structured(t, res)
	if out["agent"] != "worker-1" {
		t.Errorf("agent = %v, want worker-1", out["agent"])
	}
	if out["role"] != "engineer" {
		t.Errorf("role = %v, want engineer", out["role"])
	}
	if out["parent_id"] != "boss" {
		t.Errorf("parent_id = %v, want boss", out["parent_id"])
	}

	child := mgr.GetAgent("worker-1")
	if child == nil {
		t.Fatal("worker-1 was not registered in the manager")
	}
	if child.Task != "implement the thing" {
		t.Errorf("child task = %q, want the spawn_agent task", child.Task)
	}

	children := mgr.ListChildren("boss")
	if len(children) != 1 || children[0].Name != "worker-1" {
		t.Errorf("boss's children = %+v, want [worker-1]", children)
	}
}

// spawnChild runs spawn_agent as "boss" and returns the registered child. The
// parent is seeded with parentTemplate so inheritance can be observed.
func spawnChild(t *testing.T, parentTemplate string, args map[string]any) (*agent.Agent, map[string]any) {
	t.Helper()
	withRoleHierarchy(t, agent.Role("manager"), agent.Role("engineer"))
	mgr, svc, h := newOrchestrationSvc(t)

	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "boss", Role: agent.Role("manager"), Repo: h.RootDir, Template: parentTemplate,
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "boss")

	if args["role"] == nil {
		args["role"] = "engineer"
	}
	if args["provider"] == nil {
		args["provider"] = "mcp-test-tool"
	}
	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "spawn_agent", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("spawn_agent errored: %v", errText(t, res))
	}

	name, _ := args["name"].(string)
	child := mgr.GetAgent(name)
	if child == nil {
		t.Fatalf("%q was not registered in the manager", name)
	}
	return child, structured(t, res)
}

// TestE2E_SpawnAgent_RecordsTemplate covers the guardrail bypass: guardrails are
// enforced per template and the enforcement loop skips any agent whose Template
// is empty, so a child spawned without one is exempt from its cost cap and stuck
// detection. spawn_agent had no template field at all, which made every
// agent-spawned child unguarded — the unattended case guardrails exist for.
func TestE2E_SpawnAgent_RecordsTemplate(t *testing.T) {
	child, out := spawnChild(t, "", map[string]any{"name": "worker-tmpl", "template": "capped"})

	if child.Template != "capped" {
		t.Errorf("child template = %q, want %q — the guardrail loop skips agents with none",
			child.Template, "capped")
	}
	// Reported back, so a caller can see the child is covered rather than assume it.
	if out["template"] != "capped" {
		t.Errorf("reported template = %v, want capped", out["template"])
	}
}

// TestE2E_SpawnAgent_InheritsParentTemplate: omitting the field must not mean
// "unguarded". Every caller written before the field existed omits it, so
// without inheritance the bypass would survive the fix that added it.
func TestE2E_SpawnAgent_InheritsParentTemplate(t *testing.T) {
	child, out := spawnChild(t, "capped", map[string]any{"name": "worker-inherit"})

	if child.Template != "capped" {
		t.Errorf("child template = %q, want the parent's %q", child.Template, "capped")
	}
	if out["template"] != "capped" {
		t.Errorf("reported template = %v, want capped", out["template"])
	}
}

// TestE2E_SpawnAgent_ExplicitTemplateWins keeps inheritance from becoming a
// straitjacket: a caller may still spawn a child under a different template.
func TestE2E_SpawnAgent_ExplicitTemplateWins(t *testing.T) {
	child, _ := spawnChild(t, "capped", map[string]any{"name": "worker-override", "template": "generous"})

	if child.Template != "generous" {
		t.Errorf("child template = %q, want the explicitly requested %q", child.Template, "generous")
	}
}

// TestE2E_SpawnAgent_NoTemplateAnywhere: an unguarded parent still spawns an
// unguarded child. Inheritance propagates guardrails, it does not invent them.
func TestE2E_SpawnAgent_NoTemplateAnywhere(t *testing.T) {
	child, _ := spawnChild(t, "", map[string]any{"name": "worker-none"})

	if child.Template != "" {
		t.Errorf("child template = %q, want empty when neither side names one", child.Template)
	}
}

// TestE2E_SpawnAgent_DeniedWithoutCapability verifies the role-hierarchy
// gate: a caller whose role has no entry permitting the requested child
// role is rejected and no agent is created.
func TestE2E_SpawnAgent_DeniedWithoutCapability(t *testing.T) {
	// "intern" is not granted permission to create any role.
	withRoleHierarchy(t, agent.Role("intern"))
	mgr, svc, h := newOrchestrationSvc(t)

	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "junior", Role: agent.Role("intern"), Repo: h.RootDir,
	}); err != nil {
		t.Fatalf("seed caller: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "junior")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "spawn_agent",
		Arguments: map[string]any{"role": "engineer", "provider": "mcp-test-tool"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("spawn_agent should be denied for a role with no create permission")
	}
	if got := mgr.ListAgents(); len(got) != 1 {
		t.Errorf("expected no child to be created, agents = %+v", got)
	}
}

func TestE2E_SpawnAgent_MissingRole(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "boss")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "spawn_agent", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("spawn_agent with no role should be a tool error")
	}
}

// ─── send_to_agent ──────────────────────────────────────────────────────────

func TestE2E_SendToAgent_Delivers(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if _, err := svc.Create(t.Context(), agent.CreateOptions{
		Name: "sender", Role: agent.Role("manager"), Tool: "mcp-test-tool", Repo: h.RootDir,
	}); err != nil {
		t.Fatalf("seed sender: %v", err)
	}
	// Give send_to_agent a genuinely running session to write into —
	// AgentService.Send dispatches straight to tmux SendKeys once the
	// agent isn't stopped/starting, so a stub RegisterStopped entry
	// without a real session would fail with "session not found".
	if _, err := svc.Create(t.Context(), agent.CreateOptions{
		Name: "receiver", Role: agent.Role("engineer"), Tool: "mcp-test-tool", Repo: h.RootDir,
	}); err != nil {
		t.Fatalf("seed receiver: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "sender")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "send_to_agent",
		Arguments: map[string]any{"agent": "receiver", "message": "please review PR #1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("send_to_agent errored: %v", errText(t, res))
	}
	out := structured(t, res)
	if out["agent"] != "receiver" {
		t.Errorf("agent = %v, want receiver", out["agent"])
	}
}

func TestE2E_SendToAgent_UnknownTarget(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "sender")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "send_to_agent",
		Arguments: map[string]any{"agent": "ghost", "message": "hello?"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("send_to_agent to an unknown agent should be a tool error")
	}
}

// ─── stop_agent ─────────────────────────────────────────────────────────────

func TestE2E_StopAgent_PermittedForOwnChild(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "boss", Role: agent.Role("manager"), Repo: h.RootDir, Children: []string{"worker-1"},
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "worker-1", Role: agent.Role("engineer"), Repo: h.RootDir, ParentID: "boss",
	}); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if err := mgr.UpdateAgentState(t.Context(), "worker-1", agent.StateIdle, ""); err != nil {
		t.Fatalf("mark idle: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "boss")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "stop_agent",
		Arguments: map[string]any{"agent": "worker-1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("stop_agent on own child should succeed: %v", res.Content)
	}
	child := mgr.GetAgent("worker-1")
	if child == nil || child.State != agent.StateStopped {
		t.Errorf("worker-1 state = %+v, want stopped", child)
	}
}

func TestE2E_StopAgent_DeniedForUnrelatedAgent(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{Name: "peer-a", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed peer-a: %v", err)
	}
	if err := mgr.RegisterStopped(&agent.Agent{Name: "peer-b", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed peer-b: %v", err)
	}
	if err := mgr.UpdateAgentState(t.Context(), "peer-b", agent.StateIdle, ""); err != nil {
		t.Fatalf("mark idle: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "peer-a")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "stop_agent",
		Arguments: map[string]any{"agent": "peer-b"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("stop_agent on an unrelated peer should be denied")
	}
	if peerB := mgr.GetAgent("peer-b"); peerB == nil || peerB.State != agent.StateIdle {
		t.Errorf("peer-b should be untouched, got %+v", peerB)
	}
}

func TestE2E_StopAgent_RootCanStopAnyone(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{Name: "root", Role: agent.RoleRoot, Repo: h.RootDir}); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := mgr.RegisterStopped(&agent.Agent{Name: "stranger", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	if err := mgr.UpdateAgentState(t.Context(), "stranger", agent.StateIdle, ""); err != nil {
		t.Fatalf("mark idle: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "root")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "stop_agent",
		Arguments: map[string]any{"agent": "stranger"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("root should be permitted to stop any agent: %v", res.Content)
	}
}

func TestE2E_StopAgent_DeniedForSelf(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{Name: "solo", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed solo: %v", err)
	}
	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "solo")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{
		Name:      "stop_agent",
		Arguments: map[string]any{"agent": "solo"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("stop_agent on self should be rejected")
	}
}

// ─── list_children ──────────────────────────────────────────────────────────

func TestE2E_ListChildren(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{
		Name: "boss", Role: agent.Role("manager"), Repo: h.RootDir,
		Children: []string{"worker-1", "worker-2"},
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := mgr.RegisterStopped(&agent.Agent{Name: "worker-1", Role: agent.Role("engineer"), Repo: h.RootDir, ParentID: "boss"}); err != nil {
		t.Fatalf("seed worker-1: %v", err)
	}
	if err := mgr.RegisterStopped(&agent.Agent{Name: "worker-2", Role: agent.Role("qa"), Repo: h.RootDir, ParentID: "boss"}); err != nil {
		t.Fatalf("seed worker-2: %v", err)
	}
	// A sibling with no relation to "boss" must not leak into the listing.
	if err := mgr.RegisterStopped(&agent.Agent{Name: "unrelated", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed unrelated: %v", err)
	}

	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "boss")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "list_children"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_children errored: %v", res.Content)
	}
	out := structured(t, res)
	children, _ := out["children"].([]any)
	if len(children) != 2 {
		t.Fatalf("children = %v, want 2 entries", children)
	}
	names := map[string]bool{}
	for _, c := range children {
		m, _ := c.(map[string]any)
		name, _ := m["name"].(string)
		names[name] = true
	}
	if !names["worker-1"] || !names["worker-2"] {
		t.Errorf("children names = %v, want worker-1 and worker-2", names)
	}
	if names["unrelated"] {
		t.Error("list_children leaked an unrelated agent")
	}
}

func TestE2E_ListChildren_NoneSpawned(t *testing.T) {
	mgr, svc, h := newOrchestrationSvc(t)
	if err := mgr.RegisterStopped(&agent.Agent{Name: "loner", Role: agent.Role("engineer"), Repo: h.RootDir}); err != nil {
		t.Fatalf("seed loner: %v", err)
	}
	session, _ := newTestSession(t, Config{Home: h, Agents: mgr, AgentSvc: svc}, "loner")

	res, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "list_children"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_children errored: %v", res.Content)
	}
	out := structured(t, res)
	if children, ok := out["children"]; ok && children != nil {
		if arr, _ := children.([]any); len(arr) != 0 {
			t.Errorf("children = %v, want none", children)
		}
	}
}
